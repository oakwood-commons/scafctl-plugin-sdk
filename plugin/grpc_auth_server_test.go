// Copyright 2025-2026 Oakwood Commons
// SPDX-License-Identifier: Apache-2.0

package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"testing"
	"time"

	goplugin "github.com/hashicorp/go-plugin"
	"github.com/oakwood-commons/scafctl-plugin-sdk/auth"
	"github.com/oakwood-commons/scafctl-plugin-sdk/plugin/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

// --- mock auth handler ---

type mockAuthHandler struct {
	getAuthHandlers      func(ctx context.Context) ([]AuthHandlerInfo, error)
	configureAuthHandler func(ctx context.Context, name string, cfg ProviderConfig) error
	login                func(ctx context.Context, name string, req LoginRequest, cb func(DeviceCodePrompt)) (*LoginResponse, error)
	logout               func(ctx context.Context, name string) error
	getStatus            func(ctx context.Context, name string) (*auth.Status, error)
	getToken             func(ctx context.Context, name string, req TokenRequest) (*TokenResponse, error)
	listCachedTokens     func(ctx context.Context, name string) ([]*auth.CachedTokenInfo, error)
	purgeExpiredTokens   func(ctx context.Context, name string) (int, error)
	stopAuthHandler      func(ctx context.Context, name string) error
}

//nolint:dupl // mock mirrors AuthHandlerPlugin interface
func (m *mockAuthHandler) GetAuthHandlers(ctx context.Context) ([]AuthHandlerInfo, error) {
	if m.getAuthHandlers != nil {
		return m.getAuthHandlers(ctx)
	}
	return nil, nil
}

func (m *mockAuthHandler) ConfigureAuthHandler(ctx context.Context, name string, cfg ProviderConfig) error {
	if m.configureAuthHandler != nil {
		return m.configureAuthHandler(ctx, name, cfg)
	}
	return nil
}

func (m *mockAuthHandler) Login(ctx context.Context, name string, req LoginRequest, cb func(DeviceCodePrompt)) (*LoginResponse, error) {
	if m.login != nil {
		return m.login(ctx, name, req, cb)
	}
	return nil, errors.New("not implemented")
}

func (m *mockAuthHandler) Logout(ctx context.Context, name string) error {
	if m.logout != nil {
		return m.logout(ctx, name)
	}
	return nil
}

func (m *mockAuthHandler) GetStatus(ctx context.Context, name string) (*auth.Status, error) {
	if m.getStatus != nil {
		return m.getStatus(ctx, name)
	}
	return nil, nil
}

func (m *mockAuthHandler) GetToken(ctx context.Context, name string, req TokenRequest) (*TokenResponse, error) {
	if m.getToken != nil {
		return m.getToken(ctx, name, req)
	}
	return nil, errors.New("not implemented")
}

func (m *mockAuthHandler) ListCachedTokens(ctx context.Context, name string) ([]*auth.CachedTokenInfo, error) {
	if m.listCachedTokens != nil {
		return m.listCachedTokens(ctx, name)
	}
	return nil, nil
}

func (m *mockAuthHandler) PurgeExpiredTokens(ctx context.Context, name string) (int, error) {
	if m.purgeExpiredTokens != nil {
		return m.purgeExpiredTokens(ctx, name)
	}
	return 0, nil
}

func (m *mockAuthHandler) StopAuthHandler(ctx context.Context, name string) error {
	if m.stopAuthHandler != nil {
		return m.stopAuthHandler(ctx, name)
	}
	return nil
}

// --- mock login stream ---

type mockLoginStream struct {
	grpc.ServerStreamingServer[proto.LoginStreamMessage]
	ctx      context.Context
	messages []*proto.LoginStreamMessage
}

func (s *mockLoginStream) Context() context.Context { return s.ctx }
func (s *mockLoginStream) Send(msg *proto.LoginStreamMessage) error {
	s.messages = append(s.messages, msg)
	return nil
}

// --- Tests ---

