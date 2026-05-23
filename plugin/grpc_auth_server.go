// Copyright 2025-2026 Oakwood Commons
// SPDX-License-Identifier: Apache-2.0

package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sync"
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
	mu     sync.RWMutex
	conn   *grpc.ClientConn
	// hostClient is the HostServiceClient for secret store operations.
	// Established during ConfigureAuthHandler when the host provides a
	// HostServiceId via the gRPC broker.
	hostClient *HostServiceClient
	// closed is set by closeHostClient to prevent ensureHostClient from
	// committing a connection after the handler has been stopped.
	closed bool
	// dialFunc overrides broker.Dial; set in tests to avoid a real GRPCBroker.
	dialFunc func(id uint32) (*grpc.ClientConn, error)
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
	ctx := s.injectHostClient(stream.Context())
	ctx = auth.WithProfile(ctx, req.Profile)
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
	ctx = s.injectHostClient(ctx)
	ctx = auth.WithProfile(ctx, req.Profile)
	if err := s.Impl.Logout(ctx, req.HandlerName); err != nil {
		return nil, fmt.Errorf("Logout %q: %w", req.HandlerName, err)
	}
	return &proto.LogoutResponse{}, nil
}

func (s *AuthHandlerGRPCServer) GetStatus(ctx context.Context, req *proto.GetStatusRequest) (*proto.GetStatusResponse, error) {
	ctx = s.injectHostClient(ctx)
	ctx = auth.WithProfile(ctx, req.Profile)
	st, err := s.Impl.GetStatus(ctx, req.HandlerName)
	if err != nil {
		return nil, fmt.Errorf("GetStatus %q: %w", req.HandlerName, err)
	}
	return statusToProto(st), nil
}

func (s *AuthHandlerGRPCServer) GetToken(ctx context.Context, req *proto.GetTokenRequest) (*proto.GetTokenResponse, error) {
	ctx = s.injectHostClient(ctx)
	ctx = auth.WithProfile(ctx, req.Profile)
	tokenReq := TokenRequest{Scope: req.Scope, MinValidFor: time.Duration(req.MinValidForSeconds) * time.Second, ForceRefresh: req.ForceRefresh}
	token, err := s.Impl.GetToken(ctx, req.HandlerName, tokenReq)
	if err != nil {
		return nil, fmt.Errorf("GetToken %q: %w", req.HandlerName, err)
	}
	return tokenResponseToProto(token), nil
}

func (s *AuthHandlerGRPCServer) ListCachedTokens(ctx context.Context, req *proto.ListCachedTokensRequest) (*proto.ListCachedTokensResponse, error) {
	ctx = s.injectHostClient(ctx)
	ctx = auth.WithProfile(ctx, req.Profile)
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
	ctx = s.injectHostClient(ctx)
	ctx = auth.WithProfile(ctx, req.Profile)
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
	cfg := ProviderConfig{Quiet: req.Quiet, NoColor: req.NoColor, BinaryName: req.BinaryName, Settings: settings, HostServiceID: req.HostServiceId}
	// Dial the host's HostService broker if an ID was provided and a dial
	// function is available. When no broker or dialFunc is set (e.g. in tests),
	// this is a no-op.
	if req.HostServiceId != 0 {
		s.mu.Lock()
		s.closed = false
		s.mu.Unlock()
		if err := s.ensureHostClient(req.HostServiceId); err != nil {
			return &proto.ConfigureAuthHandlerResponse{Error: fmt.Sprintf("failed to dial host service: %v", err)}, nil //nolint:nilerr
		}
	}
	ctx = s.injectHostClient(ctx)
	if err := s.Impl.ConfigureAuthHandler(ctx, req.HandlerName, cfg); err != nil {
		s.closeHostClient(ctx)
		return &proto.ConfigureAuthHandlerResponse{Error: err.Error()}, nil //nolint:nilerr
	}
	return &proto.ConfigureAuthHandlerResponse{ProtocolVersion: PluginProtocolVersion}, nil
}

func (s *AuthHandlerGRPCServer) StopAuthHandler(ctx context.Context, req *proto.StopAuthHandlerRequest) (*proto.StopAuthHandlerResponse, error) {
	if err := s.Impl.StopAuthHandler(ctx, req.HandlerName); err != nil {
		return &proto.StopAuthHandlerResponse{Error: err.Error()}, nil //nolint:nilerr
	}
	s.closeHostClient(ctx)
	return &proto.StopAuthHandlerResponse{}, nil
}

// closeHostClient tears down the host service connection and clears the cached
// client, if any.
func (s *AuthHandlerGRPCServer) closeHostClient(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	if s.conn != nil {
		if err := s.conn.Close(); err != nil {
			logr.FromContextOrDiscard(ctx).V(1).Info("close host conn", "error", err)
		}
		s.conn = nil
	}
	s.hostClient = nil
}

// ensureHostClient dials the host service if not already connected. The dial
// happens outside the lock so slow connections don't block concurrent readers.
func (s *AuthHandlerGRPCServer) ensureHostClient(hostServiceID uint32) error {
	dialFn := s.pickDialFunc()
	if dialFn == nil {
		return nil
	}
	s.mu.RLock()
	already := s.hostClient != nil
	s.mu.RUnlock()
	if already {
		return nil
	}
	conn, err := dialFn(hostServiceID)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.hostClient != nil || s.closed {
		// Another goroutine won the race, or the handler was stopped while
		// we were dialing; close the connection we just opened.
		_ = conn.Close()
		return nil
	}
	s.conn = conn
	s.hostClient = NewHostServiceClient(conn)
	return nil
}

// injectHostClient adds the HostServiceClient to the context if one was
// established during ConfigureAuthHandler.
func (s *AuthHandlerGRPCServer) injectHostClient(ctx context.Context) context.Context {
	s.mu.RLock()
	hc := s.hostClient
	s.mu.RUnlock()
	if hc != nil {
		return WithHostClient(ctx, hc)
	}
	return ctx
}

// pickDialFunc returns the effective dial function: the test override if set,
// otherwise broker.Dial, or nil if neither is available.
func (s *AuthHandlerGRPCServer) pickDialFunc() func(id uint32) (*grpc.ClientConn, error) {
	if s.dialFunc != nil {
		return s.dialFunc
	}
	if s.broker != nil {
		return s.broker.Dial
	}
	return nil
}

func (s *AuthHandlerGRPCServer) DetectAvailableFlows(ctx context.Context, req *proto.DetectAvailableFlowsRequest) (*proto.DetectAvailableFlowsResponse, error) {
	ctx = s.injectHostClient(ctx)
	flows, err := s.Impl.DetectAvailableFlows(ctx, req.HandlerName)
	if err != nil {
		return &proto.DetectAvailableFlowsResponse{Error: err.Error()}, nil //nolint:nilerr
	}
	protoFlows := make([]*proto.FlowAvailability, len(flows))
	for i, f := range flows {
		protoFlows[i] = &proto.FlowAvailability{
			Flow:      string(f.Flow),
			Available: f.Available,
			Reason:    f.Reason,
		}
	}
	return &proto.DetectAvailableFlowsResponse{Flows: protoFlows}, nil
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
