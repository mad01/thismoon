# Profiling and Performance

## Exposing Profiles in a Server

```go
import (
    "net/http"
    _ "net/http/pprof" // registers /debug/pprof/* on DefaultServeMux
)

func main() {
    // Serve pprof on a private port, never the public listener.
    go func() {
        http.ListenAndServe("localhost:6060", nil)
    }()
    // ... run the real server
}
```

With a custom mux, register the handlers explicitly:

```go
import "net/http/pprof"

mux := http.NewServeMux()
mux.HandleFunc("/debug/pprof/", pprof.Index)
mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
mux.HandleFunc("/debug/pprof/heap", pprof.Index)
mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
```

## Profile Types

| Profile | Endpoint / flag | What it shows |
|---------|-----------------|---------------|
| CPU | `/debug/pprof/profile?seconds=30` | Where CPU time goes (sampled) |
| Heap | `/debug/pprof/heap` | Live allocations by call site |
| Allocs | `/debug/pprof/allocs` | All allocations since start |
| Goroutine | `/debug/pprof/goroutine` | Stack of every goroutine (leak hunting) |
| Block | `/debug/pprof/block` | Time blocked on channels/locks (needs `runtime.SetBlockProfileRate`) |
| Mutex | `/debug/pprof/mutex` | Contended mutex holders (needs `runtime.SetMutexProfileFraction`) |

## Reading a Profile

```bash
# Interactive: top, list <func>, web (needs graphviz)
go tool pprof http://localhost:6060/debug/pprof/profile?seconds=30

# Flame graph in the browser — usually the fastest path to insight
go tool pprof -http=:8080 cpu.out

# From a benchmark
go test -bench=BenchmarkParse -cpuprofile=cpu.out -memprofile=mem.out ./...
go tool pprof -http=:8080 mem.out
```

In the interactive prompt: `top10` ranks by flat cost, `cumtop` by cumulative,
`list funcName` annotates source lines, `peek funcName` shows callers/callees.
For heap profiles, switch between `-inuse_space` (live memory, leak hunting)
and `-alloc_space` (total churn, GC pressure).

## Benchmarks That Measure Allocations

```go
func BenchmarkParse(b *testing.B) {
    data := loadTestData(b)
    b.ReportAllocs() // report allocs/op and B/op
    b.ResetTimer()   // exclude setup from timing
    for b.Loop() {   // Go 1.24+; use `for i := 0; i < b.N; i++` before that
        Parse(data)
    }
}
```

Compare before/after with benchstat — never eyeball single runs:

```bash
go test -bench=. -count=10 ./... > old.txt
# ... apply the change ...
go test -bench=. -count=10 ./... > new.txt
benchstat old.txt new.txt   # reports delta with statistical significance
```

## Escape Analysis and Allocation Hunting

```bash
# Show which values escape to the heap and why
go build -gcflags="-m" ./... 2>&1 | grep escapes
```

Common escape causes: pointers to locals returned to callers (fine, but
heap), values stored in interfaces, closure captures that outlive the frame,
and slices that grow past their backing array. Fix the ones that show up hot
in the profile — not every escape matters.

Cheap wins that usually survive review:

```go
// Preallocate when the size is known
result := make([]Item, 0, len(input))

// Reuse buffers across iterations
var buf bytes.Buffer
for _, item := range items {
    buf.Reset()
    render(&buf, item)
}

// strconv over fmt for single values in hot paths
s := strconv.Itoa(n) // not fmt.Sprintf("%d", n)
```

`sync.Pool` is the last resort for high-churn temporary objects — profile
first; it adds complexity and helps only under real allocation pressure.

## Execution Traces

For latency questions the sampling profiler can't answer (scheduler delays,
GC pauses, goroutine blocking chains):

```bash
go test -bench=. -trace=trace.out ./...
go tool trace trace.out
```

## Quick Reference

| Task | Command |
|------|---------|
| CPU profile from a server | `go tool pprof http://localhost:6060/debug/pprof/profile?seconds=30` |
| Flame graph UI | `go tool pprof -http=:8080 profile.out` |
| Benchmark with profiles | `go test -bench=X -cpuprofile=cpu.out -memprofile=mem.out` |
| Compare benchmarks | `benchstat old.txt new.txt` (always `-count=10`) |
| Find heap escapes | `go build -gcflags="-m" ./...` |
| Goroutine leak check | `curl localhost:6060/debug/pprof/goroutine?debug=2` |
| Scheduler/GC latency | `go tool trace trace.out` |
