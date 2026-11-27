// Copyright 2025-2026 Oakwood Commons
// SPDX-License-Identifier: Apache-2.0

package plugin

import (
	"context"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"
	"testing"

	"github.com/oakwood-commons/scafctl-plugin-sdk/auth"
	"github.com/oakwood-commons/scafctl-plugin-sdk/plugin/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

// fakeHostService is a minimal in-process HostService for testing.
type fakeHostService struct {
	proto.UnimplementedHostServiceServer
	lastAuthTokenProfile string                      // captured from GetAuthToken requests
	lastAuthTokenReq     *proto.GetAuthTokenRequest  // captured from GetAuthToken requests
	lastAuthGroupsReq    *proto.GetAuthGroupsRequest // captured from GetAuthGroups requests
	nilGroups            bool                        // when true, GetAuthGroups returns nil Groups slice
	promptAuthErr        error                       // when set, PromptAuthResponse returns this gRPC error
	lastPromptReq        *proto.PromptAuthResponseRequest
	lastListPattern      string
	listCalls            int
	listNames            []string
}

func (f *fakeHostService) GetSecret(_ context.Context, req *proto.GetSecretRequest) (*proto.GetSecretResponse, error) {
	if req.Name == "missing" {
		return &proto.GetSecretResponse{Found: false}, nil
	}
	if req.Name == "error" {
		return &proto.GetSecretResponse{Error: "some error"}, nil
	}
	return &proto.GetSecretResponse{Value: "secret-" + req.Name, Found: true}, nil
}

func (f *fakeHostService) SetSecret(_ context.Context, req *proto.SetSecretRequest) (*proto.SetSecretResponse, error) {
	if req.Name == "error" {
		return &proto.SetSecretResponse{Error: "set error"}, nil
	}
	return &proto.SetSecretResponse{}, nil
}

func (f *fakeHostService) DeleteSecret(_ context.Context, req *proto.DeleteSecretRequest) (*proto.DeleteSecretResponse, error) {
	if req.Name == "error" {
		return &proto.DeleteSecretResponse{Error: "delete error"}, nil
	}
	return &proto.DeleteSecretResponse{}, nil
}

func (f *fakeHostService) ListSecrets(_ context.Context, req *proto.ListSecretsRequest) (*proto.ListSecretsResponse, error) {
	f.listCalls++
	f.lastListPattern = req.Pattern
	if req.Pattern == "error" || req.Pattern == "^error" {
		return &proto.ListSecretsResponse{Error: "list error"}, nil
	}
	if f.listNames != nil {
		return &proto.ListSecretsResponse{Names: f.listNames}, nil
	}
	return &proto.ListSecretsResponse{Names: []string{"key1", "key2"}}, nil
}

func (f *fakeHostService) GetAuthIdentity(_ context.Context, req *proto.GetAuthIdentityRequest) (*proto.GetAuthIdentityResponse, error) {
	if req.HandlerName == "error" {
		return &proto.GetAuthIdentityResponse{Error: "identity error"}, nil
	}
	return &proto.GetAuthIdentityResponse{Claims: &proto.Claims{Email: "user@test.com"}}, nil
}

func (f *fakeHostService) ListAuthHandlers(_ context.Context, _ *proto.ListAuthHandlersRequest) (*proto.ListAuthHandlersResponse, error) {
	return &proto.ListAuthHandlersResponse{HandlerNames: []string{"gh", "entra"}, DefaultHandler: "gh"}, nil
}

func (f *fakeHostService) GetAuthToken(_ context.Context, req *proto.GetAuthTokenRequest) (*proto.GetAuthTokenResponse, error) {
	f.lastAuthTokenProfile = req.Profile
	reqCopy := proto.GetAuthTokenRequest{
		HandlerName:        req.HandlerName,
		Scope:              req.Scope,
		MinValidForSeconds: req.MinValidForSeconds,
		ForceRefresh:       req.ForceRefresh,
		Profile:            req.Profile,
		Hostname:           req.Hostname,
	}
	f.lastAuthTokenReq = &reqCopy
	if req.HandlerName == "error" {
		return &proto.GetAuthTokenResponse{Error: "token error"}, nil
	}
	return &proto.GetAuthTokenResponse{AccessToken: "tok123", TokenType: "Bearer"}, nil
}

func (f *fakeHostService) GetAuthGroups(_ context.Context, req *proto.GetAuthGroupsRequest) (*proto.GetAuthGroupsResponse, error) {
	reqCopy := proto.GetAuthGroupsRequest{
		HandlerName: req.HandlerName,
		Profile:     req.Profile,
	}
	f.lastAuthGroupsReq = &reqCopy
	if req.HandlerName == "error" {
		return &proto.GetAuthGroupsResponse{Error: "groups error"}, nil
	}
	if f.nilGroups {
		return &proto.GetAuthGroupsResponse{}, nil
	}
	return &proto.GetAuthGroupsResponse{Groups: []string{"group-a", "group-b"}}, nil
}

func (f *fakeHostService) PromptAuthResponse(_ context.Context, req *proto.PromptAuthResponseRequest) (*proto.PromptAuthResponseResponse, error) {
	reqClone := proto.PromptAuthResponseRequest{
		HandlerName:      req.HandlerName,
		AuthorizationUrl: req.AuthorizationUrl,
		RedirectUri:      req.RedirectUri,
	}
	f.lastPromptReq = &reqClone
	if f.promptAuthErr != nil {
		return nil, f.promptAuthErr
	}
	return &proto.PromptAuthResponseResponse{Value: "http://localhost:8400/callback?code=abc&state=xyz"}, nil
}

func startFakeHostService(t *testing.T) (*grpc.ClientConn, func()) {
	t.Helper()
	_, conn, cleanup := startFakeHostServiceWithFake(t)
	return conn, cleanup
}

func startFakeHostServiceWithFake(t *testing.T) (*fakeHostService, *grpc.ClientConn, func()) {
	t.Helper()
	lis, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)

	fake := &fakeHostService{}
	s := grpc.NewServer()
	proto.RegisterHostServiceServer(s, fake)

	go func() { _ = s.Serve(lis) }()

	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)

	return fake, conn, func() { conn.Close(); s.Stop() }
}

