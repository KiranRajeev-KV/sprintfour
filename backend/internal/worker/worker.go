package worker

import (
	"backend/internal/detector"
	"backend/internal/document"
	"backend/internal/store"
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

type JobStatus string

const (
	JobQueued    JobStatus = "QUEUED"
	JobRunning   JobStatus = "RUNNING"
	JobSucceeded JobStatus = "SUCCEEDED"
	JobFailed    JobStatus = "FAILED"
	JobCancelled JobStatus = "CANCELLED"
)

type Job struct {
	ID         string
	DocumentID string
	text       string
	Status     JobStatus
	Attempts   int
	MaxRetries int
	Error      string
	CreatedAt  time.Time
}

type WorkerPool struct {
	logger   *slog.Logger
	store    *store.Store
	detector detector.Detector
	jobs     chan *Job
	size     int
	wg       sync.WaitGroup
	ctx      context.Context
	cancel   context.CancelFunc
	started  bool
	mu       sync.RWMutex
	byID     map[string]*Job
	jobSeq   int
}

func NewWorkerPool(logger *slog.Logger, store *store.Store, detector detector.Detector, workerCount, queueDepth int) *WorkerPool {
	ctx, cancel := context.WithCancel(context.Background())
	if logger == nil {
		logger = slog.Default()
	}
	if workerCount <= 0 {
		workerCount = 1
	}
	if queueDepth <= 0 {
		queueDepth = workerCount
	}
	return &WorkerPool{
		logger:   logger,
		store:    store,
		detector: detector,
		jobs:     make(chan *Job, queueDepth),
		size:     workerCount,
		ctx:      ctx,
		cancel:   cancel,
		byID:     make(map[string]*Job),
	}
}

func (p *WorkerPool) Start() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.started {
		return
	}
	p.started = true
	for i := 0; i < p.size; i++ {
		p.wg.Add(1)
		go p.worker(i)
	}
	p.logger.Info("worker_pool_started", slog.Int("size", p.size))
}

func (p *WorkerPool) Submit(documentID string, text string) {
	p.mu.Lock()
	p.jobSeq++
	job := &Job{
		ID:         fmt.Sprintf("job_%06d", p.jobSeq),
		DocumentID: documentID,
		text:       text,
		Status:     JobQueued,
		MaxRetries: 2,
		CreatedAt:  time.Now(),
	}
	p.byID[job.ID] = job
	p.mu.Unlock()

	select {
	case p.jobs <- job:
	case <-p.ctx.Done():
	}
}

func (p *WorkerPool) JobStatus(jobID string) (*Job, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	job, ok := p.byID[jobID]
	if !ok {
		return nil, false
	}
	copied := *job
	return &copied, true
}

func (p *WorkerPool) Shutdown(ctx context.Context) error {
	p.cancel()
	done := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *WorkerPool) worker(id int) {
	defer p.wg.Done()
	p.logger.Debug("worker_started", slog.Int("worker_id", id))
	for {
		select {
		case <-p.ctx.Done():
			return
		case job, ok := <-p.jobs:
			if !ok {
				return
			}
			p.processJob(job)
		}
	}
}

func (p *WorkerPool) processJob(job *Job) {
	p.mu.Lock()
	job.Status = JobRunning
	p.mu.Unlock()

	if err := p.store.SetDocumentProcessing(job.DocumentID); err != nil {
		p.handleFailure(job, err.Error())
		return
	}

	var detections []document.RuntimeDetection
	var execErr error

	func() {
		defer func() {
			if r := recover(); r != nil {
				execErr = fmt.Errorf("worker panic: %v", r)
			}
		}()

		if p.detector == nil {
			detections = document.DetectRuntimeRedactions(job.text)
			return
		}
		detections, execErr = p.detector.Detect(p.ctx, job.DocumentID, job.text)
	}()

	if execErr != nil {
		p.handleFailure(job, execErr.Error())
		return
	}

	if err := p.store.SetDocumentProcessed(job.DocumentID, detections); err != nil {
		p.handleFailure(job, err.Error())
		return
	}

	p.mu.Lock()
	job.Status = JobSucceeded
	p.mu.Unlock()

	p.logger.Info("job_succeeded",
		slog.String("job_id", job.ID),
		slog.String("document_id", job.DocumentID),
		slog.Int("detections", len(detections)),
	)
}

func (p *WorkerPool) handleFailure(job *Job, errMsg string) {
	job.Attempts++
	job.Error = errMsg

	if job.Attempts <= job.MaxRetries {
		job.Status = JobQueued
		delay := time.Duration(job.Attempts) * time.Second
		p.logger.Warn("job_retry",
			slog.String("job_id", job.ID),
			slog.String("document_id", job.DocumentID),
			slog.Int("attempt", job.Attempts),
			slog.Duration("delay", delay),
			slog.String("error", errMsg),
		)
		time.AfterFunc(delay, func() {
			select {
			case p.jobs <- job:
			case <-p.ctx.Done():
			}
		})
	} else {
		job.Status = JobFailed
		p.store.SetDocumentFailed(job.DocumentID, errMsg)
		p.logger.Error("job_failed_permanent",
			slog.String("job_id", job.ID),
			slog.String("document_id", job.DocumentID),
			slog.Int("attempts", job.Attempts),
			slog.String("error", errMsg),
		)
	}
}
