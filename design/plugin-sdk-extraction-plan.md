# Plugin SDK Extraction Plan

## Overview

Extract the minimal set of packages from `scafctl` into a new Go module
(`github.com/oakwood-commons/scafctl-plugin-sdk`) that plugin authors import
instead of the full scafctl module. Both scafctl (host) and every plugin binary
then depend on the SDK for their shared contract.

```text
                   scafctl-plugin-sdk
                   (shared contract)
                  /                  \
                 v                    v
           scafctl (host)        each plugin
           imports SDK for       imports SDK to
           types + proto         implement interfaces
```

## Motivation

Today a plugin binary like `examples/plugins/echo/main.go` imports:

```text
github.com/oakwood-commons/scafctl/pkg/plugin
github.com/oakwood-commons/scafctl/pkg/provider
github.com/oakwood-commons/scafctl/pkg/provider/schemahelper
```

Because Go compiles the entire package (not individual files), this transitively
pulls in **every** file in `pkg/provider` and `pkg/plugin`, including:

| Transitive dependency | Approx. size | Used by plugin? |
|-----------------------|-------------|-----------------|
| `google/cel-go` | 2.5 MB | No (inputs.go) |
| `go.opentelemetry.io/otel/*` | 2+ MB | No (metrics.go, executor.go) |
| `pkg/secrets` | -- | No (host-side only) |
| `pkg/catalog` | -- | No (host-side only) |
| `pkg/solution` | -- | No (host-side only) |

An extracted SDK eliminates these transitive dependencies entirely.

---

## SDK Repository Structure

```text
github.com/oakwood-commons/scafctl-plugin-sdk/
  go.mod                         # module github.com/oakwood-commons/scafctl-plugin-sdk
  plugin/
    interface.go                 # ProviderPlugin, AuthHandlerPlugin interfaces
    config.go                    # ProviderConfig, StreamChunk, LoginRequest, etc.
    server.go                    # Serve(), ServeAuthHandler()
    grpc_plugin.go               # GRPCPlugin struct + GRPCServer/GRPCClient methods
    grpc_server.go               # GRPCServer (plugin-side RPC handlers)
    host_client.go               # HostServiceClient (plugin calls back to host)
    constants.go                 # HandshakeConfig, PluginProtocolVersion, plugin names
    errors.go                    # ErrStreamingNotSupported
    proto/
      plugin.proto               # Self-contained gRPC definitions (unchanged)
      plugin.pb.go               # Generated
      plugin_grpc.pb.go          # Generated
  provider/
    provider.go                  # Provider, Descriptor, Output, Capability, Link, etc.
    context.go                   # With*/From* context helpers, SolutionMeta, IOStreams
    validation.go                # ValidateDescriptor()
    schemahelper/
      schemahelper.go            # StringProp, IntProp, ObjectSchema, PropOption, etc.
  auth/
    capability.go                # Capability constants, HasCapability()
    flow.go                      # Flow type + constants (FlowDeviceCode, etc.)
    handler.go                   # LoginOptions, TokenOptions, Result, Status
    claims.go                    # Claims, Token, CachedTokenInfo, IdentityType
  testutil/
    mock.go                      # MockProviderPlugin for plugin integration tests
  examples/
    echo/
      main.go                    # Reference echo plugin (current examples/plugins/echo)
```

## SDK External Dependencies

The SDK `go.mod` would contain only:

```text
require (
    github.com/hashicorp/go-plugin   v1.x
    github.com/Masterminds/semver/v3 v3.x
    github.com/google/jsonschema-go  v0.x
    google.golang.org/grpc           v1.x
    google.golang.org/protobuf       v1.x
    github.com/go-logr/logr          v1.x
)
```

No CEL, no OpenTelemetry, no Cobra, no secrets, no catalog, no solution.

---

## What Stays in scafctl

Everything that only the host process uses:

