// Copyright 2025-2026 Oakwood Commons
// SPDX-License-Identifier: Apache-2.0

package plugin

import (
	"context"
	"encoding/json"
)

// contextKey is an unexported type for plugin-package context keys.
type contextKey string

const (
	hostClientKey      contextKey = "scafctl.plugin.hostClient"
	requestSettingsKey contextKey = "scafctl.plugin.requestSettings"
)

// WithHostClient returns a new context with the given HostServiceClient attached.
func WithHostClient(ctx context.Context, client *HostServiceClient) context.Context {
	return context.WithValue(ctx, hostClientKey, client)
}

// HostClientFromContext retrieves the HostServiceClient from the context.
// Returns nil if no host client is available (e.g. the host did not start a
// HostService broker).
func HostClientFromContext(ctx context.Context) *HostServiceClient {
	v, _ := ctx.Value(hostClientKey).(*HostServiceClient)
	return v
}

// WithRequestSettings returns a new context carrying per-RPC auth handler
// settings decoded from the incoming request. Passing a nil map clears any
// previously attached request settings so RequestSettingsFromContext returns
// (nil, false).
func WithRequestSettings(ctx context.Context, settings map[string]json.RawMessage) context.Context {
	return context.WithValue(ctx, requestSettingsKey, settings)
}

// RequestSettingsFromContext retrieves per-RPC auth handler settings from the
// context. Returns the settings map and true when per-call settings were sent
// on the request, or nil and false when absent (e.g. older hosts), allowing
// handlers to fall back to ConfigureAuthHandler settings.
func RequestSettingsFromContext(ctx context.Context) (map[string]json.RawMessage, bool) {
	settings, ok := ctx.Value(requestSettingsKey).(map[string]json.RawMessage)
	if !ok || settings == nil {
		return nil, false
	}
	return settings, true
}
