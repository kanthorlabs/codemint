# Generics

Go 1.18+ supports type parameters on functions and types.

## Generic Functions

```go
func Min[T cmp.Ordered](a, b T) T {
    if a < b {
        return a
    }
    return b
}
```

## Generic Types

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

## Generic Type Aliases (Go 1.24+)

```go
type MyMap[K comparable, V any] = map[K]V
type StringMap[V any] = MyMap[string, V]
```

## Self-Referential Generics (Go 1.26+)

```go
type Adder[A Adder[A]] interface {
    Add(A) A
}
```
