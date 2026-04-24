# Performance

## Profiling

```go
import "runtime/pprof"

f, _ := os.Create("cpu.prof")
pprof.StartCPUProfile(f)
defer pprof.StopCPUProfile()
```

## Flight Recorder (Go 1.25+)

```go
import "runtime/trace"

fr := new(trace.FlightRecorder)
fr.Start()
// ... run application ...
f, _ := os.Create("trace.out")
fr.WriteTo(f)  // Capture last N seconds
```

## Memory Efficiency

Use `sync.Pool` for frequently allocated objects:

```go
var bufPool = sync.Pool{
    New: func() any {
        return new(bytes.Buffer)
    },
}
```

## Green Tea GC (Go 1.26)

Enabled by default. 10-40% reduction in GC overhead.

## Container-Aware GOMAXPROCS (Go 1.25+)

Runtime respects cgroup CPU limits automatically on Linux.

## Goroutine Leak Detection (Go 1.26+)

```bash
GOEXPERIMENT=goroutineleakprofile go build
```

Access via `/debug/pprof/goroutineleak`.
