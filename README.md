# scafctl-plugin-sdk

Lightweight Go SDK for building [scafctl](https://github.com/oakwood-commons/scafctl)
plugins. Plugin authors import this module instead of the full scafctl module,
keeping plugin binaries small and dependency-free of host-only packages
(CEL, OpenTelemetry, secrets, catalog, etc.).

## Plugin Types

The SDK supports two plugin types:

- **Provider plugins** -- stateless execution primitives (transform, validate, action)
- **Auth handler plugins** -- authentication, credential storage, and token management

Both use [hashicorp/go-plugin](https://github.com/hashicorp/go-plugin) with gRPC
for host-plugin communication.

## Installation

```bash
go get github.com/oakwood-commons/scafctl-plugin-sdk@latest
```

## Quick Start (Provider Plugin)

```go
package main

import (
    "github.com/oakwood-commons/scafctl-plugin-sdk/plugin"
    "github.com/oakwood-commons/scafctl-plugin-sdk/provider"
    "github.com/oakwood-commons/scafctl-plugin-sdk/provider/schemahelper"
)

type myPlugin struct{}

func (p *myPlugin) GetProviders() ([]string, error) {
    return []string{"my-provider"}, nil
}

func (p *myPlugin) GetProviderDescriptor(name string) (*provider.Descriptor, error) {
    return &provider.Descriptor{
        Type:    name,
        Version: "1.0.0",
        Schema:  schemahelper.ObjectSchema(schemahelper.StringProp("name", "Your name")),
    }, nil
}

// ... implement remaining ProviderPlugin interface methods ...

func main() {
    plugin.Serve(&myPlugin{})
}
```

## Quick Start (Auth Handler Plugin)

```go
package main

import (
    "github.com/oakwood-commons/scafctl-plugin-sdk/auth"
    "github.com/oakwood-commons/scafctl-plugin-sdk/plugin"
)

type myAuthPlugin struct{}

func (p *myAuthPlugin) GetAuthHandlers() ([]auth.HandlerInfo, error) {
    return []auth.HandlerInfo{
        {Name: "my-handler", Flows: []auth.Flow{auth.FlowDeviceCode}},
    }, nil
}

// ... implement remaining AuthHandlerPlugin interface methods ...

func main() {
    plugin.ServeAuthHandler(&myAuthPlugin{})
}
```

## Package Layout

| Package | Purpose |
|---------|---------|
| `plugin/` | Core plugin framework: interfaces, gRPC wiring, `Serve()` / `ServeAuthHandler()` entry points |
| `provider/` | Provider types: `Descriptor`, `Output`, `Capability`, context helpers |
| `provider/schemahelper/` | JSON Schema builder helpers for provider input schemas |
| `auth/` | Auth handler types: `Flow`, `Claims`, `Token`, `Status`, capabilities |
| `testutil/` | `MockProviderPlugin` for integration testing |

## Architecture

```text
                   scafctl-plugin-sdk
                   (shared contract)
                  /                  \
                 v                    v
           scafctl (host)        each plugin
           imports SDK for       imports SDK to
           types + proto         implement interfaces
```

See [design/plugin-sdk-extraction-plan.md](design/plugin-sdk-extraction-plan.md)
for the full extraction plan.

## External Dependencies

The SDK keeps its dependency footprint minimal:

- `github.com/hashicorp/go-plugin` -- host-plugin process management
- `github.com/Masterminds/semver/v3` -- version constraint parsing
- `github.com/google/jsonschema-go` -- JSON Schema types
- `google.golang.org/grpc` + `google.golang.org/protobuf` -- gRPC transport
- `github.com/go-logr/logr` -- structured logging interface

No CEL, no OpenTelemetry, no Cobra, no secrets, no catalog, no solution.

## Compatibility

The SDK version is independent of the scafctl version. Wire compatibility is
governed by `PluginProtocolVersion` (integer) in `plugin/constants.go`.

| SDK version | Protocol version | Compatible scafctl versions |
|-------------|------------------|-----------------------------|
| v0.1.x | 2 | TBD |

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

Apache License 2.0 -- see [LICENSE](LICENSE).
Lightweight SDK for building scafctl plugins (providers and auth handlers)
