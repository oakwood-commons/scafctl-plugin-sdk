// Copyright 2025-2026 Oakwood Commons
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHasCapability(t *testing.T) {
	tests := []struct {
		name         string
		capabilities []Capability
		capability   Capability
		expected     bool
	}{
		{
			name:         "found in list",
			capabilities: []Capability{CapScopesOnLogin, CapTenantID, CapHostname},
			capability:   CapTenantID,
			expected:     true,
		},
		{
			name:         "not found in list",
			capabilities: []Capability{CapScopesOnLogin, CapTenantID},
			capability:   CapHostname,
			expected:     false,
		},
		{
			name:         "empty list",
			capabilities: []Capability{},
			capability:   CapScopesOnLogin,
			expected:     false,
		},
		{
			name:         "nil list",
			capabilities: nil,
			capability:   CapScopesOnLogin,
			expected:     false,
		},
		{
			name:         "first element",
			capabilities: []Capability{CapFederatedToken, CapCallbackPort, CapFlowOverride},
			capability:   CapFederatedToken,
			expected:     true,
		},
		{
			name:         "last element",
			capabilities: []Capability{CapFederatedToken, CapCallbackPort, CapFlowOverride},
			capability:   CapFlowOverride,
			expected:     true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, HasCapability(tt.capabilities, tt.capability))
		})
	}
}

func TestCapabilityConstants(t *testing.T) {
	assert.Equal(t, Capability("scopes_on_login"), CapScopesOnLogin)
	assert.Equal(t, Capability("scopes_on_token_request"), CapScopesOnTokenRequest)
	assert.Equal(t, Capability("tenant_id"), CapTenantID)
	assert.Equal(t, Capability("hostname"), CapHostname)
	assert.Equal(t, Capability("federated_token"), CapFederatedToken)
	assert.Equal(t, Capability("callback_port"), CapCallbackPort)
	assert.Equal(t, Capability("flow_override"), CapFlowOverride)
}
