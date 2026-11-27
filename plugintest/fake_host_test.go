// Copyright 2025-2026 Oakwood Commons
// SPDX-License-Identifier: Apache-2.0

package plugintest

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/oakwood-commons/scafctl-plugin-sdk/auth"
	"github.com/oakwood-commons/scafctl-plugin-sdk/plugin"
	"github.com/oakwood-commons/scafctl-plugin-sdk/plugin/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestFakeHostService_ImplementsInterface(t *testing.T) {
	t.Parallel()
	var _ proto.HostServiceServer = (*FakeHostService)(nil)
}

func TestFakeHostService_SecretCRUD(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client, fake := NewFakeHostClient(t, WithInitialSecrets(map[string]string{
		"seed-key": "seed-val",
	}))

	val, found, err := client.GetSecret(ctx, "seed-key")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "seed-val", val)

	val, found, err = client.GetSecret(ctx, "missing-key")
	require.NoError(t, err)
	assert.False(t, found)
	assert.Empty(t, val)

	require.NoError(t, client.SetSecret(ctx, "scafctl.auth.github.token.default", "gho_abc"))
	got, ok := fake.Secret("scafctl.auth.github.token.default")
	assert.True(t, ok)
	assert.Equal(t, "gho_abc", got)

	// Overwrite existing secret.
	require.NoError(t, client.SetSecret(ctx, "seed-key", "updated-val"))
	val, found, err = client.GetSecret(ctx, "seed-key")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "updated-val", val)

	// Snapshot returns defensive copy.
	snap := fake.Secrets()
	assert.Equal(t, map[string]string{
		"seed-key":                          "updated-val",
		"scafctl.auth.github.token.default": "gho_abc",
	}, snap)
	snap["seed-key"] = "mutated"
	stillUpdated, _ := fake.Secret("seed-key")
	assert.Equal(t, "updated-val", stillUpdated)

	// Delete existing and non-existent secrets (idempotent).
	require.NoError(t, client.DeleteSecret(ctx, "seed-key"))
	require.NoError(t, client.DeleteSecret(ctx, "seed-key"))
	_, found, err = client.GetSecret(ctx, "seed-key")
	require.NoError(t, err)
	assert.False(t, found)

	// SecretRef integration over FakeHostService.
	require.NoError(t, fake.PutSecret("deploy-token", "tok-xyz"))
	resolved, err := plugin.SecretRef("secret://deploy-token").ResolveWithHost(ctx, client)
	require.NoError(t, err)
	assert.Equal(t, "tok-xyz", resolved)

	fake.RemoveSecret("deploy-token")
	_, ok = fake.Secret("deploy-token")
	assert.False(t, ok)
}

