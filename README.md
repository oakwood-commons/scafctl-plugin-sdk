# scafctl-plugin-sdk

[![Go Report Card](https://goreportcard.com/badge/github.com/oakwood-commons/scafctl-plugin-sdk)](https://goreportcard.com/report/github.com/oakwood-commons/scafctl-plugin-sdk)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)
[![Release](https://img.shields.io/github/v/release/oakwood-commons/scafctl-plugin-sdk)](https://github.com/oakwood-commons/scafctl-plugin-sdk/releases)
[![CI](https://github.com/oakwood-commons/scafctl-plugin-sdk/actions/workflows/test.yml/badge.svg)](https://github.com/oakwood-commons/scafctl-plugin-sdk/actions/workflows/test.yml)
[![codecov](https://codecov.io/gh/oakwood-commons/scafctl-plugin-sdk/graph/badge.svg)](https://codecov.io/gh/oakwood-commons/scafctl-plugin-sdk)
[![Go Reference](https://pkg.go.dev/badge/github.com/oakwood-commons/scafctl-plugin-sdk.svg)](https://pkg.go.dev/github.com/oakwood-commons/scafctl-plugin-sdk)
[![Documentation](https://img.shields.io/badge/docs-plugin%20tutorial-blue)](https://oakwood-commons.github.io/scafctl/tutorials/plugin-development/)

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

~~~bash
go get github.com/oakwood-commons/scafctl-plugin-sdk@latest
~~~

## Quick Start (Provider Plugin)

~~~go
package main

import (
    "context"

    "github.com/Masterminds/semver/v3"
    "github.com/google/jsonschema-go/jsonschema"
    "github.com/oakwood-commons/scafctl-plugin-sdk/plugin"
    "github.com/oakwood-commons/scafctl-plugin-sdk/provider"
    "github.com/oakwood-commons/scafctl-plugin-sdk/provider/schemahelper"
)

type myPlugin struct{}

func (p *myPlugin) GetProviders(_ context.Context) ([]string, error) {
    return []string{"my-provider"}, nil
}

func (p *myPlugin) GetProviderDescriptor(_ context.Context, name string) (*provider.Descriptor, error) {
    return &provider.Descriptor{
        Name:    name,
        Version: semver.MustParse("1.0.0"),
        Schema: schemahelper.ObjectSchema(
            []string{"name"},
            map[string]*jsonschema.Schema{"name": schemahelper.StringProp("Your name")},
        ),
    }, nil
}

// ... implement remaining ProviderPlugin interface methods ...

func main() {
    plugin.Serve(&myPlugin{})
}
~~~

## Quick Start (Auth Handler Plugin)

~~~go
package main

import (
    "context"

    "github.com/oakwood-commons/scafctl-plugin-sdk/auth"
    "github.com/oakwood-commons/scafctl-plugin-sdk/plugin"
)

type myAuthPlugin struct{}

func (p *myAuthPlugin) GetAuthHandlers(_ context.Context) ([]plugin.AuthHandlerInfo, error) {
    return []plugin.AuthHandlerInfo{
        {Name: "my-handler", Flows: []auth.Flow{auth.FlowDeviceCode}},
    }, nil
}

// ... implement remaining AuthHandlerPlugin interface methods ...

func main() {
    plugin.ServeAuthHandler(&myAuthPlugin{})
}
~~~

### Browser OAuth Callback Pages (`ProviderConfig.AuthCallbackPages`)

During `ConfigureAuthHandler`, the host may attach pre-rendered success and
failure HTML documents on `cfg.AuthCallbackPages` (`*plugin.AuthCallbackPages`,
with each document capped at `plugin.MaxAuthCallbackPageBytes` = 256 KiB).
Interactive OAuth handlers using `github.com/oakwood-commons/oauth-helpers`
forward those documents via `oauth.WithCallbackPages(...)` together with
`oauth.WithDeferredCompletion()`, settling the browser response via
`Complete(true)` / `Complete(false)` + `Shutdown(ctx)` after token exchange and
credential storage finish. When `cfg.AuthCallbackPages` is `nil` (for example on
older hosts), `oauth-helpers` serves its built-in neutral fallback pages.

### Per-RPC Stateless Settings, Browser Preference, and Group Resolution

- **Per-RPC settings and profile scoping**: Long-lived or pooled hosts may pass
  JSON-encoded profile settings per call on `Login`, `GetStatus`, `GetToken`,
  `DetectAvailableFlows`, and `GetGroups` (plus `profile` on
  `DetectAvailableFlowsRequest` and `GetGroupsRequest`). Handlers read active
  profile via `auth.ProfileFromContext(ctx)` and per-call settings via
  `plugin.RequestSettingsFromContext(ctx)` (or `req.Settings` on
  `LoginRequest`, `StatusRequest`, and `TokenRequest`), falling back to
  `ConfigureAuthHandler` settings when absent.
- **Browser preference**: Interactive login requests carry `LoginRequest.Browser`
  (mirrored on `auth.LoginOptions.Browser`) so handlers can honor a preferred
  browser without changing the `AuthHandlerPlugin` signature.
- **Optional group resolution (`plugin.AuthGroupsProvider`)**: Auth handler
  plugins that can resolve group memberships (for example when OIDC tokens omit
  groups due to claim overage) may implement the optional `AuthGroupsProvider`
  interface (`GetGroups(ctx, handlerName)`). Plugins that do not implement it
  automatically return gRPC `Unimplemented`. Provider plugins calling
  `HostServiceClient.GetAuthGroups(ctx, handler)` automatically forward
  `auth.ProfileFromContext(ctx)`.
- **Instance-scoped tokens and `secret://` references**: Provider plugins can
  request instance-scoped tokens via
  `HostServiceClient.GetAuthTokenWithOptions(ctx, plugin.AuthTokenRequest{...})`
  (specifying `Hostname` and optional `Profile`), and `plugin.SecretRef`
  supports `env://`, `file://`, and `secret://<name>` via
  `ResolveWithHost(ctx, hostClient)`.

## Package Layout

| Package | Purpose |
|---------|---------|
| `plugin/` | Core plugin framework: interfaces, gRPC wiring, `Serve()` / `ServeAuthHandler()` entry points |
| `provider/` | Provider types: `Descriptor`, `OperationDescriptor`, `Output`, `Capability`, context helpers |
| `provider/schemahelper/` | JSON Schema builder helpers for provider input schemas |
| `auth/` | Auth handler types: `Flow`, `Claims`, `Token`, `Status`, capabilities |
| `testutil/` | `MockProviderPlugin` for integration testing |

## Architecture

~~~text
                   scafctl-plugin-sdk
                   (shared contract)
                  /                  \
                 v                    v
           scafctl (host)        each plugin
           imports SDK for       imports SDK to
           types + proto         implement interfaces
~~~

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
governed by `PluginProtocolVersion` and `AuthHandlerPluginProtocolVersion` in
`plugin/interface.go`.

| SDK version | Protocol version | Compatible scafctl versions |
|-------------|------------------|-----------------------------|
| v0.1.x | 2 | TBD |

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

Apache License 2.0 -- see [LICENSE](LICENSE).
