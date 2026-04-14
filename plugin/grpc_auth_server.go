// Copyright 2025-2026 Oakwood Commons
// SPDX-License-Identifier: Apache-2.0

package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"
	goplugin "github.com/hashicorp/go-plugin"
	"github.com/oakwood-commons/scafctl-plugin-sdk/auth"
	"github.com/oakwood-commons/scafctl-plugin-sdk/plugin/proto"
	"google.golang.org/grpc"
)

// AuthHandlerGRPCServer implements the gRPC server for auth handler plugins.
type AuthHandlerGRPCServer struct {
	proto.UnimplementedAuthHandlerServiceServer
	Impl   AuthHandlerPlugin
	broker *goplugin.GRPCBroker
}

//nolint:revive
func (s *AuthHandlerGRPCServer) GetAuthHandlers(ctx context.Context, _ *proto.GetAuthHandlersRequest) (*proto.GetAuthHandlersResponse, error) {
	handlers, err := s.Impl.GetAuthHandlers(ctx)
	if err != nil {
		return nil, fmt.Errorf("GetAuthHandlers: %w", err)
	}
	resp := &proto.GetAuthHandlersResponse{Handlers: make([]*proto.AuthHandlerInfo, len(handlers))}
	for i, h := range handlers {
		flows := make([]string, len(h.Flows))
		for j, f := range h.Flows {
			flows[j] = string(f)
		}
		caps := make([]string, len(h.Capabilities))
		for j, c := range h.Capabilities {
			caps[j] = string(c)
		}
		resp.Handlers[i] = &proto.AuthHandlerInfo{Name: h.Name, DisplayName: h.DisplayName, Flows: flows, Capabilities: caps}
	}
	return resp, nil
}

func (s *AuthHandlerGRPCServer) Login(req *proto.LoginRequest, stream grpc.ServerStreamingServer[proto.LoginStreamMessage]) error {
	ctx := stream.Context()
	lgr := logr.FromContextOrDiscard(ctx)
	var sendFailed atomic.Bool
	deviceCodeCb := func(prompt DeviceCodePrompt) {
		if sendFailed.Load() {
			return
		}
		if err := stream.Send(&proto.LoginStreamMessage{
			Payload: &proto.LoginStreamMessage_DeviceCodePrompt{
				DeviceCodePrompt: &proto.DeviceCodePrompt{
					UserCode: prompt.UserCode, VerificationUri: prompt.VerificationURI, Message: prompt.Message,
				},
			},
		}); err != nil {
			sendFailed.Store(true)
			lgr.V(1).Info("failed to send device code prompt", "error", err)
		}
	}
	loginReq := LoginRequest{
		TenantID: req.TenantId, Scopes: req.Scopes,
		Flow: auth.Flow(req.Flow), Timeout: time.Duration(req.TimeoutSeconds) * time.Second,
	}
	result, err := s.Impl.Login(ctx, req.HandlerName, loginReq, deviceCodeCb)
	if err != nil {
		lgr.V(1).Info("login failed", "handler", req.HandlerName, "error", err)
		if sendErr := stream.Send(&proto.LoginStreamMessage{Payload: &proto.LoginStreamMessage_Error{Error: err.Error()}}); sendErr != nil {
			return fmt.Errorf("Login %q failed and could not send error to host: %w", req.HandlerName, sendErr)
		}
		return nil
	}
	if result == nil {
		if sendErr := stream.Send(&proto.LoginStreamMessage{Payload: &proto.LoginStreamMessage_Error{Error: "plugin returned nil login result"}}); sendErr != nil {
			return fmt.Errorf("Login %q: nil result and could not send error to host: %w", req.HandlerName, sendErr)
		}
		return nil
	}
	return stream.Send(&proto.LoginStreamMessage{
		Payload: &proto.LoginStreamMessage_Result{
			Result: &proto.LoginResult{Claims: claimsToProto(result.Claims), ExpiresAtUnix: result.ExpiresAt.Unix()},
		},
	})
}

func (s *AuthHandlerGRPCServer) Logout(ctx context.Context, req *proto.LogoutRequest) (*proto.LogoutResponse, error) {
	if err := s.Impl.Logout(ctx, req.HandlerName); err != nil {
		return nil, fmt.Errorf("Logout %q: %w", req.HandlerName, err)
	}
	return &proto.LogoutResponse{}, nil
}

func (s *AuthHandlerGRPCServer) GetStatus(ctx context.Context, req *proto.GetStatusRequest) (*proto.GetStatusResponse, error) {
	st, err := s.Impl.GetStatus(ctx, req.HandlerName)
	if err != nil {
		return nil, fmt.Errorf("GetStatus %q: %w", req.HandlerName, err)
	}
	return statusToProto(st), nil
}

func (s *AuthHandlerGRPCServer) GetToken(ctx context.Context, req *proto.GetTokenRequest) (*proto.GetTokenResponse, error) {
	tokenReq := TokenRequest{Scope: req.Scope, MinValidFor: time.Duration(req.MinValidForSeconds) * time.Second, ForceRefresh: req.ForceRefresh}
	token, err := s.Impl.GetToken(ctx, req.HandlerName, tokenReq)
	if err != nil {
		return nil, fmt.Errorf("GetToken %q: %w", req.HandlerName, err)
	}
	return tokenResponseToProto(token), nil
}

