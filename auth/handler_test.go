// Copyright 2025-2026 Oakwood Commons
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestFlowConstants(t *testing.T) {
	assert.Equal(t, Flow("device_code"), FlowDeviceCode)
	assert.Equal(t, Flow("interactive"), FlowInteractive)
	assert.Equal(t, Flow("service_principal"), FlowServicePrincipal)
	assert.Equal(t, Flow("workload_identity"), FlowWorkloadIdentity)
	assert.Equal(t, Flow("pat"), FlowPAT)
	assert.Equal(t, Flow("metadata"), FlowMetadata)
	assert.Equal(t, Flow("gcloud_adc"), FlowGcloudADC)
	assert.Equal(t, Flow("github_app"), FlowGitHubApp)
	assert.Equal(t, Flow("client_credentials"), FlowClientCredentials)
}

func TestDefaultMinValidFor(t *testing.T) {
	assert.Equal(t, 60*time.Second, DefaultMinValidFor)
}

func TestToken_IsValidFor(t *testing.T) {
	tests := []struct {
		name     string
		token    *Token
		duration time.Duration
		expected bool
	}{
		{name: "nil token", token: nil, duration: 0, expected: false},
		{name: "empty access token", token: &Token{ExpiresAt: time.Now().Add(time.Hour)}, duration: 0, expected: false},
		{name: "zero expires at", token: &Token{AccessToken: "tok"}, duration: 0, expected: false},
		{name: "valid for 1h, check 30m", token: &Token{AccessToken: "tok", ExpiresAt: time.Now().Add(time.Hour)}, duration: 30 * time.Minute, expected: true},
		{name: "valid for 1m, check 1h", token: &Token{AccessToken: "tok", ExpiresAt: time.Now().Add(time.Minute)}, duration: time.Hour, expected: false},
		{name: "expired token", token: &Token{AccessToken: "tok", ExpiresAt: time.Now().Add(-time.Hour)}, duration: 0, expected: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.token.IsValidFor(tt.duration))
		})
	}
}

func TestToken_IsExpired(t *testing.T) {
	assert.True(t, (&Token{AccessToken: "tok", ExpiresAt: time.Now().Add(-time.Hour)}).IsExpired())
	assert.False(t, (&Token{AccessToken: "tok", ExpiresAt: time.Now().Add(time.Hour)}).IsExpired())
}

func TestToken_TimeUntilExpiry(t *testing.T) {
	tests := []struct {
		name      string
		token     *Token
		expectGt0 bool
	}{
		{name: "nil token", token: nil, expectGt0: false},
		{name: "expired token", token: &Token{ExpiresAt: time.Now().Add(-time.Hour)}, expectGt0: false},
		{name: "zero time", token: &Token{}, expectGt0: false},
		{name: "valid token", token: &Token{ExpiresAt: time.Now().Add(time.Hour)}, expectGt0: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := tt.token.TimeUntilExpiry()
			if tt.expectGt0 {
				assert.Greater(t, d, time.Duration(0))
			} else {
				assert.Equal(t, time.Duration(0), d)
			}
		})
	}
}

func TestCachedTokenInfo_TimeUntilExpiry(t *testing.T) {
	expired := &CachedTokenInfo{ExpiresAt: time.Now().Add(-time.Hour)}
	assert.Equal(t, time.Duration(0), expired.TimeUntilExpiry())

	valid := &CachedTokenInfo{ExpiresAt: time.Now().Add(time.Hour)}
	assert.Greater(t, valid.TimeUntilExpiry(), time.Duration(0))

	zero := &CachedTokenInfo{}
	assert.Equal(t, time.Duration(0), zero.TimeUntilExpiry())

	var nilInfo *CachedTokenInfo
	assert.Equal(t, time.Duration(0), nilInfo.TimeUntilExpiry())
}

func TestIdentityTypeConstants(t *testing.T) {
	assert.Equal(t, IdentityType("user"), IdentityTypeUser)
	assert.Equal(t, IdentityType("service-principal"), IdentityTypeServicePrincipal)
	assert.Equal(t, IdentityType("workload-identity"), IdentityTypeWorkloadIdentity)
}
