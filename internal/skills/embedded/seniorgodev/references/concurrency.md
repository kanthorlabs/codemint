# Concurrency

## Share by Communicating

> Do not communicate by sharing memory; instead, share memory by communicating.

## Goroutines

**WaitGroup.Go** (Go 1.25+):

```go
import "sync"

var wg sync.WaitGroup
for _, item := range items {
    wg.Go(func() {
        process(item)
    })
}
wg.Wait()
```

## Channels

```go
ch := make(chan int)        // unbuffered
ch := make(chan int, 100)   // buffered

ch <- value   // send
v := <-ch     // receive
v, ok := <-ch // receive with close check
close(ch)
```

## Select

```go
select {
case v := <-ch1:
    process(v)
case ch2 <- x:
    // sent
case <-ctx.Done():
    return ctx.Err()
default:
    // non-blocking
}
```

## Weak Pointers (Go 1.24+)

For memory-efficient caches:

```go
import "weak"

type Cache[K comparable, V any] struct {
    items map[K]weak.Pointer[V]
}
```