func TestHostServiceClient_GetSecret(t *testing.T) {
	conn, cleanup := startFakeHostService(t)
	defer cleanup()
	c := NewHostServiceClient(conn)

	val, found, err := c.GetSecret(context.Background(), "mykey")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "secret-mykey", val)
}

func TestHostServiceClient_GetSecret_NotFound(t *testing.T) {
	conn, cleanup := startFakeHostService(t)
	defer cleanup()
	c := NewHostServiceClient(conn)

	_, found, err := c.GetSecret(context.Background(), "missing")
	require.NoError(t, err)
	assert.False(t, found)
}

func TestHostServiceClient_GetSecret_Error(t *testing.T) {
	conn, cleanup := startFakeHostService(t)
	defer cleanup()
	c := NewHostServiceClient(conn)

	_, _, err := c.GetSecret(context.Background(), "error")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "some error")
}

func TestHostServiceClient_SetSecret(t *testing.T) {
	conn, cleanup := startFakeHostService(t)
	defer cleanup()
	c := NewHostServiceClient(conn)

	err := c.SetSecret(context.Background(), "mykey", "myval")
	require.NoError(t, err)
}

func TestHostServiceClient_SetSecret_Error(t *testing.T) {
	conn, cleanup := startFakeHostService(t)
	defer cleanup()
	c := NewHostServiceClient(conn)

	err := c.SetSecret(context.Background(), "error", "val")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "set error")
}

func TestHostServiceClient_DeleteSecret(t *testing.T) {
	conn, cleanup := startFakeHostService(t)
	defer cleanup()
	c := NewHostServiceClient(conn)

	err := c.DeleteSecret(context.Background(), "mykey")
	require.NoError(t, err)
}

