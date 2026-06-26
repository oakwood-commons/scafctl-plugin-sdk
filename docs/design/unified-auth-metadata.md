# Design: Unified Auth Handler Metadata Schema

Status: Proposed
Owner: abaker-9
Related issues: #23 (this repo), scafctl#398 (core compat shim), scafctl#396 (auth validation)
Affected repos: scafctl-plugin-sdk (root), scafctl (core), scafctl-plugin-auth-entra, scafctl-plugin-auth-github, scafctl-plugin-auth-gcp

## 1. Summary

Auth handlers persist a small JSON "metadata" blob alongside their stored
credentials (claims, session/refresh expiry, scopes, the login flow that
produced the session, and a few handler-specific details). Today every
implementation declares its own struct with divergent JSON field names. We will
introduce a single canonical metadata type in the plugin SDK with strongly
typed shared fields plus an open `metadata` map for handler-specific data, and
adopt it in core and all plugin handlers.

Because the project is pre-1.0 (alpha), this is a clean breaking change: users
re-login once after upgrade, and no backward-compatibility shim is required.

## 2. Background and Problem

Four independent definitions of the same concept have drifted apart. Field
inventory across the affected components:

| Field / concept              | core            | entra                   | github                  | gcp                     | Classification     |
| ---------------------------- | --------------- | ----------------------- | ----------------------- | ----------------------- | ------------------ |
| claims                       | `claims`        | `claims`                | `claims`                | `claims`                | shared (typed)     |
| session expiry               | `expiresAt`     | `refreshTokenExpiresAt` | `refreshTokenExpiresAt` | `refreshTokenExpiresAt` | shared (typed)     |
| login flow                   | `lastLoginFlow` | `loginFlow`             | `flow`                  | `flow`                  | shared (typed)     |
| scopes                       | `scopes`        | `scopes`                | `scopes`                | `scopes`                | shared (typed)     |
| session id                   | --              | `sessionId`             | `sessionId`             | `sessionId`             | shared (typed)     |
| last refresh                 | --              | `lastRefresh`           | `lastRefresh`           | --                      | shared (typed)     |
| client id                    | --              | `clientId`              | `clientId`              | `clientId`              | shared (typed)     |
| tenant id                    | --              | `tenantId`              | --                      | --                      | handler-specific   |
| hostname                     | --              | --                      | `hostname`              | --                      | handler-specific   |
| identity type                | --              | --                      | `identityType`          | --                      | handler-specific   |
| project                      | --              | --                      | --                      | `project`               | handler-specific   |
| impersonate service account  | --              | --                      | --                      | `impersonateServiceAccount` | handler-specific |

Root cause: core, entra, github, and gcp each declare their own `TokenMetadata`
/ `handlerMetadata` struct. There is no shared type, so nothing structurally
prevents the field names from diverging -- and three different names for "login
flow" (`lastLoginFlow`, `loginFlow`, `flow`) prove it already has.

The `scafctl-plugin-identity` repo has no persisted handler-metadata blob and is
not affected.

### Why this matters

