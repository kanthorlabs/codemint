---
name: seniorgodev
description: Senior Go developer skill. Code review, refactoring, testing, performance optimization following Effective Go 2026 guidelines. Use for Go projects requiring idiomatic, production-quality code.
compatibility: Requires Go 1.22+, staticcheck, gofmt
metadata:
  author: kanthorlabs
  version: "1.0"
  go-version: "1.26"
---

# Senior Go Developer

Expert Go development following Effective Go 2026 best practices.

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

## References

Detailed guidelines by topic:

- [Introduction](references/introduction.md)
- [Formatting](references/formatting.md)
- [Commentary](references/commentary.md)
- [Names](references/names.md)
- [Semicolons](references/semicolons.md)
- [Control Structures](references/control-structures.md)
- [Functions](references/functions.md)
- [Data](references/data.md)
- [Methods](references/methods.md)
- [Interfaces and Types](references/interfaces-and-types.md)
- [Generics](references/generics.md)
- [Iterators](references/iterators.md)
- [Blank Identifier](references/blank-identifier.md)
- [Embedding](references/embedding.md)
- [Concurrency](references/concurrency.md)
- [Errors](references/errors.md)
- [Panic and Recover](references/panic-and-recover.md)
- [Modules and Dependencies](references/modules-and-dependencies.md)
- [Testing](references/testing.md)
- [Security and Cryptography](references/security-and-cryptography.md)
- [Performance](references/performance.md)
- [Version History](references/version-history.md)

## Available Scripts

- `scripts/lint.sh` - Run gofmt, go vet, staticcheck
- `scripts/test.sh` - Run tests with coverage report