func TestHostServiceClient_DeleteSecret_Error(t *testing.T) {
	conn, cleanup := startFakeHostService(t)
	defer cleanup()
	c := NewHostServiceClient(conn)

	err := c.DeleteSecret(context.Background(), "error")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "delete error")
}

func TestHostServiceClient_ListSecrets(t *testing.T) {
	conn, cleanup := startFakeHostService(t)
	defer cleanup()
	c := NewHostServiceClient(conn)

	names, err := c.ListSecrets(context.Background(), "*")
	require.NoError(t, err)
	assert.Equal(t, []string{"key1", "key2"}, names)
}

func TestHostServiceClient_ListSecrets_Error(t *testing.T) {
	conn, cleanup := startFakeHostService(t)
	defer cleanup()
	c := NewHostServiceClient(conn)

	_, err := c.ListSecrets(context.Background(), "error")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "list error")
}

func TestHostServiceClient_GetAuthIdentity(t *testing.T) {
	conn, cleanup := startFakeHostService(t)
	defer cleanup()
	c := NewHostServiceClient(conn)

	claims, err := c.GetAuthIdentity(context.Background(), "gh", "read")
	require.NoError(t, err)
	assert.Equal(t, "user@test.com", claims.Email)
}

func TestHostServiceClient_GetAuthIdentity_Error(t *testing.T) {
	conn, cleanup := startFakeHostService(t)
	defer cleanup()
	c := NewHostServiceClient(conn)

	_, err := c.GetAuthIdentity(context.Background(), "error", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "identity error")
}

func TestHostServiceClient_ListAuthHandlers(t *testing.T) {
	conn, cleanup := startFakeHostService(t)
	defer cleanup()
	c := NewHostServiceClient(conn)

	handlers, defaultH, err := c.ListAuthHandlers(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []string{"gh", "entra"}, handlers)
	assert.Equal(t, "gh", defaultH)
}

func TestHostServiceClient_GetAuthToken(t *testing.T) {
	conn, cleanup := startFakeHostService(t)
	defer cleanup()
	c := NewHostServiceClient(conn)

	resp, err := c.GetAuthToken(context.Background(), "gh", "read", 60, false)
	require.NoError(t, err)
	assert.Equal(t, "tok123", resp.AccessToken)
}

func TestHostServiceClient_GetAuthToken_Error(t *testing.T) {
	conn, cleanup := startFakeHostService(t)
	defer cleanup()
	c := NewHostServiceClient(conn)

	_, err := c.GetAuthToken(context.Background(), "error", "", 0, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "token error")
}

func TestHostServiceClient_GetAuthToken_ProfileFromContext(t *testing.T) {
	tests := []struct {
		name    string
		profile string
	}{
		{name: "default profile (empty)", profile: ""},
		{name: "named profile", profile: "work"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake, conn, cleanup := startFakeHostServiceWithFake(t)
			defer cleanup()
			c := NewHostServiceClient(conn)

			ctx := context.Background()
			if tt.profile != "" {
				ctx = auth.WithProfile(ctx, tt.profile)
			}
			resp, err := c.GetAuthToken(ctx, "gh", "read", 60, false)
			require.NoError(t, err)
			assert.Equal(t, "tok123", resp.AccessToken)
			assert.Equal(t, tt.profile, fake.lastAuthTokenProfile)
		})
	}
}

func TestHostServiceClient_GetAuthGroups(t *testing.T) {
	fake, conn, cleanup := startFakeHostServiceWithFake(t)
	defer cleanup()
	c := NewHostServiceClient(conn)

	ctx := auth.WithProfile(context.Background(), "corp")
	groups, err := c.GetAuthGroups(ctx, "entra")
	require.NoError(t, err)
	assert.Equal(t, []string{"group-a", "group-b"}, groups)
	require.NotNil(t, fake.lastAuthGroupsReq)
	assert.Equal(t, "entra", fake.lastAuthGroupsReq.HandlerName)
	assert.Equal(t, "corp", fake.lastAuthGroupsReq.Profile)
}