func TestAuthGRPCServer_GetAuthHandlers(t *testing.T) {
	srv := &AuthHandlerGRPCServer{Impl: &mockAuthHandler{
		getAuthHandlers: func(_ context.Context) ([]AuthHandlerInfo, error) {
			return []AuthHandlerInfo{
				{Name: "github", DisplayName: "GitHub", Flows: []auth.Flow{auth.FlowDeviceCode}, Capabilities: []auth.Capability{auth.CapScopesOnLogin}},
			}, nil
		},
	}}
	resp, err := srv.GetAuthHandlers(context.Background(), &proto.GetAuthHandlersRequest{})
	require.NoError(t, err)
	require.Len(t, resp.Handlers, 1)
	assert.Equal(t, "github", resp.Handlers[0].Name)
	assert.Equal(t, "GitHub", resp.Handlers[0].DisplayName)
	assert.Equal(t, []string{"device_code"}, resp.Handlers[0].Flows)
	assert.Equal(t, []string{"scopes_on_login"}, resp.Handlers[0].Capabilities)
}

func TestAuthGRPCServer_GetAuthHandlers_Error(t *testing.T) {
	srv := &AuthHandlerGRPCServer{Impl: &mockAuthHandler{
		getAuthHandlers: func(_ context.Context) ([]AuthHandlerInfo, error) {
			return nil, errors.New("fail")
		},
	}}
	_, err := srv.GetAuthHandlers(context.Background(), &proto.GetAuthHandlersRequest{})
	require.Error(t, err)
}

func TestAuthGRPCServer_Login_Success(t *testing.T) {
	now := time.Now()
	srv := &AuthHandlerGRPCServer{Impl: &mockAuthHandler{
		login: func(_ context.Context, _ string, req LoginRequest, cb func(DeviceCodePrompt)) (*LoginResponse, error) {
			cb(DeviceCodePrompt{UserCode: "ABC-123", VerificationURI: "https://example.com", Message: "go here"})
			return &LoginResponse{
				Claims:    &auth.Claims{Email: "user@ex.com"},
				ExpiresAt: now,
			}, nil
		},
	}}
	stream := &mockLoginStream{ctx: context.Background()}
	err := srv.Login(&proto.LoginRequest{HandlerName: "gh", Flow: "device_code", TimeoutSeconds: 60}, stream)
	require.NoError(t, err)
	require.Len(t, stream.messages, 2)
	assert.NotNil(t, stream.messages[0].GetDeviceCodePrompt())
	assert.Equal(t, "ABC-123", stream.messages[0].GetDeviceCodePrompt().UserCode)
	assert.NotNil(t, stream.messages[1].GetResult())
}

func TestAuthGRPCServer_Login_Error(t *testing.T) {
	srv := &AuthHandlerGRPCServer{Impl: &mockAuthHandler{
		login: func(_ context.Context, _ string, _ LoginRequest, _ func(DeviceCodePrompt)) (*LoginResponse, error) {
			return nil, errors.New("login failed")
		},
	}}
	stream := &mockLoginStream{ctx: context.Background()}
	err := srv.Login(&proto.LoginRequest{HandlerName: "gh"}, stream)
	require.NoError(t, err) // error sent via stream
	require.Len(t, stream.messages, 1)
	assert.Equal(t, "login failed", stream.messages[0].GetError())
}

func TestAuthGRPCServer_Logout(t *testing.T) {
	srv := &AuthHandlerGRPCServer{Impl: &mockAuthHandler{}}
	resp, err := srv.Logout(context.Background(), &proto.LogoutRequest{HandlerName: "gh"})
	require.NoError(t, err)
	assert.NotNil(t, resp)
}

func TestAuthGRPCServer_Logout_Error(t *testing.T) {
	srv := &AuthHandlerGRPCServer{Impl: &mockAuthHandler{
		logout: func(_ context.Context, _ string) error { return errors.New("fail") },
	}}
	_, err := srv.Logout(context.Background(), &proto.LogoutRequest{HandlerName: "gh"})
	require.Error(t, err)
}

