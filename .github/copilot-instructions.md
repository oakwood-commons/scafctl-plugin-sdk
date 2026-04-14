# scafctl-plugin-sdk - AI Agent Instructions

## Overview
Go SDK providing shared types, interfaces, and gRPC contracts for scafctl plugins. Plugin authors import this module instead of the full scafctl module.

## Key Packages

- **plugin/**: Plugin interfaces (`ProviderPlugin`, `AuthHandlerPlugin`), gRPC server, `Serve()` helpers, host client
- **provider/**: `Provider`, `Descriptor`, `Output`, `Capability`, context helpers, validation
- **provider/schemahelper/**: JSON Schema helpers (`StringProp`, `IntProp`, `ObjectSchema`)
- **auth/**: Auth types (`Capability`, `Flow`, `Claims`, `Token`, handler types)
- **testutil/**: `MockProviderPlugin` for plugin integration tests

## Conventions

- **Commits**: Use [conventional commits](https://www.conventionalcommits.org/en/v1.0.0/#specification)
- **Signing**: All commits must be GPG/SSH signed (`-S`) and include DCO sign-off (`-s`)
- **Errors**: Return errors with `fmt.Errorf("context: %w", err)`, don't panic
- **Logging**: Use `logr.FromContextOrDiscard(ctx)` -- never `pkg/logger` or `fmt.Printf`

## Build & Test Commands

```bash
# Build
go build ./...

# Test
go test -race ./...

# Vet
go vet ./...
```

## Critical Rules

- **Minimal dependencies**: This SDK must stay lightweight. Do not add CEL, OpenTelemetry, Cobra, or other heavy dependencies
- **Plugin-side only**: Only include code that plugins need. Host-side logic stays in scafctl
- **Interface stability**: Changes to `ProviderPlugin` or `AuthHandlerPlugin` interfaces are breaking changes
- **Proto ownership**: `plugin.proto` lives here. scafctl imports the generated code from this module
- **Test coverage**: Every new or changed file must have tests. Target 70%+ patch coverage
- **Git safety**: Never run `git commit`, `git push`, or `git commit --amend` unless the user explicitly asks