func TestHostServiceClient_GetAuthGroups_NilGroupsNormalized(t *testing.T) {
	fake, conn, cleanup := startFakeHostServiceWithFake(t)
	defer cleanup()
	fake.nilGroups = true
	c := NewHostServiceClient(conn)

	groups, err := c.GetAuthGroups(context.Background(), "entra")
	require.NoError(t, err)
	assert.NotNil(t, groups)
	assert.Empty(t, groups)
}

func TestHostServiceClient_GetAuthTokenWithOptions(t *testing.T) {
	t.Run("explicit options including hostname and profile override", func(t *testing.T) {
		fake, conn, cleanup := startFakeHostServiceWithFake(t)
		defer cleanup()
		c := NewHostServiceClient(conn)

		ctx := auth.WithProfile(context.Background(), "ctx-profile")
		resp, err := c.GetAuthTokenWithOptions(ctx, AuthTokenRequest{
			Handler:            "github",
			Scope:              "repo",
			MinValidForSeconds: 120,
			ForceRefresh:       true,
			Profile:            "explicit-profile",
			Hostname:           "ghe.corp.example.com",
		})
		require.NoError(t, err)
		assert.Equal(t, "tok123", resp.AccessToken)
		require.NotNil(t, fake.lastAuthTokenReq)
		assert.Equal(t, "github", fake.lastAuthTokenReq.HandlerName)
		assert.Equal(t, "repo", fake.lastAuthTokenReq.Scope)
		assert.Equal(t, int64(120), fake.lastAuthTokenReq.MinValidForSeconds)
		assert.True(t, fake.lastAuthTokenReq.ForceRefresh)
		assert.Equal(t, "explicit-profile", fake.lastAuthTokenReq.Profile)
		assert.Equal(t, "ghe.corp.example.com", fake.lastAuthTokenReq.Hostname)
	})

	t.Run("falls back to profile from context when opts.Profile is empty", func(t *testing.T) {
		fake, conn, cleanup := startFakeHostServiceWithFake(t)
		defer cleanup()
		c := NewHostServiceClient(conn)

		ctx := auth.WithProfile(context.Background(), "from-ctx")
		resp, err := c.GetAuthTokenWithOptions(ctx, AuthTokenRequest{
			Handler:  "openshift",
			Hostname: "api.cluster-a.example.com:6443",
		})
		require.NoError(t, err)
		assert.Equal(t, "tok123", resp.AccessToken)
		require.NotNil(t, fake.lastAuthTokenReq)
		assert.Equal(t, "from-ctx", fake.lastAuthTokenReq.Profile)
		assert.Equal(t, "api.cluster-a.example.com:6443", fake.lastAuthTokenReq.Hostname)
	})

	t.Run("response error", func(t *testing.T) {
		conn, cleanup := startFakeHostService(t)
		defer cleanup()
		c := NewHostServiceClient(conn)

		_, err := c.GetAuthTokenWithOptions(context.Background(), AuthTokenRequest{Handler: "error"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "token error")
	})

	t.Run("grpc transport error", func(t *testing.T) {
		lis, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
		require.NoError(t, err)
		s := grpc.NewServer()
		proto.RegisterHostServiceServer(s, &proto.UnimplementedHostServiceServer{})
		go func() { _ = s.Serve(lis) }()
		defer s.Stop()

		conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
		require.NoError(t, err)
		defer conn.Close()

		c := NewHostServiceClient(conn)
		_, err = c.GetAuthTokenWithOptions(context.Background(), AuthTokenRequest{Handler: "github"})
		require.Error(t, err)
		assert.True(t, IsUnimplemented(err))
	})
}

func TestHostServiceClient_GetAuthGroups_Error(t *testing.T) {
	conn, cleanup := startFakeHostService(t)
	defer cleanup()
	c := NewHostServiceClient(conn)

	_, err := c.GetAuthGroups(context.Background(), "error")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "groups error")
}

