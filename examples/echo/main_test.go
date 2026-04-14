// Copyright 2025-2026 Oakwood Commons
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"testing"

	"github.com/oakwood-commons/scafctl-plugin-sdk/plugin"
	"github.com/oakwood-commons/scafctl-plugin-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newEcho() *EchoPlugin { return &EchoPlugin{} }

func TestGetProviders(t *testing.T) {
	names, err := newEcho().GetProviders(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []string{"echo"}, names)
}

func TestGetProviderDescriptor(t *testing.T) {
	tests := []struct {
		name        string
		provider    string
		wantErr     string
		wantName    string
		wantVersion string
	}{
		{
			name:        "valid provider",
			provider:    "echo",
			wantName:    "echo",
			wantVersion: "1.0.0",
		},
		{
			name:     "unknown provider",
			provider: "nope",
			wantErr:  "unknown provider: nope",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			desc, err := newEcho().GetProviderDescriptor(context.Background(), tc.provider)
			if tc.wantErr != "" {
				require.EqualError(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantName, desc.Name)
			assert.Equal(t, tc.wantVersion, desc.Version.String())
			assert.Equal(t, "v1", desc.APIVersion)
			assert.Equal(t, "Echo Provider", desc.DisplayName)
			assert.Contains(t, desc.Capabilities, provider.CapabilityTransform)
			assert.NotNil(t, desc.Schema)
			assert.NotNil(t, desc.OutputSchemas[provider.CapabilityTransform])
		})
	}
}

func TestExecuteProvider(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		input    map[string]any
		wantErr  string
		wantData map[string]any
	}{
		{
			name:     "echo message",
			provider: "echo",
			input:    map[string]any{"message": "hello"},
			wantData: map[string]any{"echoed": "hello"},
		},
		{
			name:     "echo uppercase",
			provider: "echo",
			input:    map[string]any{"message": "hello", "uppercase": true},
			wantData: map[string]any{"echoed": "HELLO"},
		},
		{
			name:     "echo without uppercase flag",
			provider: "echo",
			input:    map[string]any{"message": "world", "uppercase": false},
			wantData: map[string]any{"echoed": "world"},
		},
		{
			name:     "unknown provider",
			provider: "nope",
			input:    map[string]any{"message": "hi"},
			wantErr:  "unknown provider: nope",
		},
		{
			name:     "missing message",
			provider: "echo",
			input:    map[string]any{},
			wantErr:  "message must be a string",
		},
		{
			name:     "non-string message",
			provider: "echo",
			input:    map[string]any{"message": 42},
			wantErr:  "message must be a string",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, err := newEcho().ExecuteProvider(context.Background(), tc.provider, tc.input)
			if tc.wantErr != "" {
				require.EqualError(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantData, out.Data)
		})
	}
}

func TestDescribeWhatIf(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		input    map[string]any
		wantErr  string
		want     string
	}{
		{
			name:     "with message",
			provider: "echo",
			input:    map[string]any{"message": "hi"},
			want:     "Would echo \"hi\"",
		},
		{
			name:     "empty message",
			provider: "echo",
			input:    map[string]any{},
			want:     "Would echo message",
		},
		{
			name:     "unknown provider",
			provider: "nope",
			input:    map[string]any{},
			wantErr:  "unknown provider: nope",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			desc, err := newEcho().DescribeWhatIf(context.Background(), tc.provider, tc.input)
			if tc.wantErr != "" {
				require.EqualError(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, desc)
		})
	}
}

func TestConfigureProvider(t *testing.T) {
	err := newEcho().ConfigureProvider(context.Background(), "echo", plugin.ProviderConfig{})
	require.NoError(t, err)
}

func TestExecuteProviderStream(t *testing.T) {
	err := newEcho().ExecuteProviderStream(context.Background(), "echo", nil, nil)
	assert.ErrorIs(t, err, plugin.ErrStreamingNotSupported)
}

func TestExtractDependencies(t *testing.T) {
	deps, err := newEcho().ExtractDependencies(context.Background(), "echo", nil)
	require.NoError(t, err)
	assert.Nil(t, deps)
}

func TestStopProvider(t *testing.T) {
	err := newEcho().StopProvider(context.Background(), "echo")
	require.NoError(t, err)
}