func TestFakeHostService_SecretNameValidation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client, fake := NewFakeHostClient(t)

	invalidCases := []struct {
		name    string
		key     string
		wantErr string
	}{
		{
			name:    "empty",
			key:     "",
			wantErr: "secret name must not be empty",
		},
		{
			name:    "exceeds max length",
			key:     strings.Repeat("a", plugin.MaxHostSecretNameLength+1),
			wantErr: "secret name exceeds maximum length of 256",
		},
		{
			name:    "colon in token cache key",
			key:     "scafctl.auth.github.token:github.com",
			wantErr: "secret name contains invalid characters",
		},
		{
			name:    "glob wildcard",
			key:     "scafctl.auth.github.*",
			wantErr: "secret name contains invalid characters",
		},
		{
			name:    "whitespace",
			key:     "my secret",
			wantErr: "secret name contains invalid characters",
		},
		{
			name:    "path traversal",
			key:     "scafctl/../../etc/passwd",
			wantErr: "secret name must not contain path traversal sequences",
		},
		{
			name:    "consecutive dots",
			key:     "scafctl..token",
			wantErr: "secret name must not contain path traversal sequences",
		},
		{
			name:    "slash rejected by backing store",
			key:     "scafctl.auth.entra/profile-a/refresh_token",
			wantErr: "name contains invalid characters",
		},
		{
			name:    "256-byte name exceeds backing store 255-byte limit",
			key:     strings.Repeat("k", plugin.MaxHostSecretNameLength),
			wantErr: "name exceeds maximum length of 255 characters",
		},
		{
			name:    "leading dot rejected by backing store",
			key:     ".leading-dot",
			wantErr: "name cannot start with '.'",
		},
		{
			name:    "leading dash rejected by backing store",
			key:     "-leading-dash",
			wantErr: "name cannot start with '-'",
		},
		{
			name:    "leading underscore rejected by backing store",
			key:     "_leading_underscore",
			wantErr: "name contains invalid characters",
		},
	}

	for _, tc := range invalidCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, _, err := client.GetSecret(ctx, tc.key)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)

			err = client.SetSecret(ctx, tc.key, "v")
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)

			err = client.DeleteSecret(ctx, tc.key)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)

			err = fake.PutSecret(tc.key, "v")
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}

	// Valid host names satisfying both RPC and backing-store rules (1..255 bytes,
	// leading ASCII alphanumeric, and only ASCII alphanumeric, '.', '_', or '-').
	validNames := []string{
		"a",
		"scafctl.auth.github.token.default",
		"scafctl.auth.entra.profile-a_refresh_token",
		strings.Repeat("k", plugin.MaxHostSecretNameLength-1),
	}
	for _, validKey := range validNames {
		require.NoError(t, client.SetSecret(ctx, validKey, "ok"))
		got, found, err := client.GetSecret(ctx, validKey)
		require.NoError(t, err)
		assert.True(t, found)
		assert.Equal(t, "ok", got)
	}

	// Invalid initial secrets (both RPC-level and backing-store violations) fail
	// NewFakeHostService and StartFakeHostServer.
	for _, badSeedKey := range []string{
		"invalid:key",
		"scafctl.auth.entra/profile-a/refresh_token",
		strings.Repeat("k", plugin.MaxHostSecretNameLength),
	} {
		_, err := NewFakeHostService(WithInitialSecrets(map[string]string{
			badSeedKey: "val",
		}))
		require.Error(t, err, "expected seed rejection for %q", badSeedKey)

		badClient, badFake, badCleanup, err := StartFakeHostServer(WithInitialSecrets(map[string]string{
			badSeedKey: "val",
		}))
		require.Error(t, err, "expected server start rejection for %q", badSeedKey)
		assert.Nil(t, badClient)
		assert.Nil(t, badFake)
		assert.Nil(t, badCleanup)
	}

	// Direct backing-store helper remains self-contained across all validation branches.
	require.ErrorContains(t, validateStoreSecretName(""), "name cannot be empty")
	require.ErrorContains(t, validateStoreSecretName("a..b"), "name cannot contain '..'")
}

func TestFakeHostService_ListSecretsAndPrefix(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client, _ := NewFakeHostClient(t, WithInitialSecrets(map[string]string{
		"scafctl.auth.github.token.work":    "t1",
		"scafctl.auth.github.token.default": "t2",
		"scafctl_auth_github_token_other":   "t3",
		"other.scafctl.auth.github.token.x": "t4",
		"app-secret":                        "t5",
	}))

	// Empty pattern returns all names in deterministic sorted order.
	all, err := client.ListSecrets(ctx, "")
	require.NoError(t, err)
	assert.Equal(t, []string{
		"app-secret",
		"other.scafctl.auth.github.token.x",
		"scafctl.auth.github.token.default",
		"scafctl.auth.github.token.work",
		"scafctl_auth_github_token_other",
	}, all)

	// Unescaped glob-like regex matches unintended keys because '.' matches '_'
	// and the pattern is unanchored on the host.
	buggyGlobMatches, err := client.ListSecrets(ctx, "scafctl.auth.github.token.*")
	require.NoError(t, err)
	assert.Contains(t, buggyGlobMatches, "scafctl_auth_github_token_other")
	assert.Contains(t, buggyGlobMatches, "other.scafctl.auth.github.token.x")

	// ListSecretsByPrefix escapes '.' and anchors with '^', returning only exact prefix matches.
	exactPrefixMatches, err := client.ListSecretsByPrefix(ctx, "scafctl.auth.github.token.")
	require.NoError(t, err)
	assert.Equal(t, []string{
		"scafctl.auth.github.token.default",
		"scafctl.auth.github.token.work",
	}, exactPrefixMatches)

	// Invalid regular expression fails even on an empty store.
	emptyClient, _ := NewFakeHostClient(t)
	_, err = emptyClient.ListSecrets(ctx, "*")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid pattern")

	// Pattern exceeding 256 bytes is rejected.
	_, err = emptyClient.ListSecrets(ctx, strings.Repeat("a", plugin.MaxHostSecretPatternLength+1))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pattern too long: max 256 characters")
}

