// Copyright 2025-2026 Oakwood Commons
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestClaims_IsEmpty(t *testing.T) {
	tests := []struct {
		name     string
		claims   *Claims
		expected bool
	}{
		{name: "nil claims", claims: nil, expected: true},
		{name: "zero value", claims: &Claims{}, expected: true},
		{name: "only issuer set", claims: &Claims{Issuer: "https://example.com"}, expected: true},
		{name: "subject set", claims: &Claims{Subject: "user@example.com"}, expected: false},
		{name: "email set", claims: &Claims{Email: "user@example.com"}, expected: false},
		{name: "name set", claims: &Claims{Name: "Jane Doe"}, expected: false},
		{name: "username set", claims: &Claims{Username: "janedoe"}, expected: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.claims.IsEmpty())
		})
	}
}

func TestClaims_DisplayIdentity(t *testing.T) {
	tests := []struct {
		name     string
		claims   *Claims
		expected string
	}{
		{name: "nil claims", claims: nil, expected: ""},
		{name: "empty claims", claims: &Claims{}, expected: ""},
		{name: "email priority", claims: &Claims{Email: "a@b.com", Username: "user", Name: "N", Subject: "sub"}, expected: "a@b.com"},
		{name: "username when no email", claims: &Claims{Username: "user", Name: "N", Subject: "sub"}, expected: "user"},
		{name: "name when no email/username", claims: &Claims{Name: "N", Subject: "sub"}, expected: "N"},
		{name: "subject as fallback", claims: &Claims{Subject: "sub"}, expected: "sub"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.claims.DisplayIdentity())
		})
	}
}

func TestClaims_FullyPopulated(t *testing.T) {
	now := time.Now()
	c := &Claims{
		Issuer: "https://issuer.example.com", Subject: "subject-123",
		TenantID: "tenant-abc", ObjectID: "obj-456", ClientID: "client-789",
		Email: "test@example.com", Name: "Test User", Username: "testuser",
		IssuedAt: now, ExpiresAt: now.Add(time.Hour),
	}
	assert.False(t, c.IsEmpty())
	assert.Equal(t, "test@example.com", c.DisplayIdentity())
}
