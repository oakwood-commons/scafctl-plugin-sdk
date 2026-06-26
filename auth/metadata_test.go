// Copyright 2025-2026 Oakwood Commons
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandlerMetadata_RoundTrip(t *testing.T) {
	expires := time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)
	refresh := time.Date(2026, 6, 26, 11, 0, 0, 0, time.UTC)

	tests := []struct {
		name string
		md   HandlerMetadata
	}{
		{
			name: "zero value",
			md:   HandlerMetadata{},
		},
		{
			name: "typed fields only",
			md: HandlerMetadata{
				Claims:        &Claims{Subject: "user@example.com", Email: "user@example.com"},
				ExpiresAt:     expires,
				Scopes:        []string{"openid", "profile"},
				LastLoginFlow: FlowDeviceCode,
				SessionID:     "session-123",
				LastRefresh:   refresh,
				ClientID:      "client-789",
			},
		},
		{
			name: "with handler-specific metadata",
			md: HandlerMetadata{
				LastLoginFlow: FlowServicePrincipal,
				ClientID:      "client-789",
				Metadata: map[string]any{
					"tenantId": "tenant-abc",
					"project":  "my-gcp-project",
				},
			},
		},
		{
			name: "with nested structured metadata",
			md: HandlerMetadata{
				Metadata: map[string]any{
					"validationIssues": map[string]any{
						"groups": []any{"group-a", "group-b"},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.md)
			require.NoError(t, err)

			var got HandlerMetadata
			require.NoError(t, json.Unmarshal(data, &got))

			assert.Equal(t, tt.md, got)
		})
	}
}

func TestHandlerMetadata_CanonicalJSONTags(t *testing.T) {
	md := HandlerMetadata{
		Claims:        &Claims{Subject: "user@example.com"},
		ExpiresAt:     time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC),
		Scopes:        []string{"openid"},
		LastLoginFlow: FlowDeviceCode,
		SessionID:     "session-123",
		LastRefresh:   time.Date(2026, 6, 26, 11, 0, 0, 0, time.UTC),
		ClientID:      "client-789",
		Metadata:      map[string]any{"tenantId": "tenant-abc"},
	}

	data, err := json.Marshal(md)
	require.NoError(t, err)

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &raw))

	for _, key := range []string{
		"claims", "expiresAt", "scopes", "lastLoginFlow",
		"sessionId", "lastRefresh", "clientId", "metadata",
	} {
		_, ok := raw[key]
		assert.Truef(t, ok, "expected canonical JSON key %q", key)
	}
}

func TestHandlerMetadata_OmitEmpty(t *testing.T) {
	data, err := json.Marshal(HandlerMetadata{})
	require.NoError(t, err)

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &raw))

	// Pointer, slice, string, and map fields are omitted when empty. The
	// time.Time fields are not (encoding/json does not treat a zero time.Time
	// as empty), matching the behavior of the sibling Claims type.
	for _, key := range []string{"claims", "scopes", "lastLoginFlow", "sessionId", "clientId", "metadata"} {
		_, ok := raw[key]
		assert.Falsef(t, ok, "expected empty field %q to be omitted", key)
	}
	for _, key := range []string{"expiresAt", "lastRefresh"} {
		_, ok := raw[key]
		assert.Truef(t, ok, "expected zero time.Time field %q to be present (not omitted)", key)
	}
}

func TestHandlerMetadata_MetaString(t *testing.T) {
	tests := []struct {
		name     string
		metadata map[string]any
		key      string
		expected string
	}{
		{name: "present string", metadata: map[string]any{"tenantId": "tenant-abc"}, key: "tenantId", expected: "tenant-abc"},
		{name: "absent key", metadata: map[string]any{"tenantId": "tenant-abc"}, key: "project", expected: ""},
		{name: "nil map", metadata: nil, key: "tenantId", expected: ""},
		{name: "wrong type int", metadata: map[string]any{"count": 5}, key: "count", expected: ""},
		{name: "wrong type nested map", metadata: map[string]any{"obj": map[string]any{"a": "b"}}, key: "obj", expected: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			md := HandlerMetadata{Metadata: tt.metadata}
			assert.Equal(t, tt.expected, md.MetaString(tt.key))
		})
	}
}

func TestHandlerMetadata_SetMeta(t *testing.T) {
	t.Run("allocates map on first use", func(t *testing.T) {
		var md HandlerMetadata
		require.Nil(t, md.Metadata)

		md.SetMeta("tenantId", "tenant-abc")

		require.NotNil(t, md.Metadata)
		assert.Equal(t, "tenant-abc", md.MetaString("tenantId"))
	})

	t.Run("overwrites existing key", func(t *testing.T) {
		md := HandlerMetadata{Metadata: map[string]any{"tenantId": "old"}}
		md.SetMeta("tenantId", "new")
		assert.Equal(t, "new", md.MetaString("tenantId"))
	})

	t.Run("preserves other keys", func(t *testing.T) {
		md := HandlerMetadata{Metadata: map[string]any{"project": "p1"}}
		md.SetMeta("tenantId", "tenant-abc")
		assert.Equal(t, "p1", md.MetaString("project"))
		assert.Equal(t, "tenant-abc", md.MetaString("tenantId"))
	})
}
