// Copyright 2025-2026 Oakwood Commons
// SPDX-License-Identifier: Apache-2.0

// Package auth provides authentication handler types and utilities for scafctl plugins.
package auth

import (
	"context"
	"time"
)

// Flow represents an authentication flow type.
type Flow string

const (
	FlowDeviceCode        Flow = "device_code"
	FlowInteractive       Flow = "interactive"
	FlowServicePrincipal  Flow = "service_principal"
	FlowWorkloadIdentity  Flow = "workload_identity"
	FlowPAT               Flow = "pat"
	FlowMetadata          Flow = "metadata"
	FlowGcloudADC         Flow = "gcloud_adc"
	FlowGitHubApp         Flow = "github_app"
	FlowClientCredentials Flow = "client_credentials"
)

// DefaultMinValidFor is the default minimum validity duration for tokens.
const DefaultMinValidFor = 60 * time.Second

// LoginOptions configures the login process.
type LoginOptions struct {
	TenantID           string                                          `json:"tenantId,omitempty" yaml:"tenantId,omitempty"`
	Scopes             []string                                        `json:"scopes,omitempty" yaml:"scopes,omitempty"`
	Flow               Flow                                            `json:"flow,omitempty" yaml:"flow,omitempty"`
	Timeout            time.Duration                                   `json:"timeout,omitempty" yaml:"timeout,omitempty"`
	CallbackPort       int                                             `json:"callbackPort,omitempty" yaml:"callbackPort,omitempty"`
	DeviceCodeCallback func(userCode, verificationURI, message string) `json:"-" yaml:"-"`
}

// TokenOptions configures token acquisition.
type TokenOptions struct {
	Scope        string        `json:"scope,omitempty" yaml:"scope,omitempty"`
	MinValidFor  time.Duration `json:"minValidFor,omitempty" yaml:"minValidFor,omitempty"`
	ForceRefresh bool          `json:"forceRefresh,omitempty" yaml:"forceRefresh,omitempty"`
}

// Result contains the result of a successful authentication.
type Result struct {
	Claims    *Claims   `json:"claims,omitempty" yaml:"claims,omitempty"`
	ExpiresAt time.Time `json:"expiresAt,omitempty" yaml:"expiresAt,omitempty"`
}

// IdentityType represents the type of authenticated identity.
type IdentityType string

const (
	IdentityTypeUser             IdentityType = "user"
	IdentityTypeServicePrincipal IdentityType = "service-principal"
	IdentityTypeWorkloadIdentity IdentityType = "workload-identity"
)

// Status represents the current authentication state.
type Status struct {
	Authenticated bool         `json:"authenticated" yaml:"authenticated"`
	Reason        string       `json:"reason,omitempty" yaml:"reason,omitempty"`
	Claims        *Claims      `json:"claims,omitempty" yaml:"claims,omitempty"`
	ExpiresAt     time.Time    `json:"expiresAt,omitempty" yaml:"expiresAt,omitempty"`
	LastRefresh   time.Time    `json:"lastRefresh,omitempty" yaml:"lastRefresh,omitempty"`
	TenantID      string       `json:"tenantId,omitempty" yaml:"tenantId,omitempty"`
	IdentityType  IdentityType `json:"identityType,omitempty" yaml:"identityType,omitempty"`
	ClientID      string       `json:"clientId,omitempty" yaml:"clientId,omitempty"`
	TokenFile     string       `json:"tokenFile,omitempty" yaml:"tokenFile,omitempty"`
	Scopes        []string     `json:"scopes,omitempty" yaml:"scopes,omitempty"`
}

// Token represents a short-lived access token.
type Token struct {
	AccessToken string    `json:"accessToken" yaml:"accessToken"` //nolint:gosec
	TokenType   string    `json:"tokenType" yaml:"tokenType"`
	ExpiresAt   time.Time `json:"expiresAt" yaml:"expiresAt"`
	Scope       string    `json:"scope,omitempty" yaml:"scope,omitempty"`
	CachedAt    time.Time `json:"cachedAt,omitempty" yaml:"cachedAt,omitempty"`
	Flow        Flow      `json:"flow,omitempty" yaml:"flow,omitempty"`
	SessionID   string    `json:"sessionId,omitempty" yaml:"sessionId,omitempty"`
}

// IsValidFor returns true if the token will be valid for at least the specified duration.
func (t *Token) IsValidFor(duration time.Duration) bool {
	if t == nil || t.AccessToken == "" || t.ExpiresAt.IsZero() {
		return false
	}
	return time.Now().Add(duration).Before(t.ExpiresAt)
}

// IsExpired returns true if the token has expired.
func (t *Token) IsExpired() bool {
	return !t.IsValidFor(0)
}

// TimeUntilExpiry returns the duration until the token expires.
func (t *Token) TimeUntilExpiry() time.Duration {
	if t == nil {
		return 0
	}
	return timeUntilExpiry(t.ExpiresAt)
}

// CachedTokenInfo holds display metadata for a cached token.
type CachedTokenInfo struct {
	Handler     string    `json:"handler" yaml:"handler"`
	TokenKind   string    `json:"tokenKind" yaml:"tokenKind"`
	Scope       string    `json:"scope,omitempty" yaml:"scope,omitempty"`
	TokenType   string    `json:"tokenType,omitempty" yaml:"tokenType,omitempty"`
	Flow        Flow      `json:"flow,omitempty" yaml:"flow,omitempty"`
	Fingerprint string    `json:"fingerprint,omitempty" yaml:"fingerprint,omitempty"`
	ExpiresAt   time.Time `json:"expiresAt,omitempty" yaml:"expiresAt,omitempty"`
	CachedAt    time.Time `json:"cachedAt,omitempty" yaml:"cachedAt,omitempty"`
	IsExpired   bool      `json:"isExpired" yaml:"isExpired"`
	SessionID   string    `json:"sessionId,omitempty" yaml:"sessionId,omitempty"`
}

// TimeUntilExpiry returns the duration until this cached token expires.
func (c *CachedTokenInfo) TimeUntilExpiry() time.Duration {
	if c == nil {
		return 0
	}
	return timeUntilExpiry(c.ExpiresAt)
}

func timeUntilExpiry(expiresAt time.Time) time.Duration {
	if expiresAt.IsZero() {
		return 0
	}
	remaining := time.Until(expiresAt)
	if remaining < 0 {
		return 0
	}
	return remaining
}

// TokenLister is an optional interface for auth handlers that can enumerate cached tokens.
type TokenLister interface {
	ListCachedTokens(ctx context.Context) ([]*CachedTokenInfo, error)
}

// TokenPurger is an optional interface for auth handlers that can remove expired tokens.
type TokenPurger interface {
	PurgeExpiredTokens(ctx context.Context) (int, error)
}