func TestFakeHostService_ScopingAndReadOnly(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("read-only blocks writes and deletes while allowing reads", func(t *testing.T) {
		t.Parallel()
		client, _ := NewFakeHostClient(t,
			WithInitialSecrets(map[string]string{"deploy-token": "v1"}),
			WithReadOnlySecrets(true),
		)

		val, found, err := client.GetSecret(ctx, "deploy-token")
		require.NoError(t, err)
		assert.True(t, found)
		assert.Equal(t, "v1", val)

		names, err := client.ListSecrets(ctx, "")
		require.NoError(t, err)
		assert.Equal(t, []string{"deploy-token"}, names)

		err = client.SetSecret(ctx, "deploy-token", "v2")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "read-only for provider plugins")

		err = client.DeleteSecret(ctx, "deploy-token")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "read-only for provider plugins")
	})

	t.Run("allowed prefix restricts access and filters list", func(t *testing.T) {
		t.Parallel()
		client, _ := NewFakeHostClient(t,
			WithInitialSecrets(map[string]string{
				"scafctl.auth.github.tok": "g1",
				"scafctl.auth.entra.tok":  "e1",
			}),
			WithAllowedSecretPrefix("scafctl.auth.github."),
		)

		val, found, err := client.GetSecret(ctx, "scafctl.auth.github.tok")
		require.NoError(t, err)
		assert.True(t, found)
		assert.Equal(t, "g1", val)

		_, _, err = client.GetSecret(ctx, "scafctl.auth.entra.tok")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "outside allowed scope")

		err = client.SetSecret(ctx, "scafctl.auth.entra.new", "e2")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "outside allowed scope")

		err = client.DeleteSecret(ctx, "scafctl.auth.entra.tok")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "outside allowed scope")

		names, err := client.ListSecrets(ctx, "")
		require.NoError(t, err)
		assert.Equal(t, []string{"scafctl.auth.github.tok"}, names)
	})

	t.Run("denied prefixes block internal namespaces and filter list", func(t *testing.T) {
		t.Parallel()
		client, _ := NewFakeHostClient(t,
			WithInitialSecrets(map[string]string{
				"scafctl.auth.github.tok": "internal",
				"user-api-token":          "public",
			}),
			WithDeniedSecretPrefixes([]string{"scafctl."}),
		)

		val, found, err := client.GetSecret(ctx, "user-api-token")
		require.NoError(t, err)
		assert.True(t, found)
		assert.Equal(t, "public", val)

		_, _, err = client.GetSecret(ctx, "scafctl.auth.github.tok")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "reserved internal namespace")

		err = client.SetSecret(ctx, "scafctl.auth.github.tok", "x")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "reserved internal namespace")

		err = client.DeleteSecret(ctx, "scafctl.auth.github.tok")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "reserved internal namespace")

		names, err := client.ListSecrets(ctx, "")
		require.NoError(t, err)
		assert.Equal(t, []string{"user-api-token"}, names)
	})
}

