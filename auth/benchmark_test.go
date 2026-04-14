// Copyright 2025-2026 Oakwood Commons
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"testing"
	"time"
)

func BenchmarkParseFlow(b *testing.B) {
	flows := []string{"device_code", "interactive", "service_principal", "pat", "client_credentials", "gcloud_adc", "github_app"}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		for _, f := range flows {
			_, _ = ParseFlow(f, "")
		}
	}
}

func BenchmarkParseFlow_Aliases(b *testing.B) {
	aliases := []string{"device-code", "service-principal", "sp", "workload-identity", "wi", "gcloud-adc", "adc", "github-app", "app", "cc"}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		for _, a := range aliases {
			_, _ = ParseFlow(a, "")
		}
	}
}

func BenchmarkClaims_DisplayIdentity(b *testing.B) {
	claims := &Claims{
		Issuer:  "https://login.microsoftonline.com",
		Subject: "user@example.com",
		Email:   "user@example.com",
		Name:    "Test User",
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = claims.DisplayIdentity()
	}
}

func BenchmarkClaims_IsEmpty(b *testing.B) {
	b.Run("NonEmpty", func(b *testing.B) {
		claims := &Claims{Subject: "user123", Email: "user@example.com"}
		b.ReportAllocs()
		for b.Loop() {
			_ = claims.IsEmpty()
		}
	})

	b.Run("Empty", func(b *testing.B) {
		claims := &Claims{}
		b.ReportAllocs()
		for b.Loop() {
			_ = claims.IsEmpty()
		}
	})

	b.Run("Nil", func(b *testing.B) {
		var claims *Claims
		b.ReportAllocs()
		for b.Loop() {
			_ = claims.IsEmpty()
		}
	})
}

func BenchmarkToken_IsValid(b *testing.B) {
	token := &Token{
		AccessToken: "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.test",
		TokenType:   "Bearer",
		ExpiresAt:   time.Now().Add(time.Hour),
		Scope:       "read write",
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = token.IsValidFor(0)
	}
}

func BenchmarkHasCapability(b *testing.B) {
	caps := []Capability{CapScopesOnLogin, CapTenantID, CapHostname, CapFederatedToken, CapCallbackPort}

	b.Run("Found", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_ = HasCapability(caps, CapHostname)
		}
	})

	b.Run("NotFound", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_ = HasCapability(caps, "nonexistent")
		}
	})
}
