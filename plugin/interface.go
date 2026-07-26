// Copyright 2025-2026 Oakwood Commons
// SPDX-License-Identifier: Apache-2.0

// Package plugin provides the shared contract between scafctl (host) and plugin
// binaries. Plugin authors implement ProviderPlugin and/or AuthHandlerPlugin,
// then call Serve() or ServeAuthHandler() from main().
package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/oakwood-commons/scafctl-plugin-sdk/auth"
	"github.com/oakwood-commons/scafctl-plugin-sdk/provider"
)

// ProviderConfig holds host-side configuration sent to a provider once
// after plugin load via the ConfigureProvider RPC.
type ProviderConfig struct {
	Quiet         bool                       `json:"quiet" yaml:"quiet"`
	NoColor       bool                       `json:"noColor" yaml:"noColor"`
	BinaryName    string                     `json:"binaryName" yaml:"binaryName"`
	HostServiceID uint32                     `json:"hostServiceId,omitempty" yaml:"hostServiceId,omitempty"`
	Profile       string                     `json:"profile,omitempty" yaml:"profile,omitempty"`
	Settings      map[string]json.RawMessage `json:"settings,omitempty" yaml:"settings,omitempty"`
}

// StreamChunk represents one chunk from a streaming provider execution.
type StreamChunk struct {
	Stdout []byte           `json:"stdout,omitempty" yaml:"stdout,omitempty"`
	Stderr []byte           `json:"stderr,omitempty" yaml:"stderr,omitempty"`
	Result *provider.Output `json:"result,omitempty" yaml:"result,omitempty"`
	Error  string           `json:"error,omitempty" yaml:"error,omitempty"`
}

// ProviderPlugin is the interface that plugins must implement.
type ProviderPlugin interface {
	GetProviders(ctx context.Context) ([]string, error)
	GetProviderDescriptor(ctx context.Context, providerName string) (*provider.Descriptor, error)
	ConfigureProvider(ctx context.Context, providerName string, cfg ProviderConfig) error
	ExecuteProvider(ctx context.Context, providerName string, input map[string]any) (*provider.Output, error)
	ExecuteProviderStream(ctx context.Context, providerName string, input map[string]any, cb func(StreamChunk)) error
	DescribeWhatIf(ctx context.Context, providerName string, input map[string]any) (string, error)
	ExtractDependencies(ctx context.Context, providerName string, inputs map[string]any) ([]string, error)
	StopProvider(ctx context.Context, providerName string) error
}

// ErrStreamingNotSupported is returned by ExecuteProviderStream when the plugin
// does not support streaming execution.
var ErrStreamingNotSupported = errors.New("streaming execution not supported")

// AuthHandlerInfo holds static metadata about an auth handler exposed by a plugin.
type AuthHandlerInfo struct {
	Name         string            `json:"name" yaml:"name"`
	DisplayName  string            `json:"displayName" yaml:"displayName"`
	Flows        []auth.Flow       `json:"flows" yaml:"flows"`
	Capabilities []auth.Capability `json:"capabilities" yaml:"capabilities"`
}

// LoginRequest contains parameters for a plugin Login call.
type LoginRequest struct {
	TenantID string        `json:"tenantId,omitempty" yaml:"tenantId,omitempty"`
	Scopes   []string      `json:"scopes,omitempty" yaml:"scopes,omitempty"`
	Flow     auth.Flow     `json:"flow,omitempty" yaml:"flow,omitempty"`
	Timeout  time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
	// Hostname selects a specific instance (e.g. a cluster or server) for
	// handlers that advertise auth.CapHostname.
	Hostname string `json:"hostname,omitempty" yaml:"hostname,omitempty"`
	// CallbackPort requests a specific loopback port for the OAuth callback
	// server, for handlers that advertise auth.CapCallbackPort. A zero value
	// preserves the handler's default (ephemeral or config-driven) behavior.
	// When set, it is expected to be an unprivileged, in-range TCP port
	// (1024-65535); the host is responsible for validating the value before
	// sending it, and handlers should treat out-of-range values as unset.
	CallbackPort int `json:"callbackPort,omitempty" yaml:"callbackPort,omitempty"`
}

// LoginResponse contains the result of a plugin Login call.
type LoginResponse struct {
	Claims    *auth.Claims `json:"claims,omitempty" yaml:"claims,omitempty"`
	ExpiresAt time.Time    `json:"expiresAt,omitempty" yaml:"expiresAt,omitempty"`
}

// DeviceCodePrompt is sent over streaming Login to relay device-code info to the host.
type DeviceCodePrompt struct {
	UserCode        string `json:"userCode" yaml:"userCode"`
	VerificationURI string `json:"verificationUri" yaml:"verificationUri"`
	Message         string `json:"message" yaml:"message"`
}

// LoginStreamMessage represents a message in the Login server-stream.
type LoginStreamMessage struct {
	DeviceCodePrompt *DeviceCodePrompt `json:"deviceCodePrompt,omitempty" yaml:"deviceCodePrompt,omitempty"`
	Result           *LoginResponse    `json:"result,omitempty" yaml:"result,omitempty"`
	Error            string            `json:"error,omitempty" yaml:"error,omitempty"`
}

// TokenRequest contains parameters for a plugin GetToken call.
type TokenRequest struct {
	Scope         string             `json:"scope,omitempty" yaml:"scope,omitempty"`
	MinValidFor   time.Duration      `json:"minValidFor,omitempty" yaml:"minValidFor,omitempty"`
	ForceRefresh  bool               `json:"forceRefresh,omitempty" yaml:"forceRefresh,omitempty"`
	ServerContext auth.ServerContext `json:"serverContext,omitempty" yaml:"serverContext,omitempty"`
	Caller        auth.CallerType    `json:"caller,omitempty" yaml:"caller,omitempty"`
	Assertion     string             `json:"assertion,omitempty" yaml:"assertion,omitempty"`
	// Hostname selects a specific instance (e.g. a cluster or server) for
	// handlers that advertise auth.CapTokenHostname.
	Hostname string `json:"hostname,omitempty" yaml:"hostname,omitempty"`
}

