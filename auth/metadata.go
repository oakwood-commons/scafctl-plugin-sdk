// Copyright 2025-2026 Oakwood Commons
// SPDX-License-Identifier: Apache-2.0

package auth

import "time"

// HandlerMetadata is the canonical persisted metadata for an auth session,
// shared by core and all plugin auth handlers.
//
// The typed fields cover everything common across handlers. Handler-specific
// values (for example the entra tenant ID or the gcp project) go in the open
// Metadata map, keyed by exported constants defined in each plugin to avoid
// key drift.
type HandlerMetadata struct {
	Claims        *Claims        `json:"claims,omitempty" yaml:"claims,omitempty" doc:"Normalized identity claims for the session"`
	ExpiresAt     time.Time      `json:"expiresAt,omitempty" yaml:"expiresAt,omitempty" doc:"Time the session or refresh credential expires"`
	Scopes        []string       `json:"scopes,omitempty" yaml:"scopes,omitempty" doc:"Scopes granted to the session"`
	LastLoginFlow Flow           `json:"lastLoginFlow,omitempty" yaml:"lastLoginFlow,omitempty" doc:"Auth flow that produced the session"`
	SessionID     string         `json:"sessionId,omitempty" yaml:"sessionId,omitempty" doc:"Opaque session identifier"`
	LastRefresh   time.Time      `json:"lastRefresh,omitempty" yaml:"lastRefresh,omitempty" doc:"Time the session was last refreshed"`
	ClientID      string         `json:"clientId,omitempty" yaml:"clientId,omitempty" doc:"Application/client ID used for the session"`
	Metadata      map[string]any `json:"metadata,omitempty" yaml:"metadata,omitempty" doc:"Handler-specific fields keyed by exported plugin constants"`
}

// MetaString returns the string value for key, or "" if the key is absent or
// its value is not a string. It uses a value receiver so it can be called on
// non-addressable values.
func (m HandlerMetadata) MetaString(key string) string {
	s, _ := m.Metadata[key].(string)
	return s
}

// SetMeta sets a handler-specific key, allocating the Metadata map on first use.
func (m *HandlerMetadata) SetMeta(key string, value any) {
	if m.Metadata == nil {
		m.Metadata = make(map[string]any)
	}
	m.Metadata[key] = value
}
