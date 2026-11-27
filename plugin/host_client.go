// Copyright 2025-2026 Oakwood Commons
// SPDX-License-Identifier: Apache-2.0

package plugin

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/oakwood-commons/scafctl-plugin-sdk/auth"
	"github.com/oakwood-commons/scafctl-plugin-sdk/plugin/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	// MaxHostSecretNameLength is the maximum allowed byte length for a secret
	// name passed to HostService GetSecret, SetSecret, and DeleteSecret RPCs.
	MaxHostSecretNameLength = 256

	// MaxHostSecretPatternLength is the maximum allowed byte length for a regular
	// expression pattern passed to HostService ListSecrets RPCs.
	MaxHostSecretPatternLength = 256
)

// ValidateHostSecretName checks that name satisfies the host secret-store
// RPC naming contract enforced by HostService GetSecret, SetSecret, and
// DeleteSecret handlers:
//   - Non-empty and at most MaxHostSecretNameLength (256) bytes
//   - Contains only ASCII alphanumeric characters, '_', '-', '.', and '/'
//     (matching ^[a-zA-Z0-9_\-./]+$; characters such as ':' are rejected)
//   - Does not contain ".." path traversal sequences
//
// Note: the host's encrypted backing secret store further requires names to be
// 1..255 bytes, start with an ASCII alphanumeric character, and contain only
// ASCII alphanumeric characters, '.', '_', or '-' (no '/'). User-authored
// secret://<name> references validated by SecretRef.Validate also reject the
// reserved "scafctl." prefix, whereas auth handler plugins may store internal
// credentials under prefixes such as "scafctl.auth.<handler>.".
func ValidateHostSecretName(name string) error {
	if name == "" {
		return errors.New("secret name must not be empty")
	}
	if len(name) > MaxHostSecretNameLength {
		return fmt.Errorf("secret name exceeds maximum length of %d", MaxHostSecretNameLength)
	}
	if !isValidHostSecretNameChars(name) {
		return errors.New("secret name contains invalid characters")
	}
	if strings.Contains(name, "..") {
		return errors.New("secret name must not contain path traversal sequences")
	}
	return nil
}

func isValidHostSecretNameChars(name string) bool {
	if len(name) == 0 {
		return false
	}
	for i := 0; i < len(name); i++ {
		b := name[i]
		if !isASCIIAlnum(b) && b != '_' && b != '-' && b != '.' && b != '/' {
			return false
		}
	}
	return true
}

// HostServiceClient wraps the HostService gRPC client (used by plugins).
type HostServiceClient struct {
	client proto.HostServiceClient
}

// NewHostServiceClient creates a HostServiceClient from a gRPC connection.
func NewHostServiceClient(conn *grpc.ClientConn) *HostServiceClient {
	return &HostServiceClient{client: proto.NewHostServiceClient(conn)}
}

// GetSecret retrieves a secret from the host's secret store.
//
// The host validates name at the RPC layer with ValidateHostSecretName and at
// the backing store layer (1..255 bytes, leading ASCII alphanumeric, and only
// ASCII alphanumeric, '.', '_', or '-'), while enforcing any plugin-specific
// prefix restrictions. When the secret does not exist, GetSecret returns
// ("", false, nil).
func (c *HostServiceClient) GetSecret(ctx context.Context, name string) (string, bool, error) {
	resp, err := c.client.GetSecret(ctx, &proto.GetSecretRequest{Name: name})
	if err != nil {
		return "", false, fmt.Errorf("host GetSecret: %w", err)
	}
	if resp.Error != "" {
		return "", false, fmt.Errorf("host GetSecret: %s", resp.Error)
	}
	return resp.Value, resp.Found, nil
}

// SetSecret stores a secret in the host's secret store.
//
// The host validates name at the RPC layer with ValidateHostSecretName and at
// the backing store layer (1..255 bytes, leading ASCII alphanumeric, and only
// ASCII alphanumeric, '.', '_', or '-'), rejecting writes when the plugin's
// secret access is read-only or outside its allowed prefix scope.
func (c *HostServiceClient) SetSecret(ctx context.Context, name, value string) error {
	resp, err := c.client.SetSecret(ctx, &proto.SetSecretRequest{Name: name, Value: value})
	if err != nil {
		return fmt.Errorf("host SetSecret: %w", err)
	}
	if resp.Error != "" {
		return fmt.Errorf("host SetSecret: %s", resp.Error)
	}
	return nil
}

// DeleteSecret removes a secret from the host's secret store. Deleting a
// non-existent secret is a no-op when name is valid and permitted.
//
// The host validates name at the RPC layer with ValidateHostSecretName and at
// the backing store layer (1..255 bytes, leading ASCII alphanumeric, and only
// ASCII alphanumeric, '.', '_', or '-'), rejecting deletes when the plugin's
// secret access is read-only or outside its allowed prefix scope.
func (c *HostServiceClient) DeleteSecret(ctx context.Context, name string) error {
	resp, err := c.client.DeleteSecret(ctx, &proto.DeleteSecretRequest{Name: name})
	if err != nil {
		return fmt.Errorf("host DeleteSecret: %w", err)
	}
	if resp.Error != "" {
		return fmt.Errorf("host DeleteSecret: %s", resp.Error)
	}
	return nil
}