func TestFakeHostService_SecretErrorHooks(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	client, fake := NewFakeHostClient(t,
		WithInitialSecrets(map[string]string{"k1": "v1"}),
		WithGetSecretErrorFunc(func(_ context.Context, name string) error {
			if name == "boom" {
				return errors.New("disk read error")
			}
			return nil
		}),
		WithSetSecretErrorFunc(func(_ context.Context, name, _ string) error {
			if name == "full" {
				return errors.New("quota exceeded")
			}
			return nil
		}),
		WithDeleteSecretErrorFunc(func(_ context.Context, name string) error {
			if name == "locked" {
				return status.Error(codes.Unavailable, "store locked")
			}
			return nil
		}),
		WithListSecretsErrorFunc(func(_ context.Context, pattern string) error {
			if pattern == "^fail" {
				return errors.New("corrupt index")
			}
			return nil
		}),
	)

	_, _, err := client.GetSecret(ctx, "boom")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "disk read error")

	// Backing-store name validation runs before error hooks.
	hookInvoked := false
	fake.SetSetSecretErrorFunc(func(_ context.Context, _, _ string) error {
		hookInvoked = true
		return errors.New("quota exceeded")
	})
	err = client.SetSecret(ctx, "invalid/slash", "val")
	require.Error(t, err)
	assert.False(t, hookInvoked)

	fake.SetSetSecretErrorFunc(func(_ context.Context, name, _ string) error {
		if name == "full" {
			return errors.New("quota exceeded")
		}
		return nil
	})
	err = client.SetSecret(ctx, "full", "val")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "quota exceeded")

	err = client.DeleteSecret(ctx, "locked")
	require.Error(t, err)
	assert.Equal(t, codes.Unavailable, status.Code(errors.Unwrap(err)))

	_, err = client.ListSecrets(ctx, "^fail")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "corrupt index")

	// Runtime setter callbacks do not hold f.mu when invoked (can inspect fake safely).
	fake.SetGetSecretErrorFunc(func(_ context.Context, name string) error {
		_, _ = fake.Secret(name)
		return nil
	})
	val, found, err := client.GetSecret(ctx, "k1")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "v1", val)

	fake.SetSetSecretErrorFunc(nil)
	require.NoError(t, client.SetSecret(ctx, "full", "val"))
	fake.SetDeleteSecretErrorFunc(nil)
	require.NoError(t, client.DeleteSecret(ctx, "locked"))
	fake.SetListSecretsErrorFunc(nil)
	names, err := client.ListSecrets(ctx, "^full$")
	require.NoError(t, err)
	assert.Equal(t, []string{"full"}, names)
}

