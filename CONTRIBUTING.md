# Contributing to scafctl-plugin-sdk

Thank you for your interest in contributing!

## Development

### Prerequisites

- Go 1.26+
- [protoc](https://grpc.io/docs/protoc-installation/) (for proto regeneration)
- `protoc-gen-go` and `protoc-gen-go-grpc`

### Building

```bash
go build ./...
```

### Testing

```bash
go test ./...
```

### Proto Regeneration

The proto definitions live in `plugin/proto/`. To regenerate:

```bash
protoc --go_out=. --go_opt=paths=source_relative \
       --go-grpc_out=. --go-grpc_opt=paths=source_relative \
       plugin/proto/plugin.proto
```

## Commits

Use [conventional commits](https://www.conventionalcommits.org/en/v1.0.0/#specification).
All commits must be signed (`-S`) and include DCO sign-off (`-s`).

## Coordination with scafctl

This SDK is the shared contract between scafctl (host) and plugin binaries.
Changes to interfaces or proto definitions must be coordinated with the
[scafctl](https://github.com/oakwood-commons/scafctl) repository.

See [design/plugin-sdk-extraction-plan.md](design/plugin-sdk-extraction-plan.md)
for the versioning contract and migration strategy.

## License

By contributing, you agree that your contributions will be licensed under the
Apache License 2.0.
