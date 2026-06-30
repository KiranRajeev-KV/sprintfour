import { cn } from "#/lib/utils.ts"

function Progress({
  className,
  value,
  ...props
}: React.ComponentProps<"div"> & { value?: number }) {
  return (
    <div
      data-slot="progress"
      data-value={value}
      className={cn(
        "relative h-2 w-full overflow-hidden rounded-full bg-black/10",
        className,
      )}
      {...props}
    >
      <div
        className="h-full w-full flex-1 rounded-full bg-[var(--lagoon-deep)] transition-all duration-500 ease-out"
        style={{ transform: `translateX(-${100 - (value ?? 0)}%)` }}
      />
    </div>
  )
}

export { Progress }
