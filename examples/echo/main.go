// Copyright 2025-2026 Oakwood Commons
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/oakwood-commons/scafctl-plugin-sdk/plugin"
	"github.com/oakwood-commons/scafctl-plugin-sdk/provider"
	"github.com/oakwood-commons/scafctl-plugin-sdk/provider/schemahelper"
)

// EchoPlugin implements a simple echo plugin that returns its input.
type EchoPlugin struct{}

func (p *EchoPlugin) GetProviders(_ context.Context) ([]string, error) {
	return []string{"echo"}, nil
}

func (p *EchoPlugin) GetProviderDescriptor(_ context.Context, providerName string) (*provider.Descriptor, error) {
	if providerName != "echo" {
		return nil, fmt.Errorf("unknown provider: %s", providerName)
	}
	maxLen := 1000
	return &provider.Descriptor{
		Name:         "echo",
		DisplayName:  "Echo Provider",
		Description:  "A simple provider that echoes its input",
		APIVersion:   "v1",
		Version:      semver.MustParse("1.0.0"),
		Category:     "utility",
		Capabilities: []provider.Capability{provider.CapabilityTransform},
		Schema: schemahelper.ObjectSchema(
			[]string{"message"},
			map[string]*jsonschema.Schema{
				"message": schemahelper.StringProp("The message to echo",
					schemahelper.WithExample("Hello, World!"),
					schemahelper.WithMaxLength(maxLen),
				),
				"uppercase": schemahelper.BoolProp("Whether to convert the message to uppercase",
					schemahelper.WithDefault(json.RawMessage("false")),
				),
			},
		),
		OutputSchemas: map[provider.Capability]*jsonschema.Schema{
			provider.CapabilityTransform: schemahelper.ObjectSchema(nil,
				map[string]*jsonschema.Schema{
					"echoed": schemahelper.StringProp("The echoed message"),
				},
			),
		},
	}, nil
}

func (p *EchoPlugin) ExecuteProvider(_ context.Context, providerName string, input map[string]any) (*provider.Output, error) {
	if providerName != "echo" {
		return nil, fmt.Errorf("unknown provider: %s", providerName)
	}
	message, ok := input["message"].(string)
	if !ok {
		return nil, fmt.Errorf("message must be a string")
	}
	uppercase, _ := input["uppercase"].(bool)
	result := message
	if uppercase {
		result = strings.ToUpper(result)
	}
	return &provider.Output{Data: map[string]any{"echoed": result}}, nil
}

func (p *EchoPlugin) DescribeWhatIf(_ context.Context, providerName string, input map[string]any) (string, error) {
	if providerName != "echo" {
		return "", fmt.Errorf("unknown provider: %s", providerName)
	}
	message, _ := input["message"].(string)
	if message != "" {
		return fmt.Sprintf("Would echo %q", message), nil
	}
	return "Would echo message", nil
}

func (p *EchoPlugin) ConfigureProvider(_ context.Context, _ string, _ plugin.ProviderConfig) error {
	return nil
}

func (p *EchoPlugin) ExecuteProviderStream(_ context.Context, _ string, _ map[string]any, _ func(plugin.StreamChunk)) error {
	return plugin.ErrStreamingNotSupported
}

func (p *EchoPlugin) ExtractDependencies(_ context.Context, _ string, _ map[string]any) ([]string, error) {
	return nil, nil
}

func (p *EchoPlugin) StopProvider(_ context.Context, _ string) error {
	return nil
}

func main() {
	plugin.Serve(&EchoPlugin{})
}