func TestAuthGRPCServer_GetStatus(t *testing.T) {
	now := time.Now()
	srv := &AuthHandlerGRPCServer{Impl: &mockAuthHandler{
		getStatus: func(_ context.Context, _ string) (*auth.Status, error) {
			return &auth.Status{
				Authenticated: true,
				Reason:        "token valid",
				Claims:        &auth.Claims{Email: "u@x.com"},
				ExpiresAt:     now,
				TenantID:      "t1",
				IdentityType:  auth.IdentityTypeUser,
				Scopes:        []string{"read"},
			}, nil
		},
	}}
	resp, err := srv.GetStatus(context.Background(), &proto.GetStatusRequest{HandlerName: "gh"})
	require.NoError(t, err)
	assert.True(t, resp.Authenticated)
	assert.Equal(t, "token valid", resp.Reason)
	assert.Equal(t, "u@x.com", resp.Claims.Email)
	assert.Equal(t, "t1", resp.TenantId)
}

func TestAuthGRPCServer_GetStatus_Nil(t *testing.T) {
	srv := &AuthHandlerGRPCServer{Impl: &mockAuthHandler{
		getStatus: func(_ context.Context, _ string) (*auth.Status, error) {
			return nil, nil
		},
	}}
	resp, err := srv.GetStatus(context.Background(), &proto.GetStatusRequest{HandlerName: "gh"})
	require.NoError(t, err)
	assert.False(t, resp.Authenticated)
}

func TestAuthGRPCServer_GetToken(t *testing.T) {
	now := time.Now()
	srv := &AuthHandlerGRPCServer{Impl: &mockAuthHandler{
		getToken: func(_ context.Context, _ string, req TokenRequest) (*TokenResponse, error) {
			return &TokenResponse{
				AccessToken: "tok123", TokenType: "Bearer",
				ExpiresAt: now, Scope: req.Scope, Flow: auth.FlowDeviceCode,
			}, nil
		},
	}}
	resp, err := srv.GetToken(context.Background(), &proto.GetTokenRequest{
		HandlerName: "gh", Scope: "read", MinValidForSeconds: 60, ForceRefresh: true,
	})
	require.NoError(t, err)
	assert.Equal(t, "tok123", resp.AccessToken)
	assert.Equal(t, "Bearer", resp.TokenType)
}

func TestAuthGRPCServer_ListCachedTokens(t *testing.T) {
	srv := &AuthHandlerGRPCServer{Impl: &mockAuthHandler{
		listCachedTokens: func(_ context.Context, _ string) ([]*auth.CachedTokenInfo, error) {
			return []*auth.CachedTokenInfo{
				{Handler: "gh", TokenKind: "access", Scope: "read", Flow: auth.FlowDeviceCode, IsExpired: false},
			}, nil
		},
	}}
	resp, err := srv.ListCachedTokens(context.Background(), &proto.ListCachedTokensRequest{HandlerName: "gh"})
	require.NoError(t, err)
	require.Len(t, resp.Tokens, 1)
	assert.Equal(t, "gh", resp.Tokens[0].Handler)
}

func TestAuthGRPCServer_PurgeExpiredTokens(t *testing.T) {
	srv := &AuthHandlerGRPCServer{Impl: &mockAuthHandler{
		purgeExpiredTokens: func(_ context.Context, _ string) (int, error) { return 3, nil },
	}}
	resp, err := srv.PurgeExpiredTokens(context.Background(), &proto.PurgeExpiredTokensRequest{HandlerName: "gh"})
	require.NoError(t, err)
	assert.Equal(t, int32(3), resp.PurgedCount)
}

func TestAuthGRPCServer_ConfigureAuthHandler(t *testing.T) {
	var gotCfg ProviderConfig
	srv := &AuthHandlerGRPCServer{
		Impl: &mockAuthHandler{
			configureAuthHandler: func(_ context.Context, _ string, cfg ProviderConfig) error {
				gotCfg = cfg
				return nil
			},
		},
		broker: &goplugin.GRPCBroker{},
	}
	resp, err := srv.ConfigureAuthHandler(context.Background(), &proto.ConfigureAuthHandlerRequest{
		HandlerName: "gh", Quiet: true, NoColor: true, BinaryName: "bin",
		HostServiceId: 42, Settings: map[string][]byte{"k": []byte(`"v"`)},
	})
	require.NoError(t, err)
	assert.Empty(t, resp.Error)
	assert.Equal(t, PluginProtocolVersion, resp.ProtocolVersion)
	assert.True(t, gotCfg.Quiet)
	assert.Equal(t, uint32(42), gotCfg.HostServiceID)
}