- Core added a one-directional compat shim (`unmarshalMetadata`, scafctl#398)
  so the generic `oauth2.Handler` can read plugin-format metadata. That shim is
  already incomplete: it maps the entra name `loginFlow` but not the
  github/gcp name `flow`, so their login flow is silently dropped in that path.
- More importantly, the multi-way drift is a maintainability hazard: every new
  provider currently invents its own struct and can diverge again.

### Architectural enabler

Core's `pkg/auth` package is already a thin re-export of the SDK's `auth`
package -- `Claims`, `Flow`, `Capability`, `Result`, and the `Flow*` constants
are all type aliases to `sdkauth.*`, and core depends on the SDK. The SDK is
therefore the established single source of truth for shared auth types, which
makes it the correct home for a canonical metadata type. Core adopts it with
zero type friction because its `auth.Claims` / `auth.Flow` already alias the
SDK's.

## 3. Goals

- One canonical metadata type, defined once in the SDK, used by core and all
  plugin handlers.
- Strongly typed shared fields for everything common; an open `metadata` map for
  handler-specific data so new providers extend without new structs.
- Resolve the github/gcp `flow` divergence and prevent future drift
  structurally.
- Keep the SDK dependency-light (standard library only for this change).

## 4. Non-Goals

- Backward compatibility with existing stored blobs. This is an intentional
  breaking change; users re-login once. No dual-read shim.
- Unifying the secret-key namespaces of core vs plugins. Each handler keeps its
  own namespace; this design only unifies the JSON schema of the blob.
- Sharing access-token cache entries across handlers (out of scope; the token
  cache already works per-handler via the host secret store).
- Changing how credentials (refresh tokens) themselves are stored.

## 5. Proposed Design

### 5.1 Canonical type in the SDK

Add to the SDK `auth` package a canonical metadata type: strongly typed shared
fields plus an open `Metadata` map for handler-specific values.

~~~go
// HandlerMetadata is the canonical persisted metadata for an auth session,
// shared by core and all plugin auth handlers.
//
// The typed fields cover everything common across handlers. Handler-specific
// values (e.g. entra tenantId, gcp project) go in the open Metadata map, keyed
// by exported constants defined in each plugin to avoid key drift.
type HandlerMetadata struct {
    Claims        *Claims   `json:"claims,omitempty"`
    ExpiresAt     time.Time `json:"expiresAt,omitempty"`
    Scopes        []string  `json:"scopes,omitempty"`
    LastLoginFlow Flow      `json:"lastLoginFlow,omitempty"`
    SessionID     string    `json:"sessionId,omitempty"`
    LastRefresh   time.Time `json:"lastRefresh,omitempty"`
    ClientID      string    `json:"clientId,omitempty"`

    // Metadata holds handler-specific fields that are not part of the canonical
    // schema. Keys should be defined as exported constants by each handler.
    // Values are typically strings; numeric values decode as float64 per
    // encoding/json semantics.
    Metadata map[string]any `json:"metadata,omitempty"`
}

// Convenience accessors keep call sites free of repetitive type assertions.

// MetaString returns the string value for key, or "" if absent or not a string.
func (m HandlerMetadata) MetaString(key string) string {
    s, _ := m.Metadata[key].(string)
    return s
}

// SetMeta sets a handler-specific key, allocating the map on first use.
func (m *HandlerMetadata) SetMeta(key string, value any) {
    if m.Metadata == nil {
        m.Metadata = make(map[string]any)
    }
    m.Metadata[key] = value
}
~~~

No dual-read helper is needed: plugins `json.Unmarshal` directly into
`HandlerMetadata`.

### 5.2 Core adoption

Replace core's private `handlerMetadata` struct and the `unmarshalMetadata`
compat shim in `pkg/auth/oauth2/handler.go` with `auth.HandlerMetadata` and a
plain `json.Unmarshal`. Because core's `auth.Claims` / `auth.Flow` already alias
the SDK types, this is a mechanical swap; `Status` and `GetToken` keep reading
the same typed fields. The now-unnecessary compat shim and its tests are
deleted.

### 5.3 Plugin adoption (entra, github, gcp)

Each plugin deletes its local `TokenMetadata` struct and uses
`auth.HandlerMetadata` directly. Handler-specific fields move into the `Metadata`
map under exported key constants:

~~~go
// entra example
const MetaKeyTenantID = "tenantId"

md := auth.HandlerMetadata{
    Claims:        claims,
    ExpiresAt:     refreshExpiry,
    Scopes:        scopes,
    LastLoginFlow: flow,
    SessionID:     sessionID,
    ClientID:      clientID,
}
md.SetMeta(MetaKeyTenantID, tenantID)
~~~

Key constants per handler:

- entra: `tenantId`
- github: `hostname`, `identityType`
- gcp: `project`, `impersonateServiceAccount`

Reads use `json.Unmarshal` into `auth.HandlerMetadata` and `md.MetaString(key)`
for the extras. Writes emit canonical names only.

### 5.4 Precedent: ford-cloud/platform-assets

The open-`metadata` pattern is already established house style in
`ford-cloud/platform-assets`. Its public API types
(`pkg/apis/platformassets/v1/types.go`) attach an open extension field to nearly
every resource (`PlatformProfile`, `Application`, `GCPProject`, `KubeNamespace`,
`GithubRepository`, and the add/update request types):

~~~go
Metadata map[string]any `json:"metadata,omitempty" yaml:"metadata,omitempty" doc:"..."`
~~~

Key takeaways that inform this design:

- The field type is `map[string]any` with `json` + `yaml` tags. This design
  matches that, so the SDK stays consistent with the Ford house pattern. (The
  `doc:` tag in platform-assets feeds its OpenAPI generation; the SDK auth
  structs likewise carry `json` + `yaml` + `doc` tags, so `HandlerMetadata`
  includes `doc:` tags to match the existing SDK convention.)
- platform-assets genuinely nests structured sub-objects under metadata keys
  (for example `metadata["EAMS"]`, `metadata["service_now"]`,
  `metadata["validationIssues"]["groups"]`). This confirms `map[string]any` over
  `map[string]string`: the openness to structured values is used in practice.
- platform-assets is fully freeform: it uses raw string keys and inline type
  assertions, with no key constants and no accessor helpers. We deliberately
  diverge on this one point (see 5.5).

### 5.5 Where this design adds guardrails over platform-assets

platform-assets metadata holds dynamic, externally sourced data (EAMS,
ServiceNow blobs) whose keys are not known at compile time, so freeform string
keys are appropriate there. The auth handlers are the opposite case: the
handler-specific extras are a small, fixed, compile-time-known set
(`tenantId`, `hostname`, `identityType`, `project`,
`impersonateServiceAccount`). For a fixed set, leaving keys as scattered string
literals would reintroduce exactly the drift issue #23 exists to fix.

Therefore this design keeps the identical field type and tags as
platform-assets, but adds two lightweight guardrails the platform-assets domain
does not need: exported key constants per handler, and the `MetaString` /
`SetMeta` accessors. Handlers that ever need dynamic, unbounded keys can still
use the map directly, exactly as platform-assets does.

### 5.6 Compatibility

This is a breaking change to the persisted metadata schema. Old blobs written
with legacy field names will not populate the new typed fields, so handlers will
treat the session as needing re-login. This is acceptable in alpha and is the
intended behavior; it is called out in release notes.

## 6. Rollout Plan

Dependency order matters because core and all plugins depend on the SDK, and
nothing depends on the plugins.

1. SDK: add `HandlerMetadata` + accessors + table-driven tests. Tag a release.
2. Core: bump SDK dependency, swap to the shared type, delete the private shim
   and its tests, verify.
3. Plugins (entra, github, gcp): bump SDK dependency, delete local
   `TokenMetadata`, adopt the shared type with key constants, verify.
4. Release notes: state that a one-time re-login is required after upgrade.

## 7. Testing Strategy

- SDK: table-driven tests for `HandlerMetadata` round-trip marshal/unmarshal
  (typed fields + `metadata` map), `MetaString` on present/absent/wrong-type
  keys, and `SetMeta` map allocation. Confirm canonical JSON tag names.
- Core: existing `oauth2.Handler` tests (`Status`, `GetToken`,
  `TestWithSecretKeyPrefix`) updated to the canonical schema; delete
  `TestUnmarshalMetadata_*` compat tests.
- Plugins: round-trip tests asserting canonical tags on write and correct
  retrieval of handler-specific keys from `metadata`.
- All repos: `go build ./...`, `go vet ./...`, `go test -race ./...`.

## 8. Risks and Mitigations

- Risk: stringly-typed keys in `metadata` drift like the structs did.
  Mitigation: each handler exports key constants; tests assert them.
- Risk: `map[string]any` numeric values decode as float64. Mitigation: current
  extras are all strings; documented; revisit with `json.RawMessage` if a
  handler ever needs structured/numeric extras.
- Risk: breaking change surprises users. Mitigation: alpha status, explicit
  release notes, one-time re-login only.

## 9. Decisions

1. Open-field value type: `map[string]any` (DECIDED). Matches the established
   `ford-cloud/platform-assets` house pattern (section 5.4), and its openness to
   structured values is exercised in practice there. Rejected alternatives:
   `map[string]string` (lossless for today's all-string extras but cannot hold
   structured values, and diverges from the house pattern) and
   `json.RawMessage` (full plugin-side typing at the cost of boilerplate, not
   warranted for the small fixed key set).
2. Key access strategy: exported key constants per handler plus `MetaString` /
   `SetMeta` accessors (DECIDED). This diverges intentionally from
   platform-assets' fully freeform raw-string-key approach because the auth key
   set is small and fixed; see section 5.5.
3. Should `clientId`, `sessionId`, `lastRefresh` be typed fields or live in the
   open map? Typed (DECIDED), since they appear across multiple handlers; only
   truly single-handler fields go in `metadata`.

## 10. Alternatives Considered

- Decentralized rename (each plugin renames its own tags, no SDK type). Smaller
  diff but does not prevent future drift. Rejected; the shared type is the only
  structural fix.
- Embedding the canonical type in per-plugin wrapper structs for typed extras.
  More type safety for handler-specific fields, but reintroduces per-plugin
  structs and the drift surface. The open `metadata` map is preferred for
  extensibility; handlers that want typing can still wrap locally if needed.
- Dual-read compat shim (keep reading legacy names). Unnecessary given alpha
  breaking changes are acceptable; adds permanent complexity. Rejected.
- Core-only shim extension (just add `flow` to `unmarshalMetadata`). Fixes the
  one concrete bug but leaves schemas divergent. Superseded by the full fix.
