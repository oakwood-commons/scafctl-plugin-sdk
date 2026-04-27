// Copyright 2025-2026 Oakwood Commons
// SPDX-License-Identifier: Apache-2.0

package plugin

import "context"

// contextKey is an unexported type for plugin-package context keys.
type contextKey string

const hostClientKey contextKey = "scafctl.plugin.hostClient"

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