func TestAuthGRPCServer_ConfigureAuthHandler_NoBroker(t *testing.T) {
	var gotCfg ProviderConfig
	srv := &AuthHandlerGRPCServer{
		Impl: &mockAuthHandler{
			configureAuthHandler: func(_ context.Context, _ string, cfg ProviderConfig) error {
				gotCfg = cfg
				return nil
			},
		},
	}
	resp, err := srv.ConfigureAuthHandler(context.Background(), &proto.ConfigureAuthHandlerRequest{
		HandlerName: "gh", HostServiceId: 42,
	})
	require.NoError(t, err)
	assert.Empty(t, resp.Error)
	// HostServiceID should not be set when broker is nil
	assert.Equal(t, uint32(0), gotCfg.HostServiceID)
}

func TestAuthGRPCServer_ConfigureAuthHandler_Error(t *testing.T) {
	srv := &AuthHandlerGRPCServer{Impl: &mockAuthHandler{
		configureAuthHandler: func(_ context.Context, _ string, _ ProviderConfig) error {
			return errors.New("config fail")
		},
	}}
	resp, err := srv.ConfigureAuthHandler(context.Background(), &proto.ConfigureAuthHandlerRequest{HandlerName: "gh"})
	require.NoError(t, err)
	assert.Equal(t, "config fail", resp.Error)
}

func TestAuthGRPCServer_StopAuthHandler(t *testing.T) {
	srv := &AuthHandlerGRPCServer{Impl: &mockAuthHandler{}}
	resp, err := srv.StopAuthHandler(context.Background(), &proto.StopAuthHandlerRequest{HandlerName: "gh"})
	require.NoError(t, err)
	assert.Empty(t, resp.Error)
}

func TestAuthGRPCServer_StopAuthHandler_Error(t *testing.T) {
	srv := &AuthHandlerGRPCServer{Impl: &mockAuthHandler{
		stopAuthHandler: func(_ context.Context, _ string) error { return errors.New("stop fail") },
	}}
	resp, err := srv.StopAuthHandler(context.Background(), &proto.StopAuthHandlerRequest{HandlerName: "gh"})
	require.NoError(t, err)
	assert.Equal(t, "stop fail", resp.Error)
}

// --- Conversion helper tests ---

func TestClaimsToProto_Nil(t *testing.T) {
	assert.Nil(t, claimsToProto(nil))
}

func TestClaimsToProto(t *testing.T) {
	now := time.Now()
	c := &auth.Claims{
		Issuer: "iss", Subject: "sub", TenantID: "tid", ObjectID: "oid",
		ClientID: "cid", Email: "e@x.com", Name: "N", Username: "u",
		IssuedAt: now, ExpiresAt: now.Add(time.Hour),
	}
	pc := claimsToProto(c)
	assert.Equal(t, "iss", pc.Issuer)
	assert.Equal(t, "sub", pc.Subject)
	assert.Equal(t, "e@x.com", pc.Email)
	assert.Equal(t, now.Unix(), pc.IssuedAtUnix)
}

func TestStatusToProto_Nil(t *testing.T) {
	resp := statusToProto(nil)
	assert.NotNil(t, resp)
	assert.False(t, resp.Authenticated)
}

func TestTokenResponseToProto_Nil(t *testing.T) {
	resp := tokenResponseToProto(nil)
	assert.NotNil(t, resp)
	assert.Empty(t, resp.AccessToken)
}

func TestTokenResponseToProto(t *testing.T) {
	now := time.Now()
	tr := &TokenResponse{
		AccessToken: "tok", TokenType: "Bearer", ExpiresAt: now,
		Scope: "read", Flow: auth.FlowDeviceCode, SessionID: "sess",
	}
	resp := tokenResponseToProto(tr)
	assert.Equal(t, "tok", resp.AccessToken)
	assert.Equal(t, "device_code", resp.Flow)
}

func TestCachedTokenInfoToProto_Nil(t *testing.T) {
	resp := cachedTokenInfoToProto(nil)
	assert.NotNil(t, resp)
	assert.Empty(t, resp.Handler)
}

