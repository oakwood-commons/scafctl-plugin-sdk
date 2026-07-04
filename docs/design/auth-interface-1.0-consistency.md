# Design: Auth-Handler Interface Consistency for 1.0

Status: Proposed
Owner: abaker-9
Related issues: #60 (this repo), #62 (this repo), #58, #59
Affected repos: scafctl-plugin-sdk (root), scafctl (core, consumer only)

## 1. Summary

Issue #60 asks whether, ahead of a 1.0 stabilization, we should move the
per-call `profile` value out of `context.Context` and onto explicit
request/option struct fields, matching how every other request-scoped value
(`hostname`, `scope`, `callbackPort`) is carried on the auth-handler surface.

Decision: **keep `profile` in context.** Document the rationale and close #60 as
an intentional, recorded decision rather than an interface break. Separately,
land one small additive enhancement (`auth.Status.Hostname`) that completes the
per-instance work started in #58 and #62.

## 2. Why keep `profile` in context

`profile` is not broken (#60 states this explicitly). It is:

- A single, call-wide identity/namespace value, set once by the gRPC server
  (`ctx = auth.WithProfile(ctx, req.Profile)`) and read by handlers via
  `auth.ProfileFromContext(ctx)`. This is the accepted context-value use case
  (identity/tenant/request-id scoping), not the discouraged one (optional
  parameters that change processing logic).
- Already available explicitly where it matters at configuration time:
  `ConfigureAuthHandler` receives it via `ProviderConfig.Profile`.
- Carried explicitly on the wire already: every auth request message has a
  `profile` field. This issue is purely about the Go SDK surface, not the
  protocol.

Moving it to explicit fields would:

- Touch 7 methods that inject it (`Login`, `Logout`, `GetStatus`, `GetToken`,
  `ListCachedTokens`, `PurgeExpiredTokens`, `DetectAvailableFlows`).
- Require inventing request structs for the 3 methods that today take only
  `handlerName` (`ListCachedTokens`, `PurgeExpiredTokens`,
  `DetectAvailableFlows`), plus embedding a shared base into the other 4.
- Break every example plugin, all tests, and the host consumer.

That is a whole-interface breaking change to remove one working, isolated
context value, for a purely stylistic consistency gain. For a pre-1.0 SDK with a
handful of implementers, the churn outweighs the benefit. No functional driver
requires it (#60 notes the same).

## 3. Consequences

- The documented inconsistency (`profile` in context; other request-scoped
  values as fields) is accepted and recorded here.
- No breaking release is opened for #60. There is currently no other pending
  interface break to batch with it, so no "1.0 break train" is warranted yet.
- If a future change ever forces an auth-interface break for another reason,
  revisit moving `profile` at that time and land it in the same release.

## 4. Additive follow-ups (independent of the #60 decision)

These arose from the core (scafctl) per-cluster `auth status` work. All are
additive and backward compatible; none require the #60 decision.

### 4.1 `auth.Status.Hostname` (landed)

Added a per-instance identifier to `auth.Status`, mirroring the `Hostname` field
added to `auth.CachedTokenInfo` in #62. This lets a host render the cluster for
a status row straight from `GetStatus`, instead of deriving it from
`ListCachedTokens`. Prerequisite for core UX item N1.

Delivered:

- `Hostname string` (`json:"hostname,omitempty"`) on `auth.Status`.
- `string hostname = 12;` on the `GetStatusResponse` proto message.
- Mapped in `statusToProto`; proto Go code regenerated.
- Populated in the `multicluster` example's `GetStatus`; tests added.

Populated only by handlers advertising `auth.CapInstanceHostname`; empty
otherwise, so the zero value preserves current behavior.

### 4.2 Hostname on the host-side `auth.Handler` interface (not this repo)

The ctx-only `auth.Handler` `Logout(ctx)` / `Status(ctx)` interface that lacks a
hostname lives in scafctl (core), not in this SDK. This SDK's
`AuthHandlerPlugin` already threads hostname through `StatusRequest.Hostname` and
`LogoutRequest.Hostname` (v0.15.0), and the gRPC server maps them. No SDK change
is needed; track the remaining work in scafctl.

### 4.3 `CapScopeRequired` capability (defer)

`CapScopesOnTokenRequest` currently means "supports per-request scopes" but is
being overloaded by core as "requires a scope," which forced an exec-gate
special-case in core's token path. A dedicated marker (e.g. `CapScopeRequired`)
would let core enforce a scope only for handlers that truly need one and drop
the special-case.

Defer until core commits to consuming it. Adding an unused capability the SDK
does not itself act on is speculative; it is a one-line addition once core is
ready to wire it up.

## 5. Release strategy

- #60: close as a documented decision (this note). No version bump required
  beyond publishing the doc.
- Section 4.1 (`auth.Status.Hostname`): landed; ships as an additive minor bump
  alongside the #62 field. Backward compatible.
- Sections 4.2 and 4.3: no SDK action now.
