// Copyright 2025-2026 Oakwood Commons
// SPDX-License-Identifier: Apache-2.0

// Package plugintest provides contract-faithful test doubles for unit and
// integration testing scafctl provider and auth handler plugins.
//
// FakeHostService implements proto.HostServiceServer over an in-memory gRPC
// bufconn transport, enforcing both the RPC-level secret-name rules
// (plugin.ValidateHostSecretName) and the host backing-store naming rules
// (1..255 bytes, leading ASCII alphanumeric character, and only ASCII
// alphanumeric, '.', '_', or '-'), along with read-only and prefix scoping
// policies and regular-expression ListSecrets semantics.
package plugintest

import (
	"context"
	"errors"
	"fmt"
	"net"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/oakwood-commons/scafctl-plugin-sdk/plugin"
	"github.com/oakwood-commons/scafctl-plugin-sdk/plugin/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

const (
	defaultBufconnSize              = 1024 * 1024
	maxBackingStoreSecretNameLength = 255

	msgReadOnlyStore       = "access denied: secret store is read-only for provider plugins"
	msgOutsideAllowedScope = "access denied: secret name outside allowed scope"
	msgDeniedNamespace     = "access denied: secret name is in a reserved internal namespace"
	msgIdentityUnavailable = "auth identity not available"
	msgAuthSvcUnavailable  = "auth token service not available"
	msgGroupsUnavailable   = "auth groups not available"
	msgPromptUnimplemented = "host does not implement PromptAuthResponse"
)

// AuthIdentityFunc handles HostService.GetAuthIdentity callbacks on FakeHostService.
type AuthIdentityFunc func(ctx context.Context, req *proto.GetAuthIdentityRequest) (*proto.Claims, error)

// AuthHandlersFunc handles HostService.ListAuthHandlers callbacks on FakeHostService.
type AuthHandlersFunc func(ctx context.Context) (handlers []string, defaultHandler string, err error)

// AuthTokenFunc handles HostService.GetAuthToken callbacks on FakeHostService.
type AuthTokenFunc func(ctx context.Context, req *proto.GetAuthTokenRequest) (*proto.GetAuthTokenResponse, error)

// AuthGroupsFunc handles HostService.GetAuthGroups callbacks on FakeHostService.
type AuthGroupsFunc func(ctx context.Context, req *proto.GetAuthGroupsRequest) ([]string, error)

// PromptAuthResponseFunc handles HostService.PromptAuthResponse callbacks on FakeHostService.
type PromptAuthResponseFunc func(ctx context.Context, req *proto.PromptAuthResponseRequest) (string, error)

// Option configures a FakeHostService.
type Option func(*fakeHostConfig)

type fakeHostConfig struct {
	initialSecrets         map[string]string
	allowedSecretPrefix    string
	deniedSecretPrefixes   []string
	readOnlySecrets        bool
	getSecretErrFunc       func(ctx context.Context, name string) error
	setSecretErrFunc       func(ctx context.Context, name, value string) error
	deleteSecretErrFunc    func(ctx context.Context, name string) error
	listSecretsErrFunc     func(ctx context.Context, pattern string) error
	authIdentityFunc       AuthIdentityFunc
	authHandlersFunc       AuthHandlersFunc
	authTokenFunc          AuthTokenFunc
	authGroupsFunc         AuthGroupsFunc
	promptAuthResponseFunc PromptAuthResponseFunc
}

// WithInitialSecrets pre-populates the fake host's secret store before any
// read-only or prefix scoping restrictions take effect on incoming RPCs. Each
// secret name must satisfy both plugin.ValidateHostSecretName and the host
// backing-store naming rules (1..255 bytes, leading ASCII alphanumeric, and
// only ASCII alphanumeric, '.', '_', or '-').
func WithInitialSecrets(secrets map[string]string) Option {
	return func(cfg *fakeHostConfig) {
		if len(secrets) == 0 {
			return
		}
		if cfg.initialSecrets == nil {
			cfg.initialSecrets = make(map[string]string, len(secrets))
		}
		for k, v := range secrets {
			cfg.initialSecrets[k] = v
		}
	}
}

// WithAllowedSecretPrefix restricts GetSecret, SetSecret, and DeleteSecret to
// secret names starting with prefix, and filters ListSecrets to matching names.
// An empty prefix allows all valid secret names.
func WithAllowedSecretPrefix(prefix string) Option {
	return func(cfg *fakeHostConfig) {
		cfg.allowedSecretPrefix = prefix
	}
}

// WithDeniedSecretPrefixes rejects GetSecret, SetSecret, and DeleteSecret for
// secret names starting with any non-empty prefix in prefixes, and excludes
// those names from ListSecrets (for example []string{"scafctl."} for provider
// plugins).
func WithDeniedSecretPrefixes(prefixes []string) Option {
	return func(cfg *fakeHostConfig) {
		cfg.deniedSecretPrefixes = slices.Clone(prefixes)
	}
}

// WithReadOnlySecrets blocks SetSecret and DeleteSecret RPCs when readOnly is
// true, matching the host policy applied to provider plugins.
func WithReadOnlySecrets(readOnly bool) Option {
	return func(cfg *fakeHostConfig) {
		cfg.readOnlySecrets = readOnly
	}
}

// WithGetSecretErrorFunc injects an optional error callback evaluated on
// GetSecret after name and scope validation succeed.
func WithGetSecretErrorFunc(fn func(ctx context.Context, name string) error) Option {
	return func(cfg *fakeHostConfig) {
		cfg.getSecretErrFunc = fn
	}
}

// WithSetSecretErrorFunc injects an optional error callback evaluated on
// SetSecret after read-only, name, and scope validation succeed.
func WithSetSecretErrorFunc(fn func(ctx context.Context, name, value string) error) Option {
	return func(cfg *fakeHostConfig) {
		cfg.setSecretErrFunc = fn
	}
}

// WithDeleteSecretErrorFunc injects an optional error callback evaluated on
// DeleteSecret after read-only, name, and scope validation succeed.
func WithDeleteSecretErrorFunc(fn func(ctx context.Context, name string) error) Option {
	return func(cfg *fakeHostConfig) {
		cfg.deleteSecretErrFunc = fn
	}
}

// WithListSecretsErrorFunc injects an optional error callback evaluated on
// ListSecrets after pattern validation succeeds.
func WithListSecretsErrorFunc(fn func(ctx context.Context, pattern string) error) Option {
	return func(cfg *fakeHostConfig) {
		cfg.listSecretsErrFunc = fn
	}
}

// WithAuthIdentityFunc configures the callback invoked by GetAuthIdentity.
func WithAuthIdentityFunc(fn AuthIdentityFunc) Option {
	return func(cfg *fakeHostConfig) {
		cfg.authIdentityFunc = fn
	}
}

// WithAuthHandlersFunc configures the callback invoked by ListAuthHandlers.
func WithAuthHandlersFunc(fn AuthHandlersFunc) Option {
	return func(cfg *fakeHostConfig) {
		cfg.authHandlersFunc = fn
	}
}

// WithAuthTokenFunc configures the callback invoked by GetAuthToken.
func WithAuthTokenFunc(fn AuthTokenFunc) Option {
	return func(cfg *fakeHostConfig) {
		cfg.authTokenFunc = fn
	}
}

// WithAuthGroupsFunc configures the callback invoked by GetAuthGroups.
func WithAuthGroupsFunc(fn AuthGroupsFunc) Option {
	return func(cfg *fakeHostConfig) {
		cfg.authGroupsFunc = fn
	}
}

// WithPromptAuthResponseFunc configures the callback invoked by
// PromptAuthResponse. When nil (the default), FakeHostService returns gRPC
// Unimplemented, matching hosts that do not support paste-back prompts.
func WithPromptAuthResponseFunc(fn PromptAuthResponseFunc) Option {
	return func(cfg *fakeHostConfig) {
		cfg.promptAuthResponseFunc = fn
	}
}

// FakeHostService is a concurrency-safe in-memory implementation of
// proto.HostServiceServer that enforces the scafctl host secret-store contract.
type FakeHostService struct {
	proto.UnimplementedHostServiceServer

	mu                     sync.RWMutex
	secrets                map[string]string
	allowedSecretPrefix    string
	deniedSecretPrefixes   []string
	readOnlySecrets        bool
	getSecretErrFunc       func(ctx context.Context, name string) error
	setSecretErrFunc       func(ctx context.Context, name, value string) error
	deleteSecretErrFunc    func(ctx context.Context, name string) error
	listSecretsErrFunc     func(ctx context.Context, pattern string) error
	authIdentityFunc       AuthIdentityFunc
	authHandlersFunc       AuthHandlersFunc
	authTokenFunc          AuthTokenFunc
	authGroupsFunc         AuthGroupsFunc
	promptAuthResponseFunc PromptAuthResponseFunc
}

// NewFakeHostService constructs an unstarted FakeHostService with the given
// options. Returns an error if any initial secret name fails host RPC or
// backing-store secret-name validation.
func NewFakeHostService(opts ...Option) (*FakeHostService, error) {
	var cfg fakeHostConfig
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}

	secretsMap := make(map[string]string, len(cfg.initialSecrets))
	for name, val := range cfg.initialSecrets {
		if err := validateFakeSecretName(name); err != nil {
			return nil, fmt.Errorf("initial secret %q: %w", name, err)
		}
		secretsMap[name] = val
	}

	return &FakeHostService{
		secrets:                secretsMap,
		allowedSecretPrefix:    cfg.allowedSecretPrefix,
		deniedSecretPrefixes:   slices.Clone(cfg.deniedSecretPrefixes),
		readOnlySecrets:        cfg.readOnlySecrets,
		getSecretErrFunc:       cfg.getSecretErrFunc,
		setSecretErrFunc:       cfg.setSecretErrFunc,
		deleteSecretErrFunc:    cfg.deleteSecretErrFunc,
		listSecretsErrFunc:     cfg.listSecretsErrFunc,
		authIdentityFunc:       cfg.authIdentityFunc,
		authHandlersFunc:       cfg.authHandlersFunc,
		authTokenFunc:          cfg.authTokenFunc,
		authGroupsFunc:         cfg.authGroupsFunc,
		promptAuthResponseFunc: cfg.promptAuthResponseFunc,
	}, nil
}