func TestCachedTokenInfoToProto(t *testing.T) {
	info := &auth.CachedTokenInfo{
		Handler: "gh", TokenKind: "access", Scope: "read",
		Flow: auth.FlowPAT, IsExpired: true, SessionID: "sess",
		Fingerprint: "abc123",
	}
	resp := cachedTokenInfoToProto(info)
	assert.Equal(t, "gh", resp.Handler)
	assert.True(t, resp.IsExpired)
	assert.Equal(t, "pat", resp.Flow)
	assert.Equal(t, "abc123", resp.Fingerprint)
}

// --- GRPCPlugin tests ---

func TestGRPCPlugin_GRPCServer(t *testing.T) {
	p := &GRPCPlugin{Impl: &mockProvider{}}
	s := grpc.NewServer()
	defer s.Stop()
	err := p.GRPCServer(nil, s)
	require.NoError(t, err)
}

func TestAuthHandlerGRPCPlugin_GRPCServer(t *testing.T) {
	p := &AuthHandlerGRPCPlugin{Impl: &mockAuthHandler{}}
	s := grpc.NewServer()
	defer s.Stop()
	err := p.GRPCServer(nil, s)
	require.NoError(t, err)
}

// --- Interface/constants tests ---

func TestHandshakeConfig(t *testing.T) {
	hc := HandshakeConfig()
	assert.Equal(t, uint(1), hc.ProtocolVersion)
	assert.Equal(t, "SCAFCTL_PLUGIN", hc.MagicCookieKey)
	assert.Equal(t, "scafctl_provider_plugin", hc.MagicCookieValue)
}

func TestAuthHandlerHandshakeConfig(t *testing.T) {
	hc := AuthHandlerHandshakeConfig()
	assert.Equal(t, uint(1), hc.ProtocolVersion)
	assert.Equal(t, "SCAFCTL_AUTH_PLUGIN", hc.MagicCookieKey)
}

func TestPluginProtocolVersion(t *testing.T) {
	assert.Equal(t, int32(2), PluginProtocolVersion)
}

func TestPluginNames(t *testing.T) {
	assert.Equal(t, "provider", PluginName)
	assert.Equal(t, "auth-handler", AuthHandlerPluginName)
}

func TestErrStreamingNotSupported(t *testing.T) {
	assert.EqualError(t, ErrStreamingNotSupported, "streaming execution not supported")
}

func TestAuthGRPCServer_Login_NilResult(t *testing.T) {
	srv := &AuthHandlerGRPCServer{Impl: &mockAuthHandler{
		login: func(_ context.Context, _ string, _ LoginRequest, _ func(DeviceCodePrompt)) (*LoginResponse, error) {
			return nil, nil
		},
	}}
	stream := &mockLoginStream{ctx: context.Background()}
	err := srv.Login(&proto.LoginRequest{HandlerName: "gh"}, stream)
	require.NoError(t, err)
	require.Len(t, stream.messages, 1)
	assert.Equal(t, "plugin returned nil login result", stream.messages[0].GetError())
}

func TestAuthGRPCServer_Login_ErrorSendFails(t *testing.T) {
	srv := &AuthHandlerGRPCServer{Impl: &mockAuthHandler{
		login: func(_ context.Context, _ string, _ LoginRequest, _ func(DeviceCodePrompt)) (*LoginResponse, error) {
			return nil, errors.New("login failed")
		},
	}}
	stream := &failingSendLoginStream{ctx: context.Background()}
	err := srv.Login(&proto.LoginRequest{HandlerName: "gh"}, stream)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not send error to host")
}

func TestAuthGRPCServer_Login_NilResultSendFails(t *testing.T) {
	srv := &AuthHandlerGRPCServer{Impl: &mockAuthHandler{
		login: func(_ context.Context, _ string, _ LoginRequest, _ func(DeviceCodePrompt)) (*LoginResponse, error) {
			return nil, nil
		},
	}}
	stream := &failingSendLoginStream{ctx: context.Background()}
	err := srv.Login(&proto.LoginRequest{HandlerName: "gh"}, stream)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not send error to host")
}