func TestFakeHostService_AuthAndPromptHooks(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("defaults when hooks are unset", func(t *testing.T) {
		t.Parallel()
		client, _ := NewFakeHostClient(t)

		_, err := client.GetAuthIdentity(ctx, "github", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "auth identity not available")

		handlers, def, err := client.ListAuthHandlers(ctx)
		require.NoError(t, err)
		assert.Empty(t, handlers)
		assert.Empty(t, def)

		_, err = client.GetAuthToken(ctx, "github", "repo", 60, false)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "auth token service not available")

		_, err = client.GetAuthGroups(ctx, "entra")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "auth groups not available")

		_, err = client.PromptAuthResponse(ctx, "entra", "https://login.example.com/authorize", "http://localhost:8400/callback")
		require.Error(t, err)
		assert.True(t, plugin.IsUnimplemented(err))
	})

	t.Run("configured callbacks and runtime setters", func(t *testing.T) {
		t.Parallel()
		client, fake := NewFakeHostClient(t,
			WithAuthIdentityFunc(func(_ context.Context, req *proto.GetAuthIdentityRequest) (*proto.Claims, error) {
				if req.GetHandlerName() == "err" {
					return nil, errors.New("not logged in")
				}
				return &proto.Claims{
					Email:   "dev@example.com",
					Subject: req.GetHandlerName() + ":" + req.GetScope(),
				}, nil
			}),
			WithAuthHandlersFunc(func(_ context.Context) ([]string, string, error) {
				return []string{"github", "entra"}, "github", nil
			}),
			WithAuthTokenFunc(func(_ context.Context, req *proto.GetAuthTokenRequest) (*proto.GetAuthTokenResponse, error) {
				if req.GetHandlerName() == "err" {
					return nil, errors.New("refresh failed")
				}
				return &proto.GetAuthTokenResponse{
					AccessToken: "tok-" + req.GetProfile() + "-" + req.GetHostname(),
					TokenType:   "Bearer",
					Scope:       req.GetScope(),
				}, nil
			}),
			WithAuthGroupsFunc(func(_ context.Context, req *proto.GetAuthGroupsRequest) ([]string, error) {
				if req.GetHandlerName() == "legacy" {
					return nil, status.Error(codes.Unimplemented, "older host")
				}
				return []string{"team-" + req.GetProfile()}, nil
			}),
			WithPromptAuthResponseFunc(func(_ context.Context, req *proto.PromptAuthResponseRequest) (string, error) {
				if req.GetHandlerName() == "cancel" {
					return "", status.Error(codes.Canceled, "user canceled")
				}
				return req.GetRedirectUri() + "?code=auth123", nil
			}),
		)

		claims, err := client.GetAuthIdentity(ctx, "github", "read:org")
		require.NoError(t, err)
		assert.Equal(t, "dev@example.com", claims.GetEmail())
		assert.Equal(t, "github:read:org", claims.GetSubject())

		_, err = client.GetAuthIdentity(ctx, "err", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not logged in")

		handlers, def, err := client.ListAuthHandlers(ctx)
		require.NoError(t, err)
		assert.Equal(t, []string{"github", "entra"}, handlers)
		assert.Equal(t, "github", def)

		profileCtx := auth.WithProfile(ctx, "prod")
		tok, err := client.GetAuthTokenWithOptions(profileCtx, plugin.AuthTokenRequest{
			Handler:  "github",
			Scope:    "repo",
			Hostname: "git.example.com",
		})
		require.NoError(t, err)
		assert.Equal(t, "tok-prod-git.example.com", tok.GetAccessToken())
		assert.Equal(t, "repo", tok.GetScope())

		_, err = client.GetAuthToken(ctx, "err", "", 0, false)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "refresh failed")

		groups, err := client.GetAuthGroups(profileCtx, "entra")
		require.NoError(t, err)
		assert.Equal(t, []string{"team-prod"}, groups)

		// Unimplemented from GetAuthGroups hook triggers HostServiceClient's empty-slice fallback.
		legacyGroups, err := client.GetAuthGroups(ctx, "legacy")
		require.NoError(t, err)
		assert.Empty(t, legacyGroups)

		redirectURL, err := client.PromptAuthResponse(ctx, "entra", "https://login.example.com/auth", "http://localhost:8400/cb")
		require.NoError(t, err)
		assert.Equal(t, "http://localhost:8400/cb?code=auth123", redirectURL)

		_, err = client.PromptAuthResponse(ctx, "cancel", "https://login.example.com/auth", "http://localhost:8400/cb")
		require.Error(t, err)
		assert.False(t, plugin.IsUnimplemented(err))

		// Verify runtime Set*Func setters.
		fake.SetAuthHandlersFunc(func(_ context.Context) ([]string, string, error) {
			return nil, "", errors.New("registry down")
		})
		_, _, err = client.ListAuthHandlers(ctx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "registry down")

		fake.SetAuthIdentityFunc(nil)
		_, err = client.GetAuthIdentity(ctx, "github", "")
		require.Error(t, err)

		fake.SetAuthTokenFunc(func(_ context.Context, _ *proto.GetAuthTokenRequest) (*proto.GetAuthTokenResponse, error) {
			return nil, nil
		})
		emptyTok, err := client.GetAuthToken(ctx, "github", "", 0, false)
		require.NoError(t, err)
		assert.Empty(t, emptyTok.GetAccessToken())

		fake.SetAuthGroupsFunc(func(_ context.Context, _ *proto.GetAuthGroupsRequest) ([]string, error) {
			return nil, errors.New("graph failure")
		})
		_, err = client.GetAuthGroups(ctx, "entra")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "graph failure")

		fake.SetPromptAuthResponseFunc(func(_ context.Context, _ *proto.PromptAuthResponseRequest) (string, error) {
			return "", errors.New("tty read failure")
		})
		_, err = client.PromptAuthResponse(ctx, "entra", "https://login.example.com/auth", "http://localhost:8400/cb")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "tty read failure")
	})
}

