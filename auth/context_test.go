// Copyright 2025-2026 Oakwood Commons
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWithProfile(t *testing.T) {
	tests := []struct {
		name    string
		profile string
		want    string
	}{
		{name: "empty profile (default)", profile: "", want: ""},
		{name: "named profile", profile: "work", want: "work"},
		{name: "profile with hyphens", profile: "my-company", want: "my-company"},
		{name: "profile with underscores", profile: "corp_prod", want: "corp_prod"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := WithProfile(context.Background(), tt.profile)
			got := ProfileFromContext(ctx)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestProfileFromContext_notSet(t *testing.T) {
	got := ProfileFromContext(context.Background())
	assert.Empty(t, got)
}

func TestWithProfile_overwrite(t *testing.T) {
	ctx := WithProfile(context.Background(), "first")
	ctx = WithProfile(ctx, "second")
	assert.Equal(t, "second", ProfileFromContext(ctx))
}