func TestAuthGRPCServer_Login_DeviceCodeSendFails(t *testing.T) {
	srv := &AuthHandlerGRPCServer{Impl: &mockAuthHandler{
		login: func(_ context.Context, _ string, _ LoginRequest, cb func(DeviceCodePrompt)) (*LoginResponse, error) {
			cb(DeviceCodePrompt{UserCode: "X"})
			// Second call should be no-op after failure
			cb(DeviceCodePrompt{UserCode: "Y"})
			return &LoginResponse{Claims: &auth.Claims{Email: "u@x.com"}}, nil
		},
	}}
	stream := &failingSendLoginStream{ctx: context.Background()}
	err := srv.Login(&proto.LoginRequest{HandlerName: "gh"}, stream)
	// The result Send also fails
	require.Error(t, err)
}

func TestAuthGRPCServer_PurgeExpiredTokens_Overflow(t *testing.T) {
	srv := &AuthHandlerGRPCServer{Impl: &mockAuthHandler{
		purgeExpiredTokens: func(_ context.Context, _ string) (int, error) {
			return math.MaxInt64, nil
		},
	}}
	resp, err := srv.PurgeExpiredTokens(context.Background(), &proto.PurgeExpiredTokensRequest{HandlerName: "gh"})
	require.NoError(t, err)
	assert.Equal(t, int32(math.MaxInt32), resp.PurgedCount)
}

func TestAuthGRPCServer_GetStatus_Error(t *testing.T) {
	srv := &AuthHandlerGRPCServer{Impl: &mockAuthHandler{
		getStatus: func(_ context.Context, _ string) (*auth.Status, error) {
			return nil, errors.New("status fail")
		},
	}}
	_, err := srv.GetStatus(context.Background(), &proto.GetStatusRequest{HandlerName: "gh"})
	require.Error(t, err)
}

func TestAuthGRPCServer_GetToken_Error(t *testing.T) {
	srv := &AuthHandlerGRPCServer{Impl: &mockAuthHandler{
		getToken: func(_ context.Context, _ string, _ TokenRequest) (*TokenResponse, error) {
			return nil, errors.New("token fail")
		},
	}}
	_, err := srv.GetToken(context.Background(), &proto.GetTokenRequest{HandlerName: "gh"})
	require.Error(t, err)
}

func TestAuthGRPCServer_GetToken_Nil(t *testing.T) {
	srv := &AuthHandlerGRPCServer{Impl: &mockAuthHandler{
		getToken: func(_ context.Context, _ string, _ TokenRequest) (*TokenResponse, error) {
			return nil, nil
		},
	}}
	resp, err := srv.GetToken(context.Background(), &proto.GetTokenRequest{HandlerName: "gh"})
	require.NoError(t, err)
	assert.Empty(t, resp.AccessToken)
}

func TestAuthGRPCServer_ListCachedTokens_Error(t *testing.T) {
	srv := &AuthHandlerGRPCServer{Impl: &mockAuthHandler{
		listCachedTokens: func(_ context.Context, _ string) ([]*auth.CachedTokenInfo, error) {
			return nil, errors.New("list fail")
		},
	}}
	_, err := srv.ListCachedTokens(context.Background(), &proto.ListCachedTokensRequest{HandlerName: "gh"})
	require.Error(t, err)
}

func TestAuthGRPCServer_PurgeExpiredTokens_Error(t *testing.T) {
	srv := &AuthHandlerGRPCServer{Impl: &mockAuthHandler{
		purgeExpiredTokens: func(_ context.Context, _ string) (int, error) {
			return 0, errors.New("purge fail")
		},
	}}
	_, err := srv.PurgeExpiredTokens(context.Background(), &proto.PurgeExpiredTokensRequest{HandlerName: "gh"})
	require.Error(t, err)
}

// --- failingSendLoginStream always returns an error on Send ---

type failingSendLoginStream struct {
	grpc.ServerStreamingServer[proto.LoginStreamMessage]
	ctx context.Context
}

func (s *failingSendLoginStream) Context() context.Context { return s.ctx }
func (s *failingSendLoginStream) Send(_ *proto.LoginStreamMessage) error {
	return errors.New("send failed")
}

// --- suppress unused import warnings ---
var _ = json.Marshal