| File | Content | Why it stays |
|------|---------|-------------|
| `pkg/plugin/client.go` | `Client`, `NewClient`, `ClientOption` | Host creates plugin processes |
| `pkg/plugin/wrapper.go` | `ProviderWrapper`, `RegisterPluginProviders`, `KillAll` | Host wraps plugins as providers |
| `pkg/plugin/fetcher.go` | `FetchPlugins`, `RegisterFetchedPlugins` | Host fetches from catalogs |
| `pkg/plugin/grpc_host.go` | `HostServiceServer`, `HostServiceDeps` | Host serves HostService to plugins |
| `pkg/plugin/grpc.go` (partial) | `GRPCClient` (host-side client) | Host calls PluginService RPCs |
| `pkg/provider/executor.go` | `Execute()` with telemetry + metrics | Host-side execution orchestration |
| `pkg/provider/inputs.go` | CEL/template input evaluation | Host-side input processing |
| `pkg/provider/metrics.go` | OpenTelemetry provider metrics | Host-side observability |
| `pkg/provider/registry.go` | `Registry` | Host-side provider registry |
| `pkg/auth/*.go` (most) | Registry, cache, token_acquire, login, jwt | Host-side auth management |

scafctl's `go.mod` would `require` the SDK and re-export types via type aliases
where backward compatibility is needed during transition.

---

## File-Level Split of pkg/plugin/grpc.go

`grpc.go` currently contains both host and plugin code. It must be split:

### Moves to SDK (`plugin/grpc_plugin.go` + `plugin/grpc_server.go`)

```text
GRPCPlugin struct               # Bridges hashicorp/go-plugin and gRPC
GRPCPlugin.GRPCServer()         # Plugin side: registers PluginService server
GRPCServer struct                # Plugin-side RPC handlers
GRPCServer.GetProviders()
GRPCServer.GetProviderDescriptor()
GRPCServer.ExecuteProvider()
GRPCServer.ExecuteProviderStream()
GRPCServer.ConfigureProvider()   # Plugin receives config + connects HostService
GRPCServer.DescribeWhatIf()
GRPCServer.ExtractDependencies()
GRPCServer.StopProvider()
descriptorToProto()              # Plugin serialises Descriptor for host
```

### Stays in scafctl

```text
GRPCPlugin.GRPCClient()         # Host side: creates GRPCClient + starts HostService
GRPCClient struct                # Host calls PluginService RPCs
GRPCClient.GetProviders()
GRPCClient.GetProviderDescriptor()
GRPCClient.ExecuteProvider()
GRPCClient.ExecuteProviderStream()
GRPCClient.ConfigureProvider()
GRPCClient.DescribeWhatIf()
GRPCClient.ExtractDependencies()
GRPCClient.StopProvider()
protoToDescriptor()              # Host deserialises Descriptor from wire
```

### Architectural note

`GRPCPlugin` has both `GRPCServer()` (plugin-side) and `GRPCClient()` (host-side).
The SDK needs the full struct because `hashicorp/go-plugin` requires the `Plugin`
implementation to exist on both sides. However, `GRPCClient()` on the plugin side
returns nil and is never invoked -- it only matters on the host. The cleanest
approach:

1. SDK defines `GRPCPlugin` with both methods.
2. `GRPCClient()` in the SDK returns a minimal stub (`&GRPCClient{client: ...}`).
3. scafctl wraps the stub with its own extended `GRPCClient` that adds broker
   wiring, HostService startup, and host-side dependencies.

---

## File-Level Split of pkg/plugin/grpc_host.go

### Moves to SDK (`plugin/host_client.go`)

```text
HostServiceClient struct         # Plugin calls back to host
NewHostServiceClient()
HostServiceClient.GetSecret()
HostServiceClient.SetSecret()
HostServiceClient.DeleteSecret()
HostServiceClient.ListSecrets()
HostServiceClient.GetAuthIdentity()
HostServiceClient.ListAuthHandlers()
HostServiceClient.GetAuthToken()
```

### Stays in scafctl

```text
HostServiceDeps struct           # Host configuration for callbacks
HostServiceServer struct         # Host implements HostService gRPC
HostServiceServer.GetSecret()
HostServiceServer.SetSecret()
HostServiceServer.DeleteSecret()
HostServiceServer.ListSecrets()
HostServiceServer.GetAuthIdentity()
HostServiceServer.ListAuthHandlers()
HostServiceServer.GetAuthToken()
isAuthHandlerAllowed()           # Host-side access control
```

