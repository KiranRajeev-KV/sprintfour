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

// WorkerPool processes document detection jobs with bounded concurrency.
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

// NewWorkerPool constructs a worker pool for document processing jobs.
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
	p.updateJob(job.ID, func(current *Job) {
		current.Status = JobRunning
	})

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

	p.updateJob(job.ID, func(current *Job) {
		current.Status = JobSucceeded
		current.Error = ""
	})

	p.logger.Info("job_succeeded",
		slog.String("job_id", job.ID),
		slog.String("document_id", job.DocumentID),
		slog.Int("detections", len(detections)),
	)
}

func (p *WorkerPool) handleFailure(job *Job, errMsg string) {
	current := p.updateJob(job.ID, func(current *Job) {
		current.Attempts++
		current.Error = errMsg
		if current.Attempts <= current.MaxRetries {
			current.Status = JobQueued
			return
		}
		current.Status = JobFailed
	})

	if current.Attempts <= current.MaxRetries {
		delay := time.Duration(current.Attempts) * time.Second
		p.logger.Warn("job_retry",
			slog.String("job_id", current.ID),
			slog.String("document_id", current.DocumentID),
			slog.Int("attempt", current.Attempts),
			slog.Duration("delay", delay),
			slog.String("error", errMsg),
		)
		time.AfterFunc(delay, func() {
			select {
			case p.jobs <- job:
			case <-p.ctx.Done():
			}
		})
		return
	}

	p.store.SetDocumentFailed(current.DocumentID, errMsg)
	p.logger.Error("job_failed_permanent",
		slog.String("job_id", current.ID),
		slog.String("document_id", current.DocumentID),
		slog.Int("attempts", current.Attempts),
		slog.String("error", errMsg),
	)
}

func (p *WorkerPool) updateJob(jobID string, mutate func(*Job)) Job {
	p.mu.Lock()
	defer p.mu.Unlock()

	job, ok := p.byID[jobID]
	if !ok {
		return Job{}
	}
	mutate(job)
	return *job
}
