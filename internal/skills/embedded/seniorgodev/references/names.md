# Names

## Package Names

Package names should be lowercase, single-word, concise, and evocative. The package name becomes the accessor for contents:

```go
import "encoding/json"
json.Marshal(v)  // Not encoding_json.Marshal
```

Avoid repetition - `bufio.Reader`, not `bufio.BufReader`.

## Getters and Setters

No automatic support for getters/setters. If you have field `owner`, getter is `Owner()`, setter is `SetOwner()`:

```go
func (c *Config) Timeout() time.Duration { return c.timeout }
func (c *Config) SetTimeout(d time.Duration) { c.timeout = d }
```

## Interface Names

One-method interfaces use method name + `-er` suffix: `Reader`, `Writer`, `Stringer`, `Marshaler`.

## MixedCaps

Use `MixedCaps` or `mixedCaps`, not underscores.
