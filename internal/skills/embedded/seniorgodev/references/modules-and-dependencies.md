# Modules and Dependencies

## Module Initialization

```bash
go mod init example.com/mymodule
```

## go.mod File

```go
module example.com/myproject

go 1.26

require (
    github.com/pkg/errors v0.9.1
)
```

## Ignore Directive (Go 1.25+)

Exclude directories from package patterns:

```go
ignore (
    testdata
    examples
)
```

## Tool Dependencies (Go 1.24+)

```bash
go get -tool github.com/golangci/golangci-lint
go install tool
```

## go.work for Workspaces

```go
go 1.26

use (
    ./api
    ./web
    ./shared
)
```