// StartFakeHostServer starts a FakeHostService over an in-memory gRPC bufconn
// listener and returns a connected *plugin.HostServiceClient, the underlying
// *FakeHostService, and an idempotent cleanup function that closes the client
// connection and stops the gRPC server.
func StartFakeHostServer(opts ...Option) (*plugin.HostServiceClient, *FakeHostService, func(), error) {
	fake, err := NewFakeHostService(opts...)
	if err != nil {
		return nil, nil, nil, err
	}

	lis := bufconn.Listen(defaultBufconnSize)
	srv := grpc.NewServer()
	proto.RegisterHostServiceServer(srv, fake)

	serveDone := make(chan struct{})
	go func() {
		defer close(serveDone)
		_ = srv.Serve(lis)
	}()

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		srv.Stop()
		_ = lis.Close()
		<-serveDone
		return nil, nil, nil, fmt.Errorf("dialing bufconn host server: %w", err)
	}

	var closeOnce sync.Once
	cleanup := func() {
		closeOnce.Do(func() {
			_ = conn.Close()
			srv.Stop()
			_ = lis.Close()
			<-serveDone
		})
	}

	return plugin.NewHostServiceClient(conn), fake, cleanup, nil
}

// NewFakeHostClient starts an in-memory FakeHostService over bufconn, registers
// automatic shutdown via tb.Cleanup, and returns a connected
// *plugin.HostServiceClient alongside the *FakeHostService for test assertions.
func NewFakeHostClient(tb testing.TB, opts ...Option) (*plugin.HostServiceClient, *FakeHostService) {
	tb.Helper()
	client, fake, cleanup, err := StartFakeHostServer(opts...)
	if err != nil {
		tb.Fatalf("plugintest.NewFakeHostClient: %v", err)
	}
	tb.Cleanup(cleanup)
	return client, fake
}

