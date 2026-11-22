// Copyright 2025-2026 Oakwood Commons
// SPDX-License-Identifier: Apache-2.0

package plugin

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSecretRef_Validate_ValidEnv(t *testing.T) {
	t.Parallel()
	s := SecretRef("env://MY_SECRET")
	assert.NoError(t, s.Validate())
}

func TestSecretRef_Validate_ValidFile(t *testing.T) {
	t.Parallel()
	s := SecretRef("file:///run/secrets/token")
	assert.NoError(t, s.Validate())
}

func TestSecretRef_Validate_EmptyEnvName(t *testing.T) {
	t.Parallel()
	s := SecretRef("env://")
	err := s.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "requires a variable name")
}

func TestSecretRef_Validate_EmptyFilePath(t *testing.T) {
	t.Parallel()
	s := SecretRef("file://")
	err := s.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "requires a path")
}

func TestSecretRef_Validate_UnknownScheme(t *testing.T) {
	t.Parallel()
	s := SecretRef("vault://secret/data/foo")
	err := s.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported secret ref scheme")
}

func TestSecretRef_Validate_NoScheme(t *testing.T) {
	t.Parallel()
	s := SecretRef("just-a-string")
	err := s.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported secret ref scheme")
}

func TestSecretRef_Resolve_Env(t *testing.T) {
	t.Setenv("TEST_SECRET_REF_VAL", "s3cr3t")
	s := SecretRef("env://TEST_SECRET_REF_VAL")
	val, err := s.Resolve()
	require.NoError(t, err)
	assert.Equal(t, "s3cr3t", val)
}

func TestSecretRef_Resolve_EnvEmpty(t *testing.T) {
	t.Setenv("TEST_SECRET_REF_EMPTY", "")
	s := SecretRef("env://TEST_SECRET_REF_EMPTY")
	_, err := s.Resolve()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty or not set")
}

func TestSecretRef_Resolve_FileExists(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "secret")
	require.NoError(t, os.WriteFile(path, []byte("file-secret\n"), 0o600))
	s := SecretRef("file://" + path)
	val, err := s.Resolve()
	require.NoError(t, err)
	assert.Equal(t, "file-secret", val)
}

func TestSecretRef_Resolve_FileMissing(t *testing.T) {
	t.Parallel()
	s := SecretRef("file:///nonexistent/path/secret")
	_, err := s.Resolve()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reading secret file")
}

func TestSecretRef_Scheme(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "env", SecretRef("env://FOO").Scheme())
	assert.Equal(t, "file", SecretRef("file:///bar").Scheme())
	assert.Equal(t, "secret", SecretRef("secret://my-token").Scheme())
	assert.Equal(t, "", SecretRef("noscheme").Scheme())
}

func TestSecretRef_Validate_SecretScheme(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		ref     SecretRef
		wantErr string
	}{
		{name: "valid simple name", ref: SecretRef("secret://github-pat")},
		{name: "valid dot underscore and dash", ref: SecretRef("secret://org.api_key_1-prod")},
		{name: "valid single char", ref: SecretRef("secret://a")},
		{name: "valid max length 255", ref: SecretRef("secret://" + strings.Repeat("a", 255))},
		{name: "valid non-reserved scafctl_ prefix", ref: SecretRef("secret://scafctl_custom")},
		{name: "empty name", ref: SecretRef("secret://"), wantErr: "requires a secret name"},
		{name: "whitespace-only name", ref: SecretRef("secret://   "), wantErr: "requires a secret name"},
		{name: "exceeds max length 256", ref: SecretRef("secret://" + strings.Repeat("a", 256)), wantErr: "exceeds maximum length of 255 bytes"},
		{name: "leading dot", ref: SecretRef("secret://.deploy-token"), wantErr: "must not start with '.' or '-'"},
		{name: "single dot", ref: SecretRef("secret://."), wantErr: "must not start with '.' or '-'"},
		{name: "leading dash", ref: SecretRef("secret://-deploy-token"), wantErr: "must not start with '.' or '-'"},
		{name: "leading underscore", ref: SecretRef("secret://_deploy_token"), wantErr: "contains invalid characters"},
		{name: "contains traversal", ref: SecretRef("secret://../etc/passwd"), wantErr: "must not start with '.' or '-'"},
		{name: "contains embedded double-dot", ref: SecretRef("secret://foo..bar"), wantErr: "must not contain '..'"},
		{name: "slash separator rejected", ref: SecretRef("secret://org/api_key_1"), wantErr: "contains invalid characters"},
		{name: "leading slash rejected", ref: SecretRef("secret:///scafctl.auth.github"), wantErr: "contains invalid characters"},
		{name: "backslash separator rejected", ref: SecretRef(`secret://org\my-token`), wantErr: "contains invalid characters"},
		{name: "surrounding whitespace rejected", ref: SecretRef("secret:// my-token "), wantErr: "contains invalid characters"},
		{name: "reserved scafctl. prefix", ref: SecretRef("secret://scafctl.auth.github"), wantErr: "must not start with reserved prefix"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.ref.Validate()
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestSecretRef_Resolve_SecretRequiresHost(t *testing.T) {
	t.Parallel()
	_, err := SecretRef("secret://my-token").Resolve()
	require.EqualError(t, err, "secret:// scheme requires ResolveWithHost(ctx, hostClient)")

	// Invalid secret:// refs still fail validation first.
	_, err = SecretRef("secret://scafctl.internal").Resolve()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reserved prefix")
}

func TestSecretRef_Resolve_EmptyFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "empty")
	require.NoError(t, os.WriteFile(path, []byte("   \n"), 0o600))
	_, err := SecretRef("file://" + path).Resolve()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is empty")
}

