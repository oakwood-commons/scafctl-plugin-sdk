// Copyright 2025-2026 Oakwood Commons
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseFlow(t *testing.T) {
	tests := []struct {
		name        string
		flowStr     string
		handlerName string
		expected    Flow
		wantErr     bool
		errContains string
	}{
		{name: "empty string", flowStr: "", handlerName: "github", expected: ""},
		{name: "device_code", flowStr: "device_code", handlerName: "", expected: FlowDeviceCode},
		{name: "device-code", flowStr: "device-code", handlerName: "", expected: FlowDeviceCode},
		{name: "devicecode", flowStr: "devicecode", handlerName: "", expected: FlowDeviceCode},
		{name: "interactive", flowStr: "interactive", handlerName: "", expected: FlowInteractive},
		{name: "service_principal", flowStr: "service_principal", handlerName: "", expected: FlowServicePrincipal},
		{name: "service-principal", flowStr: "service-principal", handlerName: "", expected: FlowServicePrincipal},
		{name: "sp", flowStr: "sp", handlerName: "", expected: FlowServicePrincipal},
		{name: "workload_identity", flowStr: "workload_identity", handlerName: "", expected: FlowWorkloadIdentity},
		{name: "wi", flowStr: "wi", handlerName: "", expected: FlowWorkloadIdentity},
		{name: "pat", flowStr: "pat", handlerName: "", expected: FlowPAT},
		{name: "metadata", flowStr: "metadata", handlerName: "", expected: FlowMetadata},
		{name: "gcloud_adc", flowStr: "gcloud_adc", handlerName: "", expected: FlowGcloudADC},
		{name: "adc", flowStr: "adc", handlerName: "", expected: FlowGcloudADC},
		{name: "github_app", flowStr: "github_app", handlerName: "", expected: FlowGitHubApp},
		{name: "app", flowStr: "app", handlerName: "", expected: FlowGitHubApp},
		{name: "client_credentials", flowStr: "client_credentials", handlerName: "", expected: FlowClientCredentials},
		{name: "cc", flowStr: "cc", handlerName: "", expected: FlowClientCredentials},
		{name: "case insensitive", flowStr: "DEVICE_CODE", handlerName: "", expected: FlowDeviceCode},
		// Error cases with handler-specific messages
		{name: "unknown for github", flowStr: "bad", handlerName: "github", wantErr: true, errContains: "valid for github"},
		{name: "unknown for gcp", flowStr: "bad", handlerName: "gcp", wantErr: true, errContains: "valid for gcp"},
		{name: "unknown for entra", flowStr: "bad", handlerName: "entra", wantErr: true, errContains: "valid for entra"},
		{name: "unknown for other", flowStr: "bad", handlerName: "other", wantErr: true, errContains: "unknown flow: bad"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseFlow(tt.flowStr, tt.handlerName)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}
