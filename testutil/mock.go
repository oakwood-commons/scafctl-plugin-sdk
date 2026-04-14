// Copyright 2025-2026 Oakwood Commons
// SPDX-License-Identifier: Apache-2.0

// Package testutil provides test helpers for plugin integration tests.
package testutil

import (
	"context"
	"fmt"

	"github.com/oakwood-commons/scafctl-plugin-sdk/plugin"
	"github.com/oakwood-commons/scafctl-plugin-sdk/provider"
)

// MockProviderPlugin is a configurable mock that implements plugin.ProviderPlugin.
type MockProviderPlugin struct {
	GetProvidersFunc          func(ctx context.Context) ([]string, error)
	GetProviderDescriptorFunc func(ctx context.Context, providerName string) (*provider.Descriptor, error)
	ConfigureProviderFunc     func(ctx context.Context, providerName string, cfg plugin.ProviderConfig) error
	ExecuteProviderFunc       func(ctx context.Context, providerName string, input map[string]any) (*provider.Output, error)
	ExecuteProviderStreamFunc func(ctx context.Context, providerName string, input map[string]any, cb func(plugin.StreamChunk)) error
	DescribeWhatIfFunc        func(ctx context.Context, providerName string, input map[string]any) (string, error)
	ExtractDependenciesFunc   func(ctx context.Context, providerName string, inputs map[string]any) ([]string, error)
	StopProviderFunc          func(ctx context.Context, providerName string) error
}

func (m *MockProviderPlugin) GetProviders(ctx context.Context) ([]string, error) {
	if m.GetProvidersFunc != nil {
		return m.GetProvidersFunc(ctx)
	}
	return nil, nil
}

func (m *MockProviderPlugin) GetProviderDescriptor(ctx context.Context, providerName string) (*provider.Descriptor, error) {
	if m.GetProviderDescriptorFunc != nil {
		return m.GetProviderDescriptorFunc(ctx, providerName)
	}
	return nil, fmt.Errorf("unknown provider: %s", providerName)
}

func (m *MockProviderPlugin) ConfigureProvider(ctx context.Context, providerName string, cfg plugin.ProviderConfig) error {
	if m.ConfigureProviderFunc != nil {
		return m.ConfigureProviderFunc(ctx, providerName, cfg)
	}
	return nil
}

func (m *MockProviderPlugin) ExecuteProvider(ctx context.Context, providerName string, input map[string]any) (*provider.Output, error) {
	if m.ExecuteProviderFunc != nil {
		return m.ExecuteProviderFunc(ctx, providerName, input)
	}
	return nil, fmt.Errorf("not implemented")
}

func (m *MockProviderPlugin) ExecuteProviderStream(ctx context.Context, providerName string, input map[string]any, cb func(plugin.StreamChunk)) error {
	if m.ExecuteProviderStreamFunc != nil {
		return m.ExecuteProviderStreamFunc(ctx, providerName, input, cb)
	}
	return plugin.ErrStreamingNotSupported
}

func (m *MockProviderPlugin) DescribeWhatIf(ctx context.Context, providerName string, input map[string]any) (string, error) {
	if m.DescribeWhatIfFunc != nil {
		return m.DescribeWhatIfFunc(ctx, providerName, input)
	}
	return "", nil
}

func (m *MockProviderPlugin) ExtractDependencies(ctx context.Context, providerName string, inputs map[string]any) ([]string, error) {
	if m.ExtractDependenciesFunc != nil {
		return m.ExtractDependenciesFunc(ctx, providerName, inputs)
	}
	return nil, nil
}

func (m *MockProviderPlugin) StopProvider(ctx context.Context, providerName string) error {
	if m.StopProviderFunc != nil {
		return m.StopProviderFunc(ctx, providerName)
	}
	return nil
}
