# Effective Go 2026

## Table of Contents

- [Introduction](#introduction)
- [Formatting](#formatting)
- [Commentary](#commentary)
- [Names](#names)
- [Semicolons](#semicolons)
- [Control Structures](#control-structures)
- [Functions](#functions)
- [Data](#data)
- [Methods](#methods)
- [Interfaces and Types](#interfaces-and-types)
- [Generics](#generics)
- [Iterators](#iterators)
- [The Blank Identifier](#the-blank-identifier)
- [Embedding](#embedding)
- [Concurrency](#concurrency)
- [Errors](#errors)
- [Panic and Recover](#panic-and-recover)
- [Modules and Dependencies](#modules-and-dependencies)
- [Testing](#testing)
- [Security and Cryptography](#security-and-cryptography)
- [Performance](#performance)

---

## Introduction

This document provides guidelines for writing clear, idiomatic Go code as of Go 1.26 (February 2026). It builds upon the original "Effective Go" while incorporating language changes, standard library additions, and best practices developed since Go's introduction of generics (Go 1.18) through the latest releases.

Go emphasizes simplicity, reliability, and efficiency. Understanding Go's properties and idioms is essential for writing effective programs. A straightforward translation from other languages is unlikely to produce satisfactory Go code.

Prerequisites: [Language Specification](https://go.dev/ref/spec), [Tour of Go](https://go.dev/tour/), and [How to Write Go Code](https://go.dev/doc/code).

---

## Formatting

Formatting is handled by `gofmt` (or `go fmt` at package level). Let the machine handle indentation, alignment, and spacing.

Key points:
- **Tabs for indentation** - `gofmt` emits tabs by default
- **No line length limit** - wrap long lines with extra tab indent
- **Fewer parentheses** - control structures (`if`, `for`, `switch`) have no parentheses in syntax

```go
// gofmt handles alignment automatically
type Config struct {
    Timeout     time.Duration
    MaxRetries  int
    EnableDebug bool
}
```

---

## Commentary

Go uses C-style `/* */` block comments and C++ style `//` line comments. Line comments are the norm.

Doc comments appear before top-level declarations with no intervening newlines:

```go
// Package math provides basic constants and mathematical functions.
package math

// Pi is the ratio of a circle's circumference to its diameter.
const Pi = 3.14159265358979323846
```

For detailed guidance, see [Go Doc Comments](https://go.dev/doc/comment).

---

## Names

### Package Names

Package names should be lowercase, single-word, concise, and evocative. The package name becomes the accessor for contents:

```go
import "encoding/json"
json.Marshal(v)  // Not encoding_json.Marshal
```

Avoid repetition - `bufio.Reader`, not `bufio.BufReader`.

### Getters and Setters

No automatic support for getters/setters. If you have field `owner`, getter is `Owner()`, setter is `SetOwner()`:

```go
func (c *Config) Timeout() time.Duration { return c.timeout }
func (c *Config) SetTimeout(d time.Duration) { c.timeout = d }
```

### Interface Names

One-method interfaces use method name + `-er` suffix: `Reader`, `Writer`, `Stringer`, `Marshaler`.

### MixedCaps

Use `MixedCaps` or `mixedCaps`, not underscores.

---

## Semicolons

Semicolons are inserted automatically by the lexer. Never put opening brace on new line:

```go
// Correct
if x > 0 {
    return x
}

// Wrong - semicolon inserted before brace
if x > 0
{
    return x
}
```

---

## Control Structures

### If

Accept initialization statement:

```go
if err := file.Chmod(0664); err != nil {
    return err
}
```

Avoid unnecessary else when body ends in `return`:

```go
f, err := os.Open(name)
if err != nil {
    return err
}
// continue with f
```

### For

Three forms, no `while` or `do-while`:

```go
for init; condition; post { }  // C-style
for condition { }              // while
for { }                        // infinite
```

**Range over integers** (Go 1.22+):

```go
for i := range 10 {
    fmt.Println(i)  // 0 through 9
}
```

**Range over slices/maps**:

```go
for key, value := range myMap {
    fmt.Println(key, value)
}

for i, v := range slice {
    fmt.Println(i, v)
}

// Value only
for _, v := range slice {
    process(v)
}
```

**Loop variable semantics** (Go 1.22+): Loop variables are created fresh each iteration, preventing closure capture bugs:

```go
// Safe in Go 1.22+
for _, v := range values {
    go func() {
        fmt.Println(v)  // Each goroutine sees its own v
    }()
}
```

### Switch

More flexible than C - expressions need not be constants:

```go
switch {
case x < 0:
    return -1
case x > 0:
    return 1
default:
    return 0
}
```

No automatic fallthrough. Use comma for multiple cases:

```go
switch c {
case ' ', '\t', '\n':
    return true
}
```

### Type Switch

```go
switch v := x.(type) {
case string:
    return v
case int:
    return strconv.Itoa(v)
default:
    return fmt.Sprint(v)
}
```

---

## Functions

### Multiple Return Values

```go
func divide(a, b int) (int, error) {
    if b == 0 {
        return 0, errors.New("division by zero")
    }
    return a / b, nil
}
```

### Named Result Parameters

```go
func ReadFull(r io.Reader, buf []byte) (n int, err error) {
    for len(buf) > 0 && err == nil {
        var nr int
        nr, err = r.Read(buf)
        n += nr
        buf = buf[nr:]
    }
    return
}
```

### Defer

Schedules function call for when surrounding function returns. Arguments evaluated when defer executes:

```go
func Contents(filename string) (string, error) {
    f, err := os.Open(filename)
    if err != nil {
        return "", err
    }
    defer f.Close()
    
    data, err := io.ReadAll(f)
    return string(data), err
}
```

Deferred functions execute in LIFO order:

```go
for i := 0; i < 3; i++ {
    defer fmt.Print(i)
}
// Prints: 2 1 0
```

---

## Data

### Allocation with new

`new(T)` allocates zeroed storage, returns `*T`.

**Go 1.26+**: `new` accepts value expression for initialization:

```go
// Traditional
age := new(int)
*age = 25

// Go 1.26+
age := new(25)        // *int pointing to 25
name := new("Alice")  // *string pointing to "Alice"

// Useful for struct fields
type Person struct {
    Name string
    Age  *int
}
p := Person{Name: "Bob", Age: new(30)}
```

### Allocation with make

`make(T, args)` creates slices, maps, and channels only. Returns initialized value of type `T` (not `*T`):

```go
slice := make([]int, 10, 100)  // len=10, cap=100
m := make(map[string]int)
ch := make(chan int, 10)       // buffered
```

### Composite Literals

```go
return &File{fd: fd, name: name}

// Slices and maps
s := []string{"one", "two", "three"}
m := map[string]int{"one": 1, "two": 2}
```

### Slices

Slices reference underlying arrays:

```go
func (f *File) Read(buf []byte) (n int, err error)

data := make([]byte, 100)
n, err := f.Read(data[0:50])  // Read into first 50 bytes
```

Use `append` for growing:

```go
slice = append(slice, elem1, elem2)
slice = append(slice, otherSlice...)
```

**Concatenate slices** (Go 1.22+):

```go
import "slices"
combined := slices.Concat(slice1, slice2, slice3)
```

### Maps

```go
m := make(map[string]int)
m["key"] = 42
value := m["key"]

// Check existence
value, ok := m["key"]
if !ok {
    // key not present
}

// Delete
delete(m, "key")
```

**Clear all entries** (Go 1.23+):

```go
import "sync"
var m sync.Map
m.Clear()
```

---

## Methods

### Pointer vs Value Receivers

Value methods can be invoked on pointers and values. Pointer methods can only be invoked on pointers:

```go
type Counter int

func (c Counter) Value() int    { return int(c) }
func (c *Counter) Increment()   { *c++ }

var c Counter
c.Increment()      // Compiler rewrites to (&c).Increment()
fmt.Println(c.Value())
```

Use pointer receiver when:
- Method modifies receiver
- Receiver is large struct
- Consistency with other methods

---

## Interfaces and Types

Interfaces specify behavior. Types implement interfaces implicitly:

```go
type Reader interface {
    Read(p []byte) (n int, err error)
}

type Writer interface {
    Write(p []byte) (n int, err error)
}

// Compose interfaces
type ReadWriter interface {
    Reader
    Writer
}
```

### Type Assertions

```go
str, ok := value.(string)
if !ok {
    // value is not a string
}
```

### Interface Checks

Compile-time verification:

```go
var _ json.Marshaler = (*MyType)(nil)
```

---

## Generics

Go 1.18+ supports type parameters on functions and types.

### Generic Functions

```go
func Min[T cmp.Ordered](a, b T) T {
    if a < b {
        return a
    }
    return b
}

result := Min(3, 5)           // T inferred as int
result := Min[float64](3, 5)  // Explicit type
```

### Generic Types

```go
type Stack[T any] struct {
    items []T
}

func (s *Stack[T]) Push(item T) {
    s.items = append(s.items, item)
}

func (s *Stack[T]) Pop() (T, bool) {
    if len(s.items) == 0 {
        var zero T
        return zero, false
    }
    item := s.items[len(s.items)-1]
    s.items = s.items[:len(s.items)-1]
    return item, true
}
```

### Generic Type Aliases (Go 1.24+)

```go
type MyMap[K comparable, V any] = map[K]V
type StringMap[V any] = MyMap[string, V]

m := StringMap[int]{"one": 1, "two": 2}
```

### Self-Referential Generics (Go 1.26+)

```go
type Adder[A Adder[A]] interface {
    Add(A) A
}

type Vector2D struct{ X, Y float64 }

func (v Vector2D) Add(other Vector2D) Vector2D {
    return Vector2D{v.X + other.X, v.Y + other.Y}
}

func Sum[A Adder[A]](items ...A) A {
    var result A
    for _, item := range items {
        result = result.Add(item)
    }
    return result
}
```

### Constraints

Common constraints from `constraints` and `cmp` packages:

```go
import "cmp"

func Max[T cmp.Ordered](values ...T) T {
    m := values[0]
    for _, v := range values[1:] {
        if v > m {
            m = v
        }
    }
    return m
}
```

---

## Iterators

Go 1.23+ supports range-over-function iterators.

### Iterator Function Signatures

```go
func(yield func() bool)           // No values
func(yield func(V) bool)          // Single value
func(yield func(K, V) bool)       // Key-value pairs
```

### Using Iterators

```go
import "slices"

// Iterate over slice values
for v := range slices.Values(mySlice) {
    fmt.Println(v)
}

// Iterate backward
for i, v := range slices.Backward(mySlice) {
    fmt.Println(i, v)
}

// Iterate over map keys
import "maps"
for k := range maps.Keys(myMap) {
    fmt.Println(k)
}
```

### String/Bytes Iterators (Go 1.24+)

```go
import "strings"

for line := range strings.Lines(text) {
    process(line)
}

for field := range strings.FieldsSeq(text) {
    process(field)
}
```

### Creating Custom Iterators

```go
func Fibonacci(n int) iter.Seq[int] {
    return func(yield func(int) bool) {
        a, b := 0, 1
        for i := 0; i < n; i++ {
            if !yield(a) {
                return
            }
            a, b = b, a+b
        }
    }
}

for v := range Fibonacci(10) {
    fmt.Println(v)
}
```

### Collecting from Iterators

```go
import "slices"

values := slices.Collect(slices.Values(mySlice))
sorted := slices.Sorted(maps.Keys(myMap))
```

### Reflection Iterators (Go 1.26+)

```go
import "reflect"

t := reflect.TypeOf(MyStruct{})
for field := range t.Fields() {
    fmt.Println(field.Name, field.Type)
}

for method := range t.Methods() {
    fmt.Println(method.Name)
}
```

---

## The Blank Identifier

Discard unwanted values:

```go
_, err := io.Copy(dst, src)

for _, v := range slice { }
```

### Import for Side Effect

```go
import _ "net/http/pprof"
```

### Interface Satisfaction Check

```go
var _ io.Reader = (*MyReader)(nil)
```

---

## Embedding

### Struct Embedding

```go
type ReadWriter struct {
    *bufio.Reader
    *bufio.Writer
}
// ReadWriter has methods of both Reader and Writer
```

### Interface Embedding

```go
type ReadWriteCloser interface {
    io.Reader
    io.Writer
    io.Closer
}
```

---

## Concurrency

### Share by Communicating

> Do not communicate by sharing memory; instead, share memory by communicating.

### Goroutines

Lightweight concurrent functions:

```go
go processItem(item)

go func() {
    // inline function
}()
```

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

### Channels

```go
ch := make(chan int)        // unbuffered
ch := make(chan int, 100)   // buffered

ch <- value   // send
v := <-ch     // receive
v, ok := <-ch // receive with close check
close(ch)
```

### Select

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

### Timer and Ticker (Go 1.23+)

Timers and Tickers are garbage collected when no longer referenced:

```go
timer := time.NewTimer(time.Second)
// No need to call Stop() for cleanup
// Channel is unbuffered - no stale values after Reset/Stop
```

### Weak Pointers (Go 1.24+)

For memory-efficient caches:

```go
import "weak"

type Cache[K comparable, V any] struct {
    items map[K]weak.Pointer[V]
}

func (c *Cache[K, V]) Get(key K) (V, bool) {
    if ptr, ok := c.items[key]; ok {
        if v := ptr.Value(); v != nil {
            return *v, true
        }
        delete(c.items, key)
    }
    var zero V
    return zero, false
}
```

### Value Canonicalization (Go 1.23+)

```go
import "unique"

handle1 := unique.Make("hello")
handle2 := unique.Make("hello")
// handle1 == handle2 (pointer comparison)
```

---

## Errors

### Error Interface

```go
type error interface {
    Error() string
}
```

### Creating Errors

```go
import "errors"

var ErrNotFound = errors.New("not found")

func process(id int) error {
    if id < 0 {
        return fmt.Errorf("invalid id: %d", id)
    }
    return nil
}
```

### Error Wrapping

```go
if err != nil {
    return fmt.Errorf("processing failed: %w", err)
}

// Unwrap
if errors.Is(err, ErrNotFound) { }

var pathErr *os.PathError
if errors.As(err, &pathErr) {
    fmt.Println(pathErr.Path)
}
```

### Sentinel Errors

```go
var (
    ErrNotFound    = errors.New("not found")
    ErrPermission  = errors.New("permission denied")
)
```

---

## Panic and Recover

Use panic only for unrecoverable errors:

```go
func init() {
    if config == nil {
        panic("configuration not loaded")
    }
}
```

Recover in deferred functions:

```go
func safeCall(fn func()) (err error) {
    defer func() {
        if r := recover(); r != nil {
            err = fmt.Errorf("panic: %v", r)
        }
    }()
    fn()
    return nil
}
```

---

## Modules and Dependencies

### Module Initialization

```bash
go mod init example.com/mymodule
```

### go.mod File

```go
module example.com/myproject

go 1.26

require (
    github.com/pkg/errors v0.9.1
)
```

### Ignore Directive (Go 1.25+)

Exclude directories from package patterns:

```go
module example.com/myproject

ignore (
    testdata
    examples
)
```

### Tool Dependencies (Go 1.24+)

```bash
go get -tool github.com/golangci/golangci-lint
go install tool
```

### go.work for Workspaces

```go
go 1.26

use (
    ./api
    ./web
    ./shared
)
```

---

## Testing

### Basic Tests

```go
func TestAdd(t *testing.T) {
    got := Add(2, 3)
    want := 5
    if got != want {
        t.Errorf("Add(2, 3) = %d; want %d", got, want)
    }
}
```

### Table-Driven Tests

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

### Benchmarks

```go
func BenchmarkProcess(b *testing.B) {
    for b.Loop() {  // Go 1.24+ - preferred
        Process(data)
    }
}

// Alternative
func BenchmarkProcess(b *testing.B) {
    for i := 0; i < b.N; i++ {
        Process(data)
    }
}
```

### Testing Concurrent Code (Go 1.25+)

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

### Test Artifacts (Go 1.26+)

```go
func TestGenerate(t *testing.T) {
    result := Generate()
    
    dir := t.ArtifactDir()
    os.WriteFile(filepath.Join(dir, "output.txt"), result, 0644)
}
```

---

## Security and Cryptography

### Secure Random

```go
import "crypto/rand"

// Random bytes
buf := make([]byte, 32)
rand.Read(buf)

// Random text (Go 1.24+)
text := rand.Text()
```

### Hashing

```go
import "crypto/sha256"

h := sha256.New()
h.Write(data)
sum := h.Sum(nil)

// One-shot
sum := sha256.Sum256(data)
```

### Post-Quantum Cryptography (Go 1.24+)

```go
import "crypto/mlkem"

// ML-KEM key encapsulation
pub, priv, _ := mlkem.GenerateKey768()
ciphertext, sharedKey := pub.Encapsulate()
decryptedKey := priv.Decapsulate(ciphertext)
```

### TLS Configuration

```go
import "crypto/tls"

config := &tls.Config{
    MinVersion: tls.VersionTLS13,
    // Post-quantum enabled by default in Go 1.26
}
```

### HPKE (Go 1.26+)

```go
import "crypto/hpke"

// Hybrid Public Key Encryption
suite := hpke.NewSuite(hpke.KEM_X25519, hpke.KDF_HKDF_SHA256, hpke.AEAD_AES128GCM)
sender, enc := suite.NewSender(recipientPub, info)
ciphertext := sender.Seal(plaintext, aad)
```

### FIPS 140-3 Mode (Go 1.24+)

```bash
GOFIPS140=v1.0.0 go build
GODEBUG=fips140=1 ./mybinary
```

---

## Performance

### Profiling

```go
import "runtime/pprof"

f, _ := os.Create("cpu.prof")
pprof.StartCPUProfile(f)
defer pprof.StopCPUProfile()
```

### Flight Recorder (Go 1.25+)

```go
import "runtime/trace"

fr := new(trace.FlightRecorder)
fr.Start()
// ... run application ...
f, _ := os.Create("trace.out")
fr.WriteTo(f)  // Capture last N seconds
```

### Memory Efficiency

Use `sync.Pool` for frequently allocated objects:

```go
var bufPool = sync.Pool{
    New: func() any {
        return new(bytes.Buffer)
    },
}

func process() {
    buf := bufPool.Get().(*bytes.Buffer)
    defer bufPool.Put(buf)
    buf.Reset()
    // use buf
}
```

### JSON Performance (Go 1.25+)

Enable experimental v2 encoder:

```bash
GOEXPERIMENT=jsonv2 go build
```

### Green Tea GC (Go 1.26)

Enabled by default. 10-40% reduction in GC overhead:

```bash
# Disable if needed
GOEXPERIMENT=nogreenteagc go build
```

### Container-Aware GOMAXPROCS (Go 1.25+)

Runtime respects cgroup CPU limits automatically on Linux.

### Goroutine Leak Detection (Go 1.26+)

```bash
GOEXPERIMENT=goroutineleakprofile go build
```

Access via `/debug/pprof/goroutineleak`.

---

## Best Practices Summary

1. **Format with gofmt** - always
2. **Handle all errors** - never ignore
3. **Use short variable names** - especially for locals with small scope
4. **Accept interfaces, return concrete types** - for flexibility
5. **Use generics judiciously** - when type safety adds value, not for every function
6. **Prefer composition over inheritance** - via embedding
7. **Channel-based communication** - for coordinating goroutines
8. **Context for cancellation** - pass `context.Context` as first parameter
9. **Test thoroughly** - table-driven tests, synctest for concurrency
10. **Profile before optimizing** - measure, don't guess

---

## Version History

| Version | Release Date | Key Features |
|---------|-------------|--------------|
| Go 1.22 | Feb 2024 | Loop variable fix, range integers, enhanced http routing, math/rand/v2 |
| Go 1.23 | Aug 2024 | Iterators (range-over-func), unique package, timer GC |
| Go 1.24 | Feb 2025 | Generic type aliases, weak pointers, synctest, FIPS 140-3, post-quantum TLS |
| Go 1.25 | Aug 2025 | WaitGroup.Go, FlightRecorder, json/v2, greenteagc (experimental) |
| Go 1.26 | Feb 2026 | new() with value, self-referential generics, HPKE, SIMD, greenteagc default |

---

*Last updated: April 2026 for Go 1.26*
