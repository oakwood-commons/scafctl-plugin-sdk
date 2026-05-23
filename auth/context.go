// Copyright 2025-2026 Oakwood Commons
// SPDX-License-Identifier: Apache-2.0

package auth

import "context"

// profileKeyType is an unexported type for the profile context key,
// preventing collisions with keys from other packages.
type profileKeyType struct{}

var profileKey profileKeyType

// WithProfile returns a new context with the given profile name attached.
// An empty string represents the default (unnamed) profile.
func WithProfile(ctx context.Context, profile string) context.Context {
	return context.WithValue(ctx, profileKey, profile)
}

// ProfileFromContext retrieves the profile name from the context.
// Returns an empty string if no profile is set (default profile).
func ProfileFromContext(ctx context.Context) string {
	v, _ := ctx.Value(profileKey).(string)
	return v
}
