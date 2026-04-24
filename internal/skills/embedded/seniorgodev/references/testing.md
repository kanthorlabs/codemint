# Testing

## Table-Driven Tests

```go
func TestAdd(t *testing.T) {
    tests := []struct {
        name string
        a, b int
        want int
    }{
        {"positive", 2, 3, 5},
        {"negative", -1, -2, -3},
        {"zero", 0, 0, 0},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            if got := Add(tt.a, tt.b); got != tt.want {
                t.Errorf("Add(%d, %d) = %d; want %d", tt.a, tt.b, got, tt.want)
            }
        })
    }
}
```

## Benchmarks

```go
func BenchmarkProcess(b *testing.B) {
    for b.Loop() {  // Go 1.24+ - preferred
        Process(data)
    }
}
```

## Testing Concurrent Code (Go 1.25+)

```go
import "testing/synctest"

func TestConcurrent(t *testing.T) {
    synctest.Test(t, func(t *testing.T) {
        ch := make(chan int)
        go func() {
            ch <- 42
        }()

        synctest.Wait()  // Wait for goroutines to block

        v := <-ch
        if v != 42 {
            t.Errorf("got %d, want 42", v)
        }
    })
}
```

## Test Artifacts (Go 1.26+)

```go
func TestGenerate(t *testing.T) {
    result := Generate()

    dir := t.ArtifactDir()
    os.WriteFile(filepath.Join(dir, "output.txt"), result, 0644)
}
```