func (s *AuthHandlerGRPCServer) ListCachedTokens(ctx context.Context, req *proto.ListCachedTokensRequest) (*proto.ListCachedTokensResponse, error) {
	tokens, err := s.Impl.ListCachedTokens(ctx, req.HandlerName)
	if err != nil {
		return nil, fmt.Errorf("ListCachedTokens %q: %w", req.HandlerName, err)
	}
	resp := &proto.ListCachedTokensResponse{Tokens: make([]*proto.CachedTokenInfo, len(tokens))}
	for i, t := range tokens {
		resp.Tokens[i] = cachedTokenInfoToProto(t)
	}
	return resp, nil
}

func (s *AuthHandlerGRPCServer) PurgeExpiredTokens(ctx context.Context, req *proto.PurgeExpiredTokensRequest) (*proto.PurgeExpiredTokensResponse, error) {
	count, err := s.Impl.PurgeExpiredTokens(ctx, req.HandlerName)
	if err != nil {
		return nil, fmt.Errorf("PurgeExpiredTokens %q: %w", req.HandlerName, err)
	}
	if count > math.MaxInt32 {
		count = math.MaxInt32
	}
	return &proto.PurgeExpiredTokensResponse{PurgedCount: int32(count)}, nil
}

func (s *AuthHandlerGRPCServer) ConfigureAuthHandler(ctx context.Context, req *proto.ConfigureAuthHandlerRequest) (*proto.ConfigureAuthHandlerResponse, error) {
	settings := make(map[string]json.RawMessage, len(req.Settings))
	for k, v := range req.Settings {
		settings[k] = json.RawMessage(v)
	}
	cfg := ProviderConfig{Quiet: req.Quiet, NoColor: req.NoColor, BinaryName: req.BinaryName, Settings: settings}
	if req.HostServiceId != 0 && s.broker != nil {
		cfg.HostServiceID = req.HostServiceId
	}
	if err := s.Impl.ConfigureAuthHandler(ctx, req.HandlerName, cfg); err != nil {
		return &proto.ConfigureAuthHandlerResponse{Error: err.Error()}, nil //nolint:nilerr
	}
	return &proto.ConfigureAuthHandlerResponse{ProtocolVersion: PluginProtocolVersion}, nil
}

func (s *AuthHandlerGRPCServer) StopAuthHandler(ctx context.Context, req *proto.StopAuthHandlerRequest) (*proto.StopAuthHandlerResponse, error) {
	if err := s.Impl.StopAuthHandler(ctx, req.HandlerName); err != nil {
		return &proto.StopAuthHandlerResponse{Error: err.Error()}, nil //nolint:nilerr
	}
	return &proto.StopAuthHandlerResponse{}, nil
}

// ---- Conversion helpers ----

func claimsToProto(c *auth.Claims) *proto.Claims {
	if c == nil {
		return nil
	}
	return &proto.Claims{
		Issuer: c.Issuer, Subject: c.Subject, TenantId: c.TenantID,
		ObjectId: c.ObjectID, ClientId: c.ClientID, Email: c.Email,
		Name: c.Name, Username: c.Username,
		IssuedAtUnix: c.IssuedAt.Unix(), ExpiresAtUnix: c.ExpiresAt.Unix(),
	}
}

func safeUnix(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.Unix()
}

func statusToProto(s *auth.Status) *proto.GetStatusResponse {
	if s == nil {
		return &proto.GetStatusResponse{}
	}
	return &proto.GetStatusResponse{
		Authenticated: s.Authenticated, Reason: s.Reason, Claims: claimsToProto(s.Claims),
		ExpiresAtUnix: safeUnix(s.ExpiresAt), LastRefreshUnix: safeUnix(s.LastRefresh),
		TenantId: s.TenantID, IdentityType: string(s.IdentityType),
		ClientId: s.ClientID, TokenFile: s.TokenFile, Scopes: s.Scopes,
	}
}

func tokenResponseToProto(t *TokenResponse) *proto.GetTokenResponse {
	if t == nil {
		return &proto.GetTokenResponse{}
	}
	return &proto.GetTokenResponse{
		AccessToken: t.AccessToken, TokenType: t.TokenType,
		ExpiresAtUnix: safeUnix(t.ExpiresAt), Scope: t.Scope,
		CachedAtUnix: safeUnix(t.CachedAt), Flow: string(t.Flow), SessionId: t.SessionID,
	}
}

func cachedTokenInfoToProto(t *auth.CachedTokenInfo) *proto.CachedTokenInfo {
	if t == nil {
		return &proto.CachedTokenInfo{}
	}
	return &proto.CachedTokenInfo{
		Handler: t.Handler, TokenKind: t.TokenKind, Scope: t.Scope,
		TokenType: t.TokenType, Flow: string(t.Flow), Fingerprint: t.Fingerprint,
		ExpiresAtUnix: safeUnix(t.ExpiresAt), CachedAtUnix: safeUnix(t.CachedAt),
		IsExpired: t.IsExpired, SessionId: t.SessionID,
	}
}
