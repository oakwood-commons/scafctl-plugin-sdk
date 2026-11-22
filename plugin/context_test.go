// Copyright 2025-2026 Oakwood Commons
// SPDX-License-Identifier: Apache-2.0

package plugin

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHostClientFromContext_NoClientInContext(t *testing.T) {
	ctx := context.Background()
	got := HostClientFromContext(ctx)
	assert.Nil(t, got)
}

func TestWithHostClient_RoundTrip(t *testing.T) {
	conn, cleanup := startFakeHostService(t)
	defer cleanup()

	client := NewHostServiceClient(conn)
	ctx := WithHostClient(context.Background(), client)
	got := HostClientFromContext(ctx)
	require.NotNil(t, got)

	// Verify it's functional by calling through.
	names, defaultHandler, err := got.ListAuthHandlers(ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{"gh", "entra"}, names)
	assert.Equal(t, "gh", defaultHandler)
}

func TestRequestSettingsFromContext_Absent(t *testing.T) {
	t.Parallel()
	got, ok := RequestSettingsFromContext(context.Background())
	assert.False(t, ok)
	assert.Nil(t, got)
}

func TestWithRequestSettings_RoundTrip(t *testing.T) {
	t.Parallel()
	in := map[string]json.RawMessage{
		"clientId": json.RawMessage(`"abc-123"`),
		"port":     json.RawMessage(`8443`),
	}
	ctx := WithRequestSettings(context.Background(), in)
	got, ok := RequestSettingsFromContext(ctx)
	require.True(t, ok)
	require.Len(t, got, 2)
	assert.JSONEq(t, `"abc-123"`, string(got["clientId"]))
	assert.JSONEq(t, `8443`, string(got["port"]))

	// Explicit empty map signals explicit empty per-call settings.
	emptyCtx := WithRequestSettings(ctx, map[string]json.RawMessage{})
	gotEmpty, emptyOK := RequestSettingsFromContext(emptyCtx)
	assert.True(t, emptyOK)
	assert.NotNil(t, gotEmpty)
	assert.Empty(t, gotEmpty)

	// Passing nil clears any previously attached settings.
	clearedCtx := WithRequestSettings(ctx, nil)
	gotCleared, clearedOK := RequestSettingsFromContext(clearedCtx)
	assert.False(t, clearedOK)
	assert.Nil(t, gotCleared)
}
