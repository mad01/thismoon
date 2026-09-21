# Concurrency Patterns

## Goroutine Lifecycle Management

```go
package main

import (
    "context"
    "fmt"
    "sync"
    "time"
)

// Worker pool with bounded concurrency
type WorkerPool struct {
    workers int
    tasks   chan func()
    wg      sync.WaitGroup
}

func NewWorkerPool(workers int) *WorkerPool {
    wp := &WorkerPool{
        workers: workers,
        tasks:   make(chan func(), workers*2), // Buffered channel
    }
    wp.start()
    return wp
}

func (wp *WorkerPool) start() {
    for i := 0; i < wp.workers; i++ {
        wp.wg.Add(1)
        go func() {
            defer wp.wg.Done()
            for task := range wp.tasks {
                task()
            }
        }()
    }
}

func (wp *WorkerPool) Submit(task func()) {
    wp.tasks <- task
}

func (wp *WorkerPool) Shutdown() {
    close(wp.tasks)
    wp.wg.Wait()
}
```

## Channel Patterns

```go
// Generator pattern
func generateNumbers(ctx context.Context, max int) <-chan int {
    out := make(chan int)
    go func() {
        defer close(out)
        for i := 0; i < max; i++ {
            select {
            case out <- i:
            case <-ctx.Done():
                return
            }
        }
    }()
    return out
}

// Fan-out, fan-in pattern
func fanOut(ctx context.Context, input <-chan int, workers int) []<-chan int {
    channels := make([]<-chan int, workers)
    for i := 0; i < workers; i++ {
        channels[i] = process(ctx, input)
    }
    return channels
}

func process(ctx context.Context, input <-chan int) <-chan int {
    out := make(chan int)
    go func() {
        defer close(out)
        for val := range input {
            select {
            case out <- val * 2:
            case <-ctx.Done():
                return
            }
        }
    }()
    return out
}

func fanIn(ctx context.Context, channels ...<-chan int) <-chan int {
    out := make(chan int)
    var wg sync.WaitGroup

    for _, ch := range channels {
        wg.Add(1)
        go func(c <-chan int) {
            defer wg.Done()
            for val := range c {
                select {
                case out <- val:
                case <-ctx.Done():
                    return
                }
            }
        }(ch)
    }

    go func() {
        wg.Wait()
        close(out)
    }()

    return out
}
```

## Select Statement Patterns

```go
// Timeout pattern
func fetchWithTimeout(ctx context.Context, url string) (string, error) {
    result := make(chan string, 1)
    errCh := make(chan error, 1)

    go func() {
        // Simulate network call
        time.Sleep(100 * time.Millisecond)
        result <- "data from " + url
    }()

    select {
    case res := <-result:
        return res, nil
    case err := <-errCh:
        return "", err
    case <-time.After(50 * time.Millisecond):
        return "", fmt.Errorf("timeout")
    case <-ctx.Done():
        return "", ctx.Err()
    }
}

// Done channel pattern for graceful shutdown
type Server struct {
    done chan struct{}
}

func (s *Server) Shutdown() {
    close(s.done)
}

func (s *Server) Run(ctx context.Context) {
    ticker := time.NewTicker(1 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-ticker.C:
            fmt.Println("tick")
        case <-s.done:
            fmt.Println("shutting down")
            return
        case <-ctx.Done():
            fmt.Println("context cancelled")
            return
        }
    }
}
```

## Sync Primitives

```go
import "sync"

// Mutex for protecting shared state
type Counter struct {
    mu    sync.Mutex
    count int
}

func (c *Counter) Increment() {
    c.mu.Lock()
    defer c.mu.Unlock()
    c.count++
}

func (c *Counter) Value() int {
    c.mu.Lock()
    defer c.mu.Unlock()
    return c.count
}

// RWMutex for read-heavy workloads
type Cache struct {
    mu    sync.RWMutex
    items map[string]string
}

func (c *Cache) Get(key string) (string, bool) {
    c.mu.RLock()
    defer c.mu.RUnlock()
    val, ok := c.items[key]
    return val, ok
}

func (c *Cache) Set(key, value string) {
    c.mu.Lock()
    defer c.mu.Unlock()
    c.items[key] = value
}

// sync.Once for initialization
type Service struct {
    once   sync.Once
    config *Config
}

func (s *Service) getConfig() *Config {
    s.once.Do(func() {
        s.config = loadConfig() // Only called once
    })
    return s.config
}
```