---

## File-Level Split of pkg/provider

### Moves to SDK (`provider/`)

| File | Types/Functions |
|------|----------------|
| `provider.go` | `Provider`, `Descriptor`, `Output`, `Capability`, `Link`, `Example`, `Contact`, capability constants |
| `context.go` | All `With*`/`From*` context helpers, `SolutionMeta`, `IterationContext`, `IOStreams` type |
| `validation.go` | `ValidateDescriptor()` |

### Stays in scafctl (`pkg/provider/`)

| File | Content |
|------|---------|
| `executor.go` | Host-side `Execute()` with telemetry spans |
| `inputs.go` | CEL + Go template input evaluation |
| `metrics.go` | OpenTelemetry metrics |
| `registry.go` | `Registry` type |
| `path.go` | Output path resolution (host-side) |

### The `logger` dependency problem

`provider.go` imports `pkg/logger` for `ValidateDescriptor()` warnings. Options:

1. **Preferred**: Use `logr.FromContextOrDiscard(ctx)` directly (stdlib-compatible)
   instead of `logger.FromContext()`. The `logr` package is already an SDK dep
   (required by hashicorp/go-plugin). Remove the `pkg/logger` import.
2. **Alternative**: Move the logger-using code to a separate file that stays in
   scafctl and have the SDK version return errors instead of logging.

---

## File-Level Split of pkg/auth

### Moves to SDK (`auth/`)

| File | Types | Internal deps |
|------|-------|---------------|
| `capability.go` | `Capability`, `HasCapability()` | None |
| `flow.go` | `Flow` + 8 constants | None |
| `claims.go` | `Claims`, `Token`, `CachedTokenInfo`, `IdentityType` | stdlib only |
| `handler.go` | `LoginOptions`, `TokenOptions`, `Result`, `Status` | stdlib only |

Clean -- no scafctl internal imports in any of these files.

### Stays in scafctl

Everything else: `cache.go`, `context.go`, `errors.go`, `fingerprint.go`,
`groups.go`, `jwt.go`, `login.go`, `mock.go`, `registry.go`, `token_acquire.go`.

---

## Migration Strategy

### Phase 1: Create SDK repo (no changes to scafctl)

1. Create `github.com/oakwood-commons/scafctl-plugin-sdk` repository
2. Copy the files identified above into the SDK structure
3. Update `go_package` in `plugin.proto`, regenerate
4. Replace `pkg/logger` usage with `logr.FromContextOrDiscard(ctx)`
5. Write SDK-specific tests (MockProviderPlugin, HostServiceClient unit tests)
6. Tag `v0.1.0`

### Phase 2: Update scafctl to import SDK

1. `go get github.com/oakwood-commons/scafctl-plugin-sdk@v0.1.0`
2. Replace type definitions with imports from SDK:

~~~go
// pkg/plugin/interface.go (before)
type ProviderPlugin interface { ... }

// pkg/plugin/interface.go (after)
import sdk "github.com/oakwood-commons/scafctl-plugin-sdk/plugin"

// Type alias preserves backward compat for any internal callers
type ProviderPlugin = sdk.ProviderPlugin
type ProviderConfig = sdk.ProviderConfig
type StreamChunk = sdk.StreamChunk
~~~

3. Same pattern for `pkg/provider`:

~~~go
import sdk "github.com/oakwood-commons/scafctl-plugin-sdk/provider"

type Provider = sdk.Provider
type Descriptor = sdk.Descriptor
type Output = sdk.Output
type Capability = sdk.Capability
~~~

4. Same pattern for `pkg/auth` types
5. Update `GRPCPlugin.GRPCClient()` to wrap SDK's stub client
6. Run `go mod tidy`
7. Verify: `go build ./...`, `task lint`, `task test:e2e`

### Phase 3: Update echo plugin