func TestHostServiceClient_GetAuthGroups_Unimplemented(t *testing.T) {
	// Start a server that returns gRPC Unimplemented for GetAuthGroups
	// (simulates an older host that hasn't implemented the RPC yet).
	lis, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	s := grpc.NewServer()
	proto.RegisterHostServiceServer(s, &proto.UnimplementedHostServiceServer{})
	go func() { _ = s.Serve(lis) }()
	defer s.Stop()

	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer conn.Close()

	c := NewHostServiceClient(conn)
	groups, err := c.GetAuthGroups(context.Background(), "gh")
	require.NoError(t, err)
	assert.Empty(t, groups)
}

func TestHostServiceClient_PromptAuthResponse(t *testing.T) {
	fake, conn, cleanup := startFakeHostServiceWithFake(t)
	defer cleanup()
	c := NewHostServiceClient(conn)

	val, err := c.PromptAuthResponse(context.Background(), "entra",
		"https://login.example.com/authorize?client_id=abc", "http://localhost:8400/callback")
	require.NoError(t, err)
	assert.Equal(t, "http://localhost:8400/callback?code=abc&state=xyz", val)
	require.NotNil(t, fake.lastPromptReq)
	assert.Equal(t, "entra", fake.lastPromptReq.HandlerName)
	assert.Equal(t, "https://login.example.com/authorize?client_id=abc", fake.lastPromptReq.AuthorizationUrl)
	assert.Equal(t, "http://localhost:8400/callback", fake.lastPromptReq.RedirectUri)
}

func TestHostServiceClient_PromptAuthResponse_Unavailable(t *testing.T) {
	fake, conn, cleanup := startFakeHostServiceWithFake(t)
	defer cleanup()
	fake.promptAuthErr = status.Error(codes.Unavailable, "non-interactive session")
	c := NewHostServiceClient(conn)

	_, err := c.PromptAuthResponse(context.Background(), "entra", "https://auth.example.com", "http://localhost:0/callback")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "non-interactive session")
	assert.False(t, IsUnimplemented(err), "Unavailable must not be mistaken for Unimplemented")
}

func TestHostServiceClient_PromptAuthResponse_Canceled(t *testing.T) {
	fake, conn, cleanup := startFakeHostServiceWithFake(t)
	defer cleanup()
	fake.promptAuthErr = status.Error(codes.Canceled, "user canceled")
	c := NewHostServiceClient(conn)

	_, err := c.PromptAuthResponse(context.Background(), "entra", "https://auth.example.com", "http://localhost:0/callback")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "user canceled")
	assert.False(t, IsUnimplemented(err))
}

func TestHostServiceClient_PromptAuthResponse_Unimplemented(t *testing.T) {
	// Start a server that returns gRPC Unimplemented for PromptAuthResponse
	// (simulates an older host that hasn't implemented the RPC yet).
	lis, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	s := grpc.NewServer()
	proto.RegisterHostServiceServer(s, &proto.UnimplementedHostServiceServer{})
	go func() { _ = s.Serve(lis) }()
	defer s.Stop()

	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer conn.Close()

	c := NewHostServiceClient(conn)
	_, err = c.PromptAuthResponse(context.Background(), "entra", "https://auth.example.com", "http://localhost:0/callback")
	require.Error(t, err)
	assert.True(t, IsUnimplemented(err), "older host should surface as Unimplemented")
}

func TestIsUnimplemented(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "raw status", err: status.Error(codes.Unimplemented, "rpc unimplemented"), want: true},
		{name: "wrapped status", err: fmt.Errorf("host PromptAuthResponse: %w", status.Error(codes.Unimplemented, "boom")), want: true},
		{name: "other code", err: status.Error(codes.Unavailable, "no tty"), want: false},
		{name: "non-gRPC error", err: errors.New("plain error"), want: false},
		{name: "wrapped non-gRPC error", err: fmt.Errorf("outer: %w", errors.New("inner")), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsUnimplemented(tt.err))
		})
	}
}