// ListSecrets lists secret names from the host's secret store matching pattern.
//
// IMPORTANT: pattern is an unanchored Go regular expression compiled by the
// host with regexp.Compile (at most MaxHostSecretPatternLength bytes), NOT a
// shell glob. An empty pattern ("") returns all secret names visible to the
// plugin; otherwise "." matches any character and "*" quantifies the preceding
// token. To list secrets under a literal key prefix (for example
// "scafctl.auth.myhandler.token."), prefer ListSecretsByPrefix.
func (c *HostServiceClient) ListSecrets(ctx context.Context, pattern string) ([]string, error) {
	resp, err := c.client.ListSecrets(ctx, &proto.ListSecretsRequest{Pattern: pattern})
	if err != nil {
		return nil, fmt.Errorf("host ListSecrets: %w", err)
	}
	if resp.Error != "" {
		return nil, fmt.Errorf("host ListSecrets: %s", resp.Error)
	}
	return resp.Names, nil
}

// ListSecretsByPrefix lists secret names from the host's secret store that
// start with the literal string prefix.
//
// Prefix must not exceed MaxHostSecretNameLength bytes. When prefix is
// non-empty, ListSecretsByPrefix builds a "^"-anchored regular expression
// escaping metacharacters via regexp.QuoteMeta (bounded to
// MaxHostSecretPatternLength bytes at a complete rune/escape boundary when
// QuoteMeta expansion crosses the pattern limit) and filters the returned
// slice with strings.HasPrefix as defense in depth. When prefix is empty, all
// visible secret names are returned.
func (c *HostServiceClient) ListSecretsByPrefix(ctx context.Context, prefix string) ([]string, error) {
	if len(prefix) > MaxHostSecretNameLength {
		return nil, fmt.Errorf(
			"host ListSecretsByPrefix: prefix length %d exceeds maximum of %d bytes",
			len(prefix),
			MaxHostSecretNameLength,
		)
	}
	pattern := escapedPrefixPattern(prefix)
	names, err := c.ListSecrets(ctx, pattern)
	if err != nil {
		return nil, err
	}
	filtered := make([]string, 0, len(names))
	for _, name := range names {
		if strings.HasPrefix(name, prefix) {
			filtered = append(filtered, name)
		}
	}
	return filtered, nil
}

func escapedPrefixPattern(prefix string) string {
	if prefix == "" {
		return ""
	}
	if pattern := "^" + regexp.QuoteMeta(prefix); len(pattern) <= MaxHostSecretPatternLength {
		return pattern
	}
	var b strings.Builder
	b.Grow(MaxHostSecretPatternLength)
	b.WriteByte('^')
	for i := 0; i < len(prefix); {
		r, width := utf8.DecodeRuneInString(prefix[i:])
		if r == utf8.RuneError && width == 1 {
			break
		}
		quoted := regexp.QuoteMeta(prefix[i : i+width])
		if b.Len()+len(quoted) > MaxHostSecretPatternLength {
			break
		}
		b.WriteString(quoted)
		i += width
	}
	if b.Len() == 1 {
		return ""
	}
	return b.String()
}

// GetAuthIdentity retrieves identity claims from the host's auth registry.
func (c *HostServiceClient) GetAuthIdentity(ctx context.Context, handler, scope string) (*proto.Claims, error) {
	resp, err := c.client.GetAuthIdentity(ctx, &proto.GetAuthIdentityRequest{
		HandlerName: handler,
		Scope:       scope,
	})
	if err != nil {
		return nil, fmt.Errorf("host GetAuthIdentity: %w", err)
	}
	if resp.Error != "" {
		return nil, fmt.Errorf("host GetAuthIdentity: %s", resp.Error)
	}
	return resp.Claims, nil
}

// ListAuthHandlers lists available auth handlers on the host.
func (c *HostServiceClient) ListAuthHandlers(ctx context.Context) (handlers []string, defaultHandler string, err error) {
	resp, err := c.client.ListAuthHandlers(ctx, &proto.ListAuthHandlersRequest{})
	if err != nil {
		return nil, "", fmt.Errorf("host ListAuthHandlers: %w", err)
	}
	return resp.HandlerNames, resp.DefaultHandler, nil
}

// AuthTokenRequest configures a HostServiceClient.GetAuthTokenWithOptions call.
type AuthTokenRequest struct {
	// Handler names the auth handler to request a token from.
	Handler string `json:"handler" yaml:"handler"`
	// Scope specifies the requested OAuth scope, if applicable.
	Scope string `json:"scope,omitempty" yaml:"scope,omitempty"`
	// MinValidForSeconds is the minimum remaining validity required before
	// triggering a refresh.
	MinValidForSeconds int64 `json:"minValidForSeconds,omitempty" yaml:"minValidForSeconds,omitempty"`
	// ForceRefresh bypasses cached access tokens and forces a fresh token acquisition.
	ForceRefresh bool `json:"forceRefresh,omitempty" yaml:"forceRefresh,omitempty"`
	// Profile selects the auth profile. When empty, GetAuthTokenWithOptions falls
	// back to auth.ProfileFromContext(ctx).
	Profile string `json:"profile,omitempty" yaml:"profile,omitempty"`
	// Hostname selects a specific instance (e.g. a cluster or server) for
	// handlers that advertise auth.CapTokenHostname.
	Hostname string `json:"hostname,omitempty" yaml:"hostname,omitempty"`
}

