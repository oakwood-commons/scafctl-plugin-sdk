// Copyright 2025-2026 Oakwood Commons
// SPDX-License-Identifier: Apache-2.0

package plugin

import (
	"context"
	"net"
	"testing"

	"github.com/oakwood-commons/scafctl-plugin-sdk/plugin/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// fakeHostService is a minimal in-process HostService for testing.
type fakeHostService struct {
	proto.UnimplementedHostServiceServer
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
	if req.Pattern == "error" {
		return &proto.ListSecretsResponse{Error: "list error"}, nil
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
	if req.HandlerName == "error" {
		return &proto.GetAuthTokenResponse{Error: "token error"}, nil
	}
	return &proto.GetAuthTokenResponse{AccessToken: "tok123", TokenType: "Bearer"}, nil
}

func (f *fakeHostService) GetAuthGroups(_ context.Context, req *proto.GetAuthGroupsRequest) (*proto.GetAuthGroupsResponse, error) {
	if req.HandlerName == "error" {
		return &proto.GetAuthGroupsResponse{Error: "groups error"}, nil
	}
	return &proto.GetAuthGroupsResponse{Groups: []string{"group-a", "group-b"}}, nil
}

func startFakeHostService(t *testing.T) (*grpc.ClientConn, func()) {
	t.Helper()
	lis, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)

	s := grpc.NewServer()
	proto.RegisterHostServiceServer(s, &fakeHostService{})

	go func() { _ = s.Serve(lis) }()

	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)

	return conn, func() { conn.Close(); s.Stop() }
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

func TestHostServiceClient_GetAuthGroups(t *testing.T) {
	conn, cleanup := startFakeHostService(t)
	defer cleanup()
	c := NewHostServiceClient(conn)

	groups, err := c.GetAuthGroups(context.Background(), "entra")
	require.NoError(t, err)
	assert.Equal(t, []string{"group-a", "group-b"}, groups)
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