func TestValidateHostSecretName(t *testing.T) {
	t.Parallel()
	hostRegex := regexp.MustCompile(`\A[a-zA-Z0-9_\-./]+\z`)

	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{name: "simple name", input: "my-secret"},
		{name: "auth handler internal prefix", input: "scafctl.auth.github.token.default"},
		{name: "slash separated namespace", input: "scafctl.auth.entra/work/refresh_token"},
		{name: "single character", input: "a"},
		{name: "leading dot", input: ".token"},
		{name: "leading dash", input: "-token"},
		{name: "leading slash", input: "/token"},
		{name: "max length 256", input: strings.Repeat("a", MaxHostSecretNameLength)},
		{name: "empty", input: "", wantErr: "secret name must not be empty"},
		{name: "exceeds max length 257", input: strings.Repeat("a", MaxHostSecretNameLength+1), wantErr: "secret name exceeds maximum length of 256"},
		{name: "colon rejected", input: "scafctl.auth.github.token:github.com", wantErr: "secret name contains invalid characters"},
		{name: "glob star rejected", input: "scafctl.auth.github.*", wantErr: "secret name contains invalid characters"},
		{name: "whitespace rejected", input: "my secret", wantErr: "secret name contains invalid characters"},
		{name: "backslash rejected", input: `org\token`, wantErr: "secret name contains invalid characters"},
		{name: "embedded newline rejected", input: "foo\nbar", wantErr: "secret name contains invalid characters"},
		{name: "trailing newline rejected", input: "foo\n", wantErr: "secret name contains invalid characters"},
		{name: "unicode digit rejected", input: "token\u0661", wantErr: "secret name contains invalid characters"},
		{name: "invalid utf8 rejected", input: "token\xff", wantErr: "secret name contains invalid characters"},
		{name: "path traversal prefix", input: "../etc/passwd", wantErr: "secret name must not contain path traversal sequences"},
		{name: "embedded double dot", input: "scafctl..token", wantErr: "secret name must not contain path traversal sequences"},
		{name: "bare double dot", input: "..", wantErr: "secret name must not contain path traversal sequences"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateHostSecretName(tt.input)
			refValid := tt.input != "" &&
				len(tt.input) <= MaxHostSecretNameLength &&
				hostRegex.MatchString(tt.input) &&
				!strings.Contains(tt.input, "..")
			assert.Equal(t, refValid, err == nil, "ValidateHostSecretName(%q) parity with host regex", tt.input)

			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestHostServiceClient_ListSecretsByPrefix(t *testing.T) {
	t.Parallel()

	t.Run("escapes regex metacharacters and filters returned names", func(t *testing.T) {
		t.Parallel()
		fake, conn, cleanup := startFakeHostServiceWithFake(t)
		defer cleanup()
		// Include non-matching names to verify client-side strings.HasPrefix defense in depth.
		fake.listNames = []string{
			"scafctl.auth.github.token.default",
			"scafctl.auth.github.token.work",
			"scafctl_auth_github_token_other",
			"other.scafctl.auth.github.token.default",
		}
		c := NewHostServiceClient(conn)

		got, err := c.ListSecretsByPrefix(context.Background(), "scafctl.auth.github.token.")
		require.NoError(t, err)
		assert.Equal(t, `^scafctl\.auth\.github\.token\.`, fake.lastListPattern)
		assert.Equal(t, []string{
			"scafctl.auth.github.token.default",
			"scafctl.auth.github.token.work",
		}, got)
	})

	t.Run("empty prefix sends empty pattern and returns all names", func(t *testing.T) {
		t.Parallel()
		fake, conn, cleanup := startFakeHostServiceWithFake(t)
		defer cleanup()
		c := NewHostServiceClient(conn)

		got, err := c.ListSecretsByPrefix(context.Background(), "")
		require.NoError(t, err)
		assert.Equal(t, "", fake.lastListPattern)
		assert.Equal(t, []string{"key1", "key2"}, got)
	})

	t.Run("returns non-nil empty slice when nothing matches", func(t *testing.T) {
		t.Parallel()
		fake, conn, cleanup := startFakeHostServiceWithFake(t)
		defer cleanup()
		fake.listNames = []string{"unrelated"}
		c := NewHostServiceClient(conn)

		got, err := c.ListSecretsByPrefix(context.Background(), "scafctl.auth.")
		require.NoError(t, err)
		assert.NotNil(t, got)
		assert.Empty(t, got)
	})

	t.Run("allows exact MaxHostSecretPatternLength escaped pattern", func(t *testing.T) {
		t.Parallel()
		fake, conn, cleanup := startFakeHostServiceWithFake(t)
		defer cleanup()
		c := NewHostServiceClient(conn)

		// "^" (1 byte) + 255 plain characters = 256 bytes.
		prefix := strings.Repeat("a", MaxHostSecretPatternLength-1)
		_, err := c.ListSecretsByPrefix(context.Background(), prefix)
		require.NoError(t, err)
		assert.Equal(t, 1, fake.listCalls)
		assert.Len(t, fake.lastListPattern, MaxHostSecretPatternLength)
	})

	t.Run("bounds escaped regex hint when valid prefix expands past MaxHostSecretPatternLength", func(t *testing.T) {
		t.Parallel()
		fake, conn, cleanup := startFakeHostServiceWithFake(t)
		defer cleanup()

		// 240-byte prefix with 24 dots: valid secret prefix (<= 256 bytes), but
		// "^" + QuoteMeta(prefix) is 265 bytes (> 256).
		prefix := strings.Repeat("abcdefgh.", 24) + strings.Repeat("x", 24)
		require.LessOrEqual(t, len(prefix), MaxHostSecretNameLength)

		exactMatch := prefix + "tok"
		nearMiss := prefix[:len(prefix)-1] + "ytok"
		fake.listNames = []string{exactMatch, nearMiss}

		c := NewHostServiceClient(conn)
		got, err := c.ListSecretsByPrefix(context.Background(), prefix)
		require.NoError(t, err)
		assert.Equal(t, 1, fake.listCalls)
		assert.LessOrEqual(t, len(fake.lastListPattern), MaxHostSecretPatternLength)
		_, compileErr := regexp.Compile(fake.lastListPattern)
		require.NoError(t, compileErr)
		assert.Equal(t, []string{exactMatch}, got)
	})

	t.Run("handles 256-byte prefix and invalid UTF-8 fallback", func(t *testing.T) {
		t.Parallel()
		fake, conn, cleanup := startFakeHostServiceWithFake(t)
		defer cleanup()

		maxPrefix := strings.Repeat("a", MaxHostSecretNameLength)
		fake.listNames = []string{maxPrefix, strings.Repeat("a", MaxHostSecretNameLength-1) + "b"}
		c := NewHostServiceClient(conn)

		got, err := c.ListSecretsByPrefix(context.Background(), maxPrefix)
		require.NoError(t, err)
		assert.Len(t, fake.lastListPattern, MaxHostSecretPatternLength)
		assert.Equal(t, []string{maxPrefix}, got)

		// Leading invalid UTF-8 when expansion exceeds limit falls back to empty pattern + HasPrefix filter.
		assert.Equal(t, "", escapedPrefixPattern("\xff"+strings.Repeat(".", 130)))
	})

	t.Run("fails fast before RPC when prefix exceeds MaxHostSecretNameLength", func(t *testing.T) {
		t.Parallel()
		fake, conn, cleanup := startFakeHostServiceWithFake(t)
		defer cleanup()
		c := NewHostServiceClient(conn)

		tooLongPrefix := strings.Repeat("a", MaxHostSecretNameLength+1)
		_, err := c.ListSecretsByPrefix(context.Background(), tooLongPrefix)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds maximum of 256 bytes")
		assert.Equal(t, 0, fake.listCalls)
	})

	t.Run("propagates ListSecrets error", func(t *testing.T) {
		t.Parallel()
		_, conn, cleanup := startFakeHostServiceWithFake(t)
		defer cleanup()
		c := NewHostServiceClient(conn)

		_, err := c.ListSecretsByPrefix(context.Background(), "error")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "list error")
	})
}
