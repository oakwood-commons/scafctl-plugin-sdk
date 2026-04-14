// Copyright 2025-2026 Oakwood Commons
// SPDX-License-Identifier: Apache-2.0

package testutil

import (
	"context"
	"errors"
	"testing"

	"github.com/oakwood-commons/scafctl-plugin-sdk/plugin"
	"github.com/oakwood-commons/scafctl-plugin-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMockProviderPlugin_ImplementsInterface(t *testing.T) {
	var _ plugin.ProviderPlugin = &MockProviderPlugin{}
}

func TestMockProviderPlugin_Defaults(t *testing.T) {
	m := &MockProviderPlugin{}
	ctx := context.Background()

	providers, err := m.GetProviders(ctx)
	assert.NoError(t, err)
	assert.Nil(t, providers)

	_, err = m.GetProviderDescriptor(ctx, "test")
	assert.Error(t, err)

	err = m.ConfigureProvider(ctx, "test", plugin.ProviderConfig{})
	assert.NoError(t, err)

	_, err = m.ExecuteProvider(ctx, "test", nil)
	assert.Error(t, err)

	err = m.ExecuteProviderStream(ctx, "test", nil, nil)
	assert.ErrorIs(t, err, plugin.ErrStreamingNotSupported)

	desc, err := m.DescribeWhatIf(ctx, "test", nil)
	assert.NoError(t, err)
	assert.Empty(t, desc)

	deps, err := m.ExtractDependencies(ctx, "test", nil)
	assert.NoError(t, err)
	assert.Nil(t, deps)

	err = m.StopProvider(ctx, "test")
	assert.NoError(t, err)
}

func TestMockProviderPlugin_CustomFuncs(t *testing.T) {
	ctx := context.Background()
	m := &MockProviderPlugin{
		GetProvidersFunc: func(_ context.Context) ([]string, error) {
			return []string{"echo"}, nil
		},
		GetProviderDescriptorFunc: func(_ context.Context, name string) (*provider.Descriptor, error) {
			return &provider.Descriptor{Name: name}, nil
		},
		ConfigureProviderFunc: func(_ context.Context, _ string, _ plugin.ProviderConfig) error {
			return errors.New("config error")
		},
		ExecuteProviderFunc: func(_ context.Context, _ string, input map[string]any) (*provider.Output, error) {
			return &provider.Output{Data: input}, nil
		},
		ExecuteProviderStreamFunc: func(_ context.Context, _ string, _ map[string]any, cb func(plugin.StreamChunk)) error {
			cb(plugin.StreamChunk{Stdout: []byte("hello")})
			return nil
		},
		DescribeWhatIfFunc: func(_ context.Context, _ string, _ map[string]any) (string, error) {
			return "would do thing", nil
		},
		ExtractDependenciesFunc: func(_ context.Context, _ string, _ map[string]any) ([]string, error) {
			return []string{"dep1"}, nil
		},
		StopProviderFunc: func(_ context.Context, _ string) error {
			return errors.New("stop error")
		},
	}

	providers, err := m.GetProviders(ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{"echo"}, providers)

	desc, err := m.GetProviderDescriptor(ctx, "echo")
	require.NoError(t, err)
	assert.Equal(t, "echo", desc.Name)

	err = m.ConfigureProvider(ctx, "echo", plugin.ProviderConfig{})
	assert.EqualError(t, err, "config error")

	output, err := m.ExecuteProvider(ctx, "echo", map[string]any{"key": "val"})
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"key": "val"}, output.Data)

	var chunks []plugin.StreamChunk
	err = m.ExecuteProviderStream(ctx, "echo", nil, func(c plugin.StreamChunk) {
		chunks = append(chunks, c)
	})
	require.NoError(t, err)
	assert.Len(t, chunks, 1)

	whatif, err := m.DescribeWhatIf(ctx, "echo", nil)
	require.NoError(t, err)
	assert.Equal(t, "would do thing", whatif)

	deps, err := m.ExtractDependencies(ctx, "echo", nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"dep1"}, deps)

	err = m.StopProvider(ctx, "echo")
	assert.EqualError(t, err, "stop error")
}