## Rate Limiting and Backpressure

```go
import "golang.org/x/time/rate"

// Token bucket rate limiter
type RateLimiter struct {
    limiter *rate.Limiter
}

func NewRateLimiter(rps int) *RateLimiter {
    return &RateLimiter{
        limiter: rate.NewLimiter(rate.Limit(rps), rps),
    }
}

func (rl *RateLimiter) Process(ctx context.Context, item string) error {
    if err := rl.limiter.Wait(ctx); err != nil {
        return err
    }
    // Process item
    return nil
}

// Semaphore pattern for limiting concurrency
type Semaphore struct {
    slots chan struct{}
}

func NewSemaphore(n int) *Semaphore {
    return &Semaphore{
        slots: make(chan struct{}, n),
    }
}

func (s *Semaphore) Acquire() {
    s.slots <- struct{}{}
}

func (s *Semaphore) Release() {
    <-s.slots
}

func (s *Semaphore) Do(fn func()) {
    s.Acquire()
    defer s.Release()
    fn()
}
```

## Pipeline Pattern

```go
// Stage-based processing pipeline
func pipeline(ctx context.Context, input <-chan int) <-chan int {
    // Stage 1: Square numbers
    stage1 := make(chan int)
    go func() {
        defer close(stage1)
        for num := range input {
            select {
            case stage1 <- num * num:
            case <-ctx.Done():
                return
            }
        }
    }()

    // Stage 2: Filter even numbers
    stage2 := make(chan int)
    go func() {
        defer close(stage2)
        for num := range stage1 {
            if num%2 == 0 {
                select {
                case stage2 <- num:
                case <-ctx.Done():
                    return
                }
            }
        }
    }()

    return stage2
}
```

## Errgroup: Structured Concurrency

`golang.org/x/sync/errgroup` is the default for "run N things, fail together,
wait for all" — prefer it over hand-rolled `WaitGroup` + error channel:

```go
import "golang.org/x/sync/errgroup"

func fetchAll(ctx context.Context, urls []string) ([]Result, error) {
    g, ctx := errgroup.WithContext(ctx) // ctx cancels on first error
    results := make([]Result, len(urls))

    g.SetLimit(8) // bounded concurrency without a semaphore

    for i, url := range urls {
        g.Go(func() error {
            r, err := fetch(ctx, url)
            if err != nil {
                return fmt.Errorf("fetch %s: %w", url, err)
            }
            results[i] = r // indexed write: no shared-slice race
            return nil
        })
    }
    if err := g.Wait(); err != nil {
        return nil, err
    }
    return results, nil
}
```

Key properties: first error cancels the group's context, `Wait` returns that
first error, `SetLimit` caps in-flight goroutines, and writing to distinct
indexes of a preallocated slice needs no mutex.

## Atomics vs Mutexes

Default to a mutex — it is harder to misuse and just as fast outside hot
paths. Reach for `sync/atomic` only for single-word counters, flags, and
lock-free reads of a swappable value. Use the typed wrappers, never raw
`atomic.AddInt64` on a plain field:

```go
type Metrics struct {
    requests atomic.Int64
    healthy  atomic.Bool
}

func (m *Metrics) Record() { m.requests.Add(1) }
func (m *Metrics) Total() int64 { return m.requests.Load() }

// Swappable config: readers never block
var current atomic.Pointer[Config]
current.Store(newCfg)          // writer
cfg := current.Load()          // any reader, lock-free
```

Never mix atomic and non-atomic access to the same field, and don't build
multi-field invariants out of atomics — the moment two values must change
together, that's a mutex. The race detector does not model the Go memory
model's happens-before edges for you; run `-race` regardless.

## Quick Reference

| Pattern | Use Case | Key Points |
|---------|----------|------------|
| Errgroup | Run N tasks, fail together | First error cancels ctx; `SetLimit` bounds |
| Atomic types | Counters, flags, swap-a-pointer | Typed wrappers; single word only |
| Worker Pool | Bounded concurrency | Limit goroutines, reuse workers |
| Fan-out/Fan-in | Parallel processing | Distribute work, merge results |
| Pipeline | Stream processing | Chain transformations |
| Rate Limiter | API throttling | Control request rate |
| Semaphore | Resource limits | Cap concurrent operations |
| Done Channel | Graceful shutdown | Signal completion |