// TokenResponse contains the result of a plugin GetToken call.
type TokenResponse struct {
	AccessToken string    `json:"accessToken" yaml:"accessToken"` //nolint:gosec
	TokenType   string    `json:"tokenType" yaml:"tokenType"`
	ExpiresAt   time.Time `json:"expiresAt" yaml:"expiresAt"`
	Scope       string    `json:"scope,omitempty" yaml:"scope,omitempty"`
	CachedAt    time.Time `json:"cachedAt,omitempty" yaml:"cachedAt,omitempty"`
	Flow        auth.Flow `json:"flow,omitempty" yaml:"flow,omitempty"`
	SessionID   string    `json:"sessionId,omitempty" yaml:"sessionId,omitempty"`
}

// StatusRequest contains parameters for a plugin GetStatus call.
type StatusRequest struct {
	// Hostname selects a specific instance (e.g. a cluster or server) for
	// handlers that advertise auth.CapInstanceHostname.
	Hostname string `json:"hostname,omitempty" yaml:"hostname,omitempty"`
}

// LogoutRequest contains parameters for a plugin Logout call.
type LogoutRequest struct {
	// Hostname selects a specific instance (e.g. a cluster or server) for
	// handlers that advertise auth.CapInstanceHostname.
	Hostname string `json:"hostname,omitempty" yaml:"hostname,omitempty"`
}

// FlowAvailability reports whether a specific auth flow is available based on
// environment credentials or configuration.
type FlowAvailability struct {
	Flow      auth.Flow `json:"flow" yaml:"flow"`           // flow name (e.g., "device_code", "pat", "service_principal")
	Available bool      `json:"available" yaml:"available"` // true if environment credentials exist for this flow
	Reason    string    `json:"reason" yaml:"reason"`       // human-readable explanation (e.g., "GITHUB_TOKEN is set")
}

// AuthHandlerPlugin is the interface that auth handler plugins must implement.
type AuthHandlerPlugin interface {
	GetAuthHandlers(ctx context.Context) ([]AuthHandlerInfo, error)
	ConfigureAuthHandler(ctx context.Context, handlerName string, cfg ProviderConfig) error
	Login(ctx context.Context, handlerName string, req LoginRequest, deviceCodeCb func(DeviceCodePrompt)) (*LoginResponse, error)
	Logout(ctx context.Context, handlerName string, req LogoutRequest) error
	GetStatus(ctx context.Context, handlerName string, req StatusRequest) (*auth.Status, error)
	GetToken(ctx context.Context, handlerName string, req TokenRequest) (*TokenResponse, error)
	// ListCachedTokens enumerates the handler's cached tokens. Handlers that
	// advertise auth.CapInstanceHostname SHOULD return one entry per live
	// instance, each with CachedTokenInfo.Hostname set to that instance.
	ListCachedTokens(ctx context.Context, handlerName string) ([]*auth.CachedTokenInfo, error)
	PurgeExpiredTokens(ctx context.Context, handlerName string) (int, error)
	DetectAvailableFlows(ctx context.Context, handlerName string) ([]FlowAvailability, error)
	StopAuthHandler(ctx context.Context, handlerName string) error
}

// HandshakeConfigData contains the handshake configuration.
type HandshakeConfigData struct {
	ProtocolVersion  uint   `json:"protocolVersion" yaml:"protocolVersion"`
	MagicCookieKey   string `json:"magicCookieKey" yaml:"magicCookieKey"`
	MagicCookieValue string `json:"magicCookieValue" yaml:"magicCookieValue"`
}

// HandshakeConfig returns the handshake configuration for provider plugin compatibility.
// Returns a copy to prevent mutation of shared state.
func HandshakeConfig() HandshakeConfigData {
	return handshakeConfig
}

// handshakeConfig is the internal provider plugin handshake configuration.
var handshakeConfig = HandshakeConfigData{
	ProtocolVersion:  1,
	MagicCookieKey:   "SCAFCTL_PLUGIN",
	MagicCookieValue: "scafctl_provider_plugin",
}

// PluginProtocolVersion is the current plugin protocol version.
//
// Version 3 adds per-execution provider settings (ExecuteProviderRequest.settings)
// and SolutionMeta.source. The changes are additive and backward compatible with
// version 2 hosts, which simply omit the new fields.
const PluginProtocolVersion int32 = 3

// AuthHandlerPluginProtocolVersion is the current auth handler plugin protocol version.
const AuthHandlerPluginProtocolVersion int32 = 2

// AuthHandlerHandshakeConfig returns the handshake configuration for auth handler plugin compatibility.
// Returns a copy to prevent mutation of shared state.
func AuthHandlerHandshakeConfig() HandshakeConfigData {
	return authHandlerHandshakeConfig
}

// authHandlerHandshakeConfig is the internal auth handler plugin handshake configuration.
var authHandlerHandshakeConfig = HandshakeConfigData{
	ProtocolVersion:  2,
	MagicCookieKey:   "SCAFCTL_AUTH_PLUGIN",
	MagicCookieValue: "scafctl_auth_handler_plugin",
}

const (
	PluginName            = "provider"
	AuthHandlerPluginName = "auth-handler"
)

// ServerMode is an optional interface for auth handler plugins that support
// transitioning into a long-lived server/daemon mode for issuing tokens.
type ServerMode interface {
	ActivateServerMode(ctx context.Context, settings json.RawMessage) error
}