type secretGetterFunc func(ctx context.Context, name string) (string, bool, error)

func (f secretGetterFunc) GetSecret(ctx context.Context, name string) (string, bool, error) {
	return f(ctx, name)
}

type valueSecretGetter struct {
	val string
}

func (g valueSecretGetter) GetSecret(_ context.Context, _ string) (string, bool, error) {
	return g.val, true, nil
}

func TestSecretRef_ResolveWithHost(t *testing.T) {
	conn, cleanup := startFakeHostService(t)
	defer cleanup()
	hc := NewHostServiceClient(conn)

	t.Run("secret found via HostServiceClient", func(t *testing.T) {
		val, err := SecretRef("secret://api-token").ResolveWithHost(context.Background(), hc)
		require.NoError(t, err)
		assert.Equal(t, "secret-api-token", val)
	})

	t.Run("secret not found", func(t *testing.T) {
		_, err := SecretRef("secret://missing").ResolveWithHost(context.Background(), hc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `secret "missing" not found`)
	})

	t.Run("secret getter error", func(t *testing.T) {
		_, err := SecretRef("secret://error").ResolveWithHost(context.Background(), hc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "some error")
	})

	t.Run("secret empty value", func(t *testing.T) {
		_, err := SecretRef("secret://blank").ResolveWithHost(context.Background(), valueSecretGetter{val: ""})
		require.Error(t, err)
		assert.Contains(t, err.Error(), `secret "blank" is empty`)
	})

	t.Run("value-type SecretGetter succeeds", func(t *testing.T) {
		val, err := SecretRef("secret://tok").ResolveWithHost(context.Background(), valueSecretGetter{val: "ok-val"})
		require.NoError(t, err)
		assert.Equal(t, "ok-val", val)
	})

	t.Run("nil getter interface", func(t *testing.T) {
		_, err := SecretRef("secret://api-token").ResolveWithHost(context.Background(), nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "host secret getter is nil")
	})

	t.Run("typed-nil HostServiceClient from context", func(t *testing.T) {
		// HostClientFromContext returns (*HostServiceClient)(nil) when absent.
		_, err := SecretRef("secret://api-token").ResolveWithHost(context.Background(), HostClientFromContext(context.Background()))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "host secret getter is nil")
	})

	t.Run("typed-nil func SecretGetter", func(t *testing.T) {
		var fn secretGetterFunc
		_, err := SecretRef("secret://api-token").ResolveWithHost(context.Background(), fn)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "host secret getter is nil")
	})

	t.Run("env scheme works with nil getter", func(t *testing.T) {
		t.Setenv("TEST_SECRET_WITH_HOST", "env-val")
		val, err := SecretRef("env://TEST_SECRET_WITH_HOST").ResolveWithHost(context.Background(), nil)
		require.NoError(t, err)
		assert.Equal(t, "env-val", val)
	})

	t.Run("file scheme works with nil getter", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "tok")
		require.NoError(t, os.WriteFile(path, []byte("file-val\n"), 0o600))
		val, err := SecretRef("file://"+path).ResolveWithHost(context.Background(), nil)
		require.NoError(t, err)
		assert.Equal(t, "file-val", val)
	})

	t.Run("invalid secret ref fails validation", func(t *testing.T) {
		_, err := SecretRef("secret://scafctl.secret").ResolveWithHost(context.Background(), hc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "reserved prefix")
	})
}
