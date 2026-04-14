---
description: "scafctl-plugin-sdk: Run Go code review on recent changes. Checks for idiomatic Go, security, error handling, concurrency, and SDK conventions."
agent: "go-reviewer"
---
Review the current Go code changes thoroughly. You MUST complete ALL phases below. Do not stop after finding a few issues.

## Phase 1: Automated checks

1. Run `go vet ./...`
2. Run `git diff --stat HEAD -- '*.go'` and `git status --short` to identify all changed/new files
3. Read the full diff for all changed files
4. Read the full contents of all new (untracked) files
5. Run `go test -coverprofile` on **every** changed package
6. Run `go test -race` on changed packages

## Phase 2: Systematic review (check EVERY item)

For each changed/new file, check ALL of these categories. Do not skip any.

### Security
- [ ] Hardcoded secrets, tokens, or credentials
- [ ] Unsafe deserialization of untrusted input
- [ ] Race conditions (shared state without synchronization)

### Error handling
- [ ] Ignored errors (unchecked error returns, `_ = someFunc()`)
- [ ] Missing error wrapping (`fmt.Errorf("context: %w", err)`)
- [ ] Panics used for recoverable errors

### Concurrency
- [ ] Goroutine leaks (goroutines that never exit)
- [ ] Race conditions (shared state without synchronization)
- [ ] Deadlock potential (inconsistent lock ordering)

### Code quality
- [ ] Functions over 60 lines (flag, suggest extraction)
- [ ] Nesting depth over 4 levels
- [ ] Non-idiomatic Go patterns

### SDK conventions
- [ ] Struct tags: JSON/YAML tags present on exported structs
- [ ] No heavy dependencies added (CEL, OpenTelemetry, Cobra)
- [ ] Plugin-side code only (no host-side logic)
- [ ] Interface changes are intentional and documented as breaking
- [ ] Logging uses `logr.FromContextOrDiscard(ctx)`
- [ ] No magic values (use constants)

### Proto/gRPC
- [ ] Proto field numbers not changed (only additions)
- [ ] Generated code regenerated after proto changes
- [ ] Proto changes are backward compatible

### Correctness
- [ ] Edge cases: nil inputs, empty slices, zero values handled
- [ ] Map iteration: output built from map ranges must sort keys for deterministic ordering
- [ ] `defer cancel()` placed immediately after context creation, before any early returns

### Dead code
- [ ] New exported functions have callers outside test files (use `grep` to verify)
- [ ] New struct fields are read/written somewhere (use `grep` to verify)

## Phase 3: Coverage analysis

1. Run `go test -coverprofile=cover.out ./path/to/changed/pkg/...` for each changed package
2. Run `go tool cover -func=cover.out` to get per-function coverage
3. Flag any changed function with coverage below 70%
4. Flag any NEW file with overall coverage below 70%

## Phase 4: Self-review (MANDATORY)

After completing phases 1-3, review your own findings:
1. Re-read the full diff one more time
2. For each file you reviewed, ask: "What did I NOT check?"
3. Check: did you verify every item in the Phase 2 checklist? If you skipped any, go back now.

## Output format

Use severity levels: CRITICAL > HIGH > MEDIUM > LOW > INFO
For each finding include: file, line, severity, description, and suggested fix.
End with a summary table: files reviewed, findings by severity, coverage status.