// GetAuthToken retrieves a valid access token from the host's auth registry.
func (c *HostServiceClient) GetAuthToken(ctx context.Context, handler, scope string, minValidFor int64, forceRefresh bool) (*proto.GetAuthTokenResponse, error) {
	return c.GetAuthTokenWithOptions(ctx, AuthTokenRequest{
		Handler:            handler,
		Scope:              scope,
		MinValidForSeconds: minValidFor,
		ForceRefresh:       forceRefresh,
	})
}

// GetAuthTokenWithOptions retrieves a valid access token from the host's auth
// registry using structured options, including optional per-instance Hostname
// and explicit Profile override (falling back to auth.ProfileFromContext(ctx)
// when opts.Profile is empty).
func (c *HostServiceClient) GetAuthTokenWithOptions(ctx context.Context, opts AuthTokenRequest) (*proto.GetAuthTokenResponse, error) {
	profile := opts.Profile
	if profile == "" {
		profile = auth.ProfileFromContext(ctx)
	}
	resp, err := c.client.GetAuthToken(ctx, &proto.GetAuthTokenRequest{
		HandlerName:        opts.Handler,
		Scope:              opts.Scope,
		MinValidForSeconds: opts.MinValidForSeconds,
		ForceRefresh:       opts.ForceRefresh,
		Profile:            profile,
		Hostname:           opts.Hostname,
	})
	if err != nil {
		return nil, fmt.Errorf("host GetAuthToken: %w", err)
	}
	if resp.Error != "" {
		return nil, fmt.Errorf("host GetAuthToken: %s", resp.Error)
	}
	return resp, nil
}

// GetAuthGroups retrieves group memberships for the authenticated user from
// the host's auth registry, forwarding the active profile from ctx. Entries
// may be group IDs/ObjectIDs or names depending on the handler. If the
// selected handler does not support group queries, the host populates
// GetAuthGroupsResponse.error and this method returns that error. Older hosts
// that do not implement the GetAuthGroups RPC (gRPC Unimplemented) return an
// empty slice and a nil error.
func (c *HostServiceClient) GetAuthGroups(ctx context.Context, handler string) ([]string, error) {
	resp, err := c.client.GetAuthGroups(ctx, &proto.GetAuthGroupsRequest{
		HandlerName: handler,
		Profile:     auth.ProfileFromContext(ctx),
	})
	if err != nil {
		if IsUnimplemented(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("host GetAuthGroups: %w", err)
	}
	if resp.Error != "" {
		return nil, fmt.Errorf("host GetAuthGroups: %s", resp.Error)
	}
	if resp.Groups == nil {
		return []string{}, nil
	}
	return resp.Groups, nil
}

// PromptAuthResponse asks the host to prompt the user for the redirect URL
// produced by an authorization code + PKCE login and returns the pasted
// value. In remote sessions (DevSpaces, Codespaces) the browser redirect
// cannot reach the machine running the plugin; the user pastes the URL they
// landed on instead, and the host captures it via this call.
//
// Call it only during an active Login for handler: the host renders the
// prompt text itself (displaying authURL), checks the paste against
// redirectURI (the expected URL prefix), and never logs the returned value.
//
// Error cases: a non-interactive host returns gRPC Unavailable; user
// cancellation surfaces as Canceled; older hosts return Unimplemented, which
// callers can detect with IsUnimplemented and fall back to another flow.
func (c *HostServiceClient) PromptAuthResponse(ctx context.Context, handler, authURL, redirectURI string) (string, error) {
	resp, err := c.client.PromptAuthResponse(ctx, &proto.PromptAuthResponseRequest{
		HandlerName:      handler,
		AuthorizationUrl: authURL,
		RedirectUri:      redirectURI,
	})
	if err != nil {
		return "", fmt.Errorf("host PromptAuthResponse: %w", err)
	}
	return resp.Value, nil
}

// IsUnimplemented reports whether err is or wraps a gRPC Unimplemented error,
// as gRPC itself returns from older hosts that do not implement a
// HostService RPC. Use it to detect whether PromptAuthResponse (and other
// newer host calls) is available, e.g. to fall back to another flow:
//
//	if _, err := hc.PromptAuthResponse(ctx, ...); err != nil && plugin.IsUnimplemented(err) {
//		// older host: fall back to device code
//	}
func IsUnimplemented(err error) bool {
	for err != nil {
		if s, ok := status.FromError(err); ok && s.Code() == codes.Unimplemented {
			return true
		}
		err = errors.Unwrap(err)
	}
	return false
}
