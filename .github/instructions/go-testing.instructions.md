---
description: "Go testing conventions for scafctl-plugin-sdk: table-driven tests, testify/assert, benchmarks, race detection, and coverage. Use when writing or editing Go test files."
applyTo: "**/*_test.go"
---

# Go Testing Conventions

## Framework

- Use standard `go test` with **table-driven tests**
- Use `testify/assert` for assertions
- Place mocks in `mock.go` files

## Race Detection

Always run with the `-race` flag:

```bash
go test -race ./...
```

## Coverage

```bash
go test -cover ./...
```

### Coverage Targets

| Code Type | Target |
|-----------|--------|
| Core packages (`plugin/`, `provider/`, `auth/`) | 80%+ |
| Helpers (`schemahelper/`, `testutil/`) | 70%+ |
| Generated code (`proto/`) | Exclude |

### Patch Coverage

Every PR must have **70%+ patch coverage** (percentage of new/changed lines covered by tests).

- When adding new code, write tests for it in the same PR
- Never submit a new file with 0% coverage; at minimum test the happy path and one error path

## Verification

After any change to Go files, run the end-to-end task and confirm it passes:

```bash
task test:e2e
```

This runs `go vet`, `golangci-lint`, and all `TestIntegration_*` tests with the race detector.
Do not consider a change complete until `task test:e2e` exits 0.

## Benchmarks

Add benchmark tests for performance-sensitive code:

```go
func BenchmarkMyFeature(b *testing.B) {
    b.ReportAllocs()
    b.ResetTimer()

    for b.Loop() {
        // benchmark code
    }
}
```