// Secret returns the current stored value for name directly from the backing
// map, bypassing RPC prefix scoping for test assertions.
func (f *FakeHostService) Secret(name string) (string, bool) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	val, ok := f.secrets[name]
	return val, ok
}

// Secrets returns a snapshot copy of all secrets currently stored in f.
func (f *FakeHostService) Secrets() map[string]string {
	f.mu.RLock()
	defer f.mu.RUnlock()
	out := make(map[string]string, len(f.secrets))
	for k, v := range f.secrets {
		out[k] = v
	}
	return out
}

// PutSecret stores a secret directly in the backing map for test setup,
// validating name against host RPC and backing-store naming rules while
// bypassing ReadOnlySecrets and prefix scoping rules.
func (f *FakeHostService) PutSecret(name, value string) error {
	if err := validateFakeSecretName(name); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.secrets == nil {
		f.secrets = make(map[string]string)
	}
	f.secrets[name] = value
	return nil
}

// RemoveSecret deletes name directly from the backing map for test setup,
// bypassing ReadOnlySecrets and prefix scoping rules.
func (f *FakeHostService) RemoveSecret(name string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.secrets, name)
}

// SetGetSecretErrorFunc updates the GetSecret error injection callback.
func (f *FakeHostService) SetGetSecretErrorFunc(fn func(ctx context.Context, name string) error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.getSecretErrFunc = fn
}

