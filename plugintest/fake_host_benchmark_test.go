// Copyright 2025-2026 Oakwood Commons
// SPDX-License-Identifier: Apache-2.0

package plugintest

import (
	"context"
	"fmt"
	"testing"
)

func BenchmarkFakeHostClient_GetSecret(b *testing.B) {
	client, _, cleanup, err := StartFakeHostServer(WithInitialSecrets(map[string]string{
		"scafctl.auth.github.token.default": "gho_benchmark_secret",
	}))
	if err != nil {
		b.Fatal(err)
	}
	defer cleanup()

	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _, _ = client.GetSecret(ctx, "scafctl.auth.github.token.default")
	}
}

func BenchmarkFakeHostClient_SetSecret(b *testing.B) {
	client, _, cleanup, err := StartFakeHostServer()
	if err != nil {
		b.Fatal(err)
	}
	defer cleanup()

	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = client.SetSecret(ctx, "scafctl.auth.github.token.default", "gho_benchmark_secret")
	}
}

func BenchmarkFakeHostClient_ListSecretsByPrefix(b *testing.B) {
	seed := make(map[string]string, 20)
	for i := 0; i < 10; i++ {
		seed[fmt.Sprintf("scafctl.auth.github.token.p%d", i)] = "tok"
		seed[fmt.Sprintf("scafctl.auth.entra.token.p%d", i)] = "tok"
	}
	client, _, cleanup, err := StartFakeHostServer(WithInitialSecrets(seed))
	if err != nil {
		b.Fatal(err)
	}
	defer cleanup()

	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _ = client.ListSecretsByPrefix(ctx, "scafctl.auth.github.token.")
	}
}