func TestFakeHostService_ConcurrentAccessAndIdempotentCleanup(t *testing.T) {
	t.Parallel()
	client, fake, cleanup, err := StartFakeHostServer(
		WithInitialSecrets(map[string]string{
			"scafctl.auth.test.seed": "init",
		}),
	)
	require.NoError(t, err)
	defer cleanup()

	ctx := context.Background()
	const workers = 12
	const iterations = 25

	var wg sync.WaitGroup
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		workerID := w
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				key := fmt.Sprintf("scafctl.auth.test.w%d.k%d", workerID, i)
				assert.NoError(t, client.SetSecret(ctx, key, "val"))
				_, _, err := client.GetSecret(ctx, key)
				assert.NoError(t, err)
				_, err = client.ListSecretsByPrefix(ctx, "scafctl.auth.test.")
				assert.NoError(t, err)
				if i%2 == 0 {
					assert.NoError(t, client.DeleteSecret(ctx, key))
				}
				if i == 0 {
					fake.SetAuthTokenFunc(func(_ context.Context, _ *proto.GetAuthTokenRequest) (*proto.GetAuthTokenResponse, error) {
						return &proto.GetAuthTokenResponse{AccessToken: "tok", TokenType: "Bearer"}, nil
					})
				}
				_, _ = client.GetAuthToken(ctx, "test", "", 0, false)
			}
		}()
	}
	wg.Wait()

	// Cleanup is safe to invoke multiple times.
	cleanup()
	cleanup()
}

func TestFakeHostService_ZeroValueAndContextErrors(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// Zero-value FakeHostService supports PutSecret, SetSecret, GetSecret, ListSecrets, and DeleteSecret without panicking.
	var zero FakeHostService
	require.NoError(t, zero.PutSecret("seed-a", "v-a"))
	setResp, err := zero.SetSecret(ctx, &proto.SetSecretRequest{Name: "seed-b", Value: "v-b"})
	require.NoError(t, err)
	assert.Empty(t, setResp.GetError())

	getResp, err := zero.GetSecret(ctx, &proto.GetSecretRequest{Name: "seed-b"})
	require.NoError(t, err)
	assert.True(t, getResp.GetFound())
	assert.Equal(t, "v-b", getResp.GetValue())

	listResp, err := zero.ListSecrets(ctx, &proto.ListSecretsRequest{})
	require.NoError(t, err)
	assert.Equal(t, []string{"seed-a", "seed-b"}, listResp.GetNames())

	// Pattern validation runs before listSecretsErrFunc.
	hookCalled := false
	client, _ := NewFakeHostClient(t,
		WithListSecretsErrorFunc(func(_ context.Context, _ string) error {
			hookCalled = true
			return errors.New("hook should not run on invalid pattern")
		}),
		WithAuthHandlersFunc(func(_ context.Context) ([]string, string, error) {
			return nil, "", fmt.Errorf("lookup aborted: %w", context.DeadlineExceeded)
		}),
		WithPromptAuthResponseFunc(func(_ context.Context, _ *proto.PromptAuthResponseRequest) (string, error) {
			return "", fmt.Errorf("prompt aborted: %w", context.Canceled)
		}),
	)

	_, err = client.ListSecrets(ctx, "[invalid")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid pattern")
	assert.False(t, hookCalled)

	_, _, err = client.ListAuthHandlers(ctx)
	require.Error(t, err)
	assert.Equal(t, codes.DeadlineExceeded, status.Code(errors.Unwrap(err)))

	_, err = client.PromptAuthResponse(ctx, "entra", "https://login.example.com/auth", "http://localhost:8400/cb")
	require.Error(t, err)
	assert.Equal(t, codes.Canceled, status.Code(errors.Unwrap(err)))
}