// SetSetSecretErrorFunc updates the SetSecret error injection callback.
func (f *FakeHostService) SetSetSecretErrorFunc(fn func(ctx context.Context, name, value string) error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.setSecretErrFunc = fn
}

// SetDeleteSecretErrorFunc updates the DeleteSecret error injection callback.
func (f *FakeHostService) SetDeleteSecretErrorFunc(fn func(ctx context.Context, name string) error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleteSecretErrFunc = fn
}

// SetListSecretsErrorFunc updates the ListSecrets error injection callback.
func (f *FakeHostService) SetListSecretsErrorFunc(fn func(ctx context.Context, pattern string) error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.listSecretsErrFunc = fn
}

// SetAuthIdentityFunc updates the GetAuthIdentity callback.
func (f *FakeHostService) SetAuthIdentityFunc(fn AuthIdentityFunc) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.authIdentityFunc = fn
}

// SetAuthHandlersFunc updates the ListAuthHandlers callback.
func (f *FakeHostService) SetAuthHandlersFunc(fn AuthHandlersFunc) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.authHandlersFunc = fn
}

// SetAuthTokenFunc updates the GetAuthToken callback.
func (f *FakeHostService) SetAuthTokenFunc(fn AuthTokenFunc) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.authTokenFunc = fn
}

// SetAuthGroupsFunc updates the GetAuthGroups callback.
func (f *FakeHostService) SetAuthGroupsFunc(fn AuthGroupsFunc) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.authGroupsFunc = fn
}

// SetPromptAuthResponseFunc updates the PromptAuthResponse callback.
func (f *FakeHostService) SetPromptAuthResponseFunc(fn PromptAuthResponseFunc) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.promptAuthResponseFunc = fn
}

func validateFakeSecretName(name string) error {
	if err := plugin.ValidateHostSecretName(name); err != nil {
		return err
	}
	return validateStoreSecretName(name)
}

func validateStoreSecretName(name string) error {
	if name == "" {
		return fmt.Errorf("invalid secret name %q: name cannot be empty", name)
	}
	if len(name) > maxBackingStoreSecretNameLength {
		return fmt.Errorf("invalid secret name %q: name exceeds maximum length of %d characters", name, maxBackingStoreSecretNameLength)
	}
	if name[0] == '.' {
		return fmt.Errorf("invalid secret name %q: name cannot start with '.'", name)
	}
	if name[0] == '-' {
		return fmt.Errorf("invalid secret name %q: name cannot start with '-'", name)
	}
	if strings.Contains(name, "..") {
		return fmt.Errorf("invalid secret name %q: name cannot contain '..'", name)
	}
	if !isValidStoreSecretNameChars(name) {
		return fmt.Errorf("invalid secret name %q: name contains invalid characters (allowed: a-z, A-Z, 0-9, -, _, .)", name)
	}
	return nil
}

func isValidStoreSecretNameChars(name string) bool {
	if len(name) == 0 || !isASCIIAlnum(name[0]) {
		return false
	}
	for i := 1; i < len(name); i++ {
		b := name[i]
		if !isASCIIAlnum(b) && b != '.' && b != '_' && b != '-' {
			return false
		}
	}
	return true
}

