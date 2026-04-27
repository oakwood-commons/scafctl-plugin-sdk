// Copyright 2025-2026 Oakwood Commons
// SPDX-License-Identifier: Apache-2.0

package plugin

import (
	"context"
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