1. Change echo's imports from `scafctl/pkg/plugin` to SDK imports
2. Verify it compiles against SDK alone (no scafctl dependency)
3. Build and test with scafctl host

### Phase 4: Migrate external plugins

Each plugin repo:
1. `go get github.com/oakwood-commons/scafctl-plugin-sdk@v0.1.0`
2. Import from SDK instead of scafctl
3. Verify binary size reduction

### Phase 5: Remove type aliases (breaking, optional)

Once all plugins use the SDK:
1. Remove type aliases from scafctl
2. Import SDK types directly
3. Tag scafctl major version bump

---

## Versioning Contract

| Component | Version | Governs |
|-----------|---------|---------|
| SDK module version | semver | Go API surface (types, functions) |
| `PluginProtocolVersion` | integer | Wire-format feature detection |
| `HandshakeConfig.ProtocolVersion` | integer | hashicorp/go-plugin handshake |

### Rules

- **Patch bump** (v0.1.0 -> v0.1.1): Bug fixes, doc changes
- **Minor bump** (v0.1.0 -> v0.2.0): New optional RPC, new fields, new helper functions
- **Major bump** (v0.x -> v1.0.0 or v1 -> v2): Breaking interface changes, removed methods
- `PluginProtocolVersion` increments when adding RPCs that need feature detection
- `HandshakeConfig.ProtocolVersion` increments on incompatible wire changes (rare)

---

## Risk Assessment

| Risk | Impact | Mitigation |
|------|--------|-----------|
| SDK and scafctl get out of sync | Plugins fail at runtime | CI job in scafctl that builds echo plugin against latest SDK |
| Type alias phase creates confusion | Developer friction | Keep phase short; document clearly |
| Proto regeneration in two places | Stale generated code | Proto lives in SDK only; scafctl imports generated code |
| GRPCPlugin split is complex | Bugs in broker wiring | Thorough integration tests before/after |
| Plugin binary links wrong SDK version | Handshake failure | go-plugin handshake catches version mismatch at startup |

---

## Expected Impact

| Metric | Before SDK | After SDK |
|--------|-----------|-----------|
| Plugin binary size (echo) | ~30 MB | ~8-10 MB |
| Plugin `go.mod` requires | 1 (entire scafctl) | 1 (SDK only) |
| Plugin transitive deps | ~80+ | ~15 |
| CEL in plugin binary | Yes | No |
| OpenTelemetry in plugin binary | Yes | No |
| Plugin compile time | ~15s | ~5s |

---

## Checklist

- [ ] Create `oakwood-commons/scafctl-plugin-sdk` repo
- [ ] Initialize `go.mod` with module path
- [ ] Copy and adapt `plugin.proto` (update `go_package`)
- [ ] Regenerate proto with `protoc`
- [ ] Extract plugin-side types from `pkg/plugin/interface.go`
- [ ] Extract `Serve()`, `ServeAuthHandler()` from `server.go`
- [ ] Split `GRPCPlugin` + `GRPCServer` from `grpc.go`
- [ ] Extract `HostServiceClient` from `grpc_host.go`
- [ ] Extract `Descriptor`, `Output`, `Capability` from `pkg/provider/provider.go`
- [ ] Extract context helpers from `pkg/provider/context.go`
- [ ] Extract `ValidateDescriptor()` from `pkg/provider/validation.go`
- [ ] Copy `pkg/provider/schemahelper/` (no changes needed)
- [ ] Extract auth types: `capability.go`, `flow.go`, `claims.go`, `handler.go`
- [ ] Replace `logger.FromContext` with `logr.FromContextOrDiscard` in SDK code
- [ ] Add `MockProviderPlugin` to `testutil/mock.go`
- [ ] Move echo plugin to `examples/echo/`
- [ ] Write SDK README with quick-start guide
- [ ] Tag `v0.1.0`
- [ ] Update scafctl to import SDK (type aliases)
- [ ] Verify scafctl builds and all 602 e2e tests pass
- [ ] Update echo plugin to use SDK imports
- [ ] Verify echo binary size reduction
- [ ] CI: add cross-build job for echo plugin against SDK