func isASCIIAlnum(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

func (f *FakeHostService) authorizeSecretLocked(name string, write bool) string {
	if write && f.readOnlySecrets {
		return msgReadOnlyStore
	}
	if err := plugin.ValidateHostSecretName(name); err != nil {
		return err.Error()
	}
	if f.allowedSecretPrefix != "" && !strings.HasPrefix(name, f.allowedSecretPrefix) {
		return msgOutsideAllowedScope
	}
	if f.isSecretDeniedLocked(name) {
		return msgDeniedNamespace
	}
	return ""
}

func (f *FakeHostService) isSecretDeniedLocked(name string) bool {
	for _, prefix := range f.deniedSecretPrefixes {
		if prefix != "" && strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func toGRPCStatusError(err error) error {
	if s, ok := status.FromError(err); ok {
		return s.Err()
	}
	if errors.Is(err, context.Canceled) {
		return status.Error(codes.Canceled, err.Error())
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return status.Error(codes.DeadlineExceeded, err.Error())
	}
	return nil
}

// GetSecret implements proto.HostServiceServer.
func (f *FakeHostService) GetSecret(ctx context.Context, req *proto.GetSecretRequest) (*proto.GetSecretResponse, error) {
	name := req.GetName()

	f.mu.RLock()
	errMsg := f.authorizeSecretLocked(name, false)
	errFn := f.getSecretErrFunc
	f.mu.RUnlock()

	if errMsg != "" {
		return &proto.GetSecretResponse{Error: errMsg}, nil
	}
	if err := validateStoreSecretName(name); err != nil {
		return &proto.GetSecretResponse{Error: fmt.Sprintf("get secret %q: %v", name, err)}, nil
	}
	if errFn != nil {
		if err := errFn(ctx, name); err != nil {
			if statusErr := toGRPCStatusError(err); statusErr != nil {
				return nil, statusErr
			}
			return &proto.GetSecretResponse{Error: fmt.Sprintf("get secret %q: %v", name, err)}, nil
		}
	}

	f.mu.RLock()
	val, ok := f.secrets[name]
	f.mu.RUnlock()
	if !ok {
		return &proto.GetSecretResponse{Found: false}, nil
	}
	return &proto.GetSecretResponse{Value: val, Found: true}, nil
}

// SetSecret implements proto.HostServiceServer.
func (f *FakeHostService) SetSecret(ctx context.Context, req *proto.SetSecretRequest) (*proto.SetSecretResponse, error) {
	name := req.GetName()
	val := req.GetValue()

	f.mu.RLock()
	errMsg := f.authorizeSecretLocked(name, true)
	errFn := f.setSecretErrFunc
	f.mu.RUnlock()

	if errMsg != "" {
		return &proto.SetSecretResponse{Error: errMsg}, nil
	}
	if err := validateStoreSecretName(name); err != nil {
		return &proto.SetSecretResponse{Error: fmt.Sprintf("set secret %q: %v", name, err)}, nil
	}
	if errFn != nil {
		if err := errFn(ctx, name, val); err != nil {
			if statusErr := toGRPCStatusError(err); statusErr != nil {
				return nil, statusErr
			}
			return &proto.SetSecretResponse{Error: fmt.Sprintf("set secret %q: %v", name, err)}, nil
		}
	}

	f.mu.Lock()
	if f.secrets == nil {
		f.secrets = make(map[string]string)
	}
	f.secrets[name] = val
	f.mu.Unlock()
	return &proto.SetSecretResponse{}, nil
}

// DeleteSecret implements proto.HostServiceServer.
func (f *FakeHostService) DeleteSecret(ctx context.Context, req *proto.DeleteSecretRequest) (*proto.DeleteSecretResponse, error) {
	name := req.GetName()

	f.mu.RLock()
	errMsg := f.authorizeSecretLocked(name, true)
	errFn := f.deleteSecretErrFunc
	f.mu.RUnlock()

	if errMsg != "" {
		return &proto.DeleteSecretResponse{Error: errMsg}, nil
	}
	if err := validateStoreSecretName(name); err != nil {
		return &proto.DeleteSecretResponse{Error: fmt.Sprintf("delete secret %q: %v", name, err)}, nil
	}
	if errFn != nil {
		if err := errFn(ctx, name); err != nil {
			if statusErr := toGRPCStatusError(err); statusErr != nil {
				return nil, statusErr
			}
			return &proto.DeleteSecretResponse{Error: fmt.Sprintf("delete secret %q: %v", name, err)}, nil
		}
	}

	f.mu.Lock()
	delete(f.secrets, name)
	f.mu.Unlock()
	return &proto.DeleteSecretResponse{}, nil
}

// ListSecrets implements proto.HostServiceServer.
func (f *FakeHostService) ListSecrets(ctx context.Context, req *proto.ListSecretsRequest) (*proto.ListSecretsResponse, error) {
	pattern := req.GetPattern()

	var re *regexp.Regexp
	if pattern != "" {
		if len(pattern) > plugin.MaxHostSecretPatternLength {
			return &proto.ListSecretsResponse{
				Error: fmt.Sprintf("pattern too long: max %d characters", plugin.MaxHostSecretPatternLength),
			}, nil
		}
		var err error
		re, err = regexp.Compile(pattern)
		if err != nil {
			return &proto.ListSecretsResponse{
				Error: fmt.Sprintf("invalid pattern: %v", err),
			}, nil
		}
	}

	f.mu.RLock()
	errFn := f.listSecretsErrFunc
	f.mu.RUnlock()

	if errFn != nil {
		if err := errFn(ctx, pattern); err != nil {
			if statusErr := toGRPCStatusError(err); statusErr != nil {
				return nil, statusErr
			}
			return &proto.ListSecretsResponse{Error: fmt.Sprintf("list secrets: %v", err)}, nil
		}
	}

	f.mu.RLock()
	names := make([]string, 0, len(f.secrets))
	for name := range f.secrets {
		if f.allowedSecretPrefix != "" && !strings.HasPrefix(name, f.allowedSecretPrefix) {
			continue
		}
		if f.isSecretDeniedLocked(name) {
			continue
		}
		names = append(names, name)
	}
	f.mu.RUnlock()

	slices.Sort(names)

	if re != nil {
		filtered := make([]string, 0, len(names))
		for _, name := range names {
			if re.MatchString(name) {
				filtered = append(filtered, name)
			}
		}
		names = filtered
	}

	return &proto.ListSecretsResponse{Names: names}, nil
}

// GetAuthIdentity implements proto.HostServiceServer.
func (f *FakeHostService) GetAuthIdentity(ctx context.Context, req *proto.GetAuthIdentityRequest) (*proto.GetAuthIdentityResponse, error) {
	f.mu.RLock()
	fn := f.authIdentityFunc
	f.mu.RUnlock()

	if fn == nil {
		return &proto.GetAuthIdentityResponse{Error: msgIdentityUnavailable}, nil
	}

	reqCopy := &proto.GetAuthIdentityRequest{
		HandlerName: req.GetHandlerName(),
		Scope:       req.GetScope(),
	}
	claims, err := fn(ctx, reqCopy)
	if err != nil {
		if statusErr := toGRPCStatusError(err); statusErr != nil {
			return nil, statusErr
		}
		return &proto.GetAuthIdentityResponse{Error: fmt.Sprintf("get auth identity: %v", err)}, nil
	}
	return &proto.GetAuthIdentityResponse{Claims: cloneClaims(claims)}, nil
}

// ListAuthHandlers implements proto.HostServiceServer.
func (f *FakeHostService) ListAuthHandlers(ctx context.Context, _ *proto.ListAuthHandlersRequest) (*proto.ListAuthHandlersResponse, error) {
	f.mu.RLock()
	fn := f.authHandlersFunc
	f.mu.RUnlock()

	if fn == nil {
		return &proto.ListAuthHandlersResponse{}, nil
	}

	handlers, defaultHandler, err := fn(ctx)
	if err != nil {
		if statusErr := toGRPCStatusError(err); statusErr != nil {
			return nil, statusErr
		}
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &proto.ListAuthHandlersResponse{
		HandlerNames:   slices.Clone(handlers),
		DefaultHandler: defaultHandler,
	}, nil
}

// GetAuthToken implements proto.HostServiceServer.
func (f *FakeHostService) GetAuthToken(ctx context.Context, req *proto.GetAuthTokenRequest) (*proto.GetAuthTokenResponse, error) {
	f.mu.RLock()
	fn := f.authTokenFunc
	f.mu.RUnlock()

	if fn == nil {
		return &proto.GetAuthTokenResponse{Error: msgAuthSvcUnavailable}, nil
	}

	reqCopy := &proto.GetAuthTokenRequest{
		HandlerName:        req.GetHandlerName(),
		Scope:              req.GetScope(),
		MinValidForSeconds: req.GetMinValidForSeconds(),
		ForceRefresh:       req.GetForceRefresh(),
		Profile:            req.GetProfile(),
		Hostname:           req.GetHostname(),
	}
	resp, err := fn(ctx, reqCopy)
	if err != nil {
		if statusErr := toGRPCStatusError(err); statusErr != nil {
			return nil, statusErr
		}
		return &proto.GetAuthTokenResponse{Error: fmt.Sprintf("get auth token: %v", err)}, nil
	}
	if resp == nil {
		return &proto.GetAuthTokenResponse{}, nil
	}
	return &proto.GetAuthTokenResponse{
		AccessToken:   resp.GetAccessToken(),
		TokenType:     resp.GetTokenType(),
		ExpiresAtUnix: resp.GetExpiresAtUnix(),
		Scope:         resp.GetScope(),
		Error:         resp.GetError(),
	}, nil
}

// GetAuthGroups implements proto.HostServiceServer.
func (f *FakeHostService) GetAuthGroups(ctx context.Context, req *proto.GetAuthGroupsRequest) (*proto.GetAuthGroupsResponse, error) {
	f.mu.RLock()
	fn := f.authGroupsFunc
	f.mu.RUnlock()

	if fn == nil {
		return &proto.GetAuthGroupsResponse{Error: msgGroupsUnavailable}, nil
	}

	reqCopy := &proto.GetAuthGroupsRequest{
		HandlerName: req.GetHandlerName(),
		Profile:     req.GetProfile(),
	}
	groups, err := fn(ctx, reqCopy)
	if err != nil {
		if statusErr := toGRPCStatusError(err); statusErr != nil {
			return nil, statusErr
		}
		return &proto.GetAuthGroupsResponse{Error: fmt.Sprintf("get auth groups: %v", err)}, nil
	}
	return &proto.GetAuthGroupsResponse{Groups: slices.Clone(groups)}, nil
}

// PromptAuthResponse implements proto.HostServiceServer.
func (f *FakeHostService) PromptAuthResponse(ctx context.Context, req *proto.PromptAuthResponseRequest) (*proto.PromptAuthResponseResponse, error) {
	f.mu.RLock()
	fn := f.promptAuthResponseFunc
	f.mu.RUnlock()

	if fn == nil {
		return nil, status.Error(codes.Unimplemented, msgPromptUnimplemented)
	}

	reqCopy := &proto.PromptAuthResponseRequest{
		HandlerName:      req.GetHandlerName(),
		AuthorizationUrl: req.GetAuthorizationUrl(),
		RedirectUri:      req.GetRedirectUri(),
	}
	val, err := fn(ctx, reqCopy)
	if err != nil {
		if statusErr := toGRPCStatusError(err); statusErr != nil {
			return nil, statusErr
		}
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &proto.PromptAuthResponseResponse{Value: val}, nil
}

func cloneClaims(c *proto.Claims) *proto.Claims {
	if c == nil {
		return nil
	}
	return &proto.Claims{
		Issuer:        c.GetIssuer(),
		Subject:       c.GetSubject(),
		TenantId:      c.GetTenantId(),
		ObjectId:      c.GetObjectId(),
		ClientId:      c.GetClientId(),
		Email:         c.GetEmail(),
		Name:          c.GetName(),
		Username:      c.GetUsername(),
		IssuedAtUnix:  c.GetIssuedAtUnix(),
		ExpiresAtUnix: c.GetExpiresAtUnix(),
	}
}
