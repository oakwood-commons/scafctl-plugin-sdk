// Copyright 2025-2026 Oakwood Commons
// SPDX-License-Identifier: Apache-2.0

// Command multicluster is an example auth handler that maintains a separate
// credential per instance (cluster/server), keyed by hostname. It demonstrates
// the per-instance capabilities and request fields:
//
//   - auth.CapHostname        -- Login honors LoginRequest.Hostname
//   - auth.CapTokenHostname   -- GetToken honors TokenRequest.Hostname
//   - auth.CapInstanceHostname -- GetStatus/Logout honor their request Hostname
//   - auth.CapCallbackPort    -- Login reads LoginRequest.CallbackPort
//
// Each hostname selects an independent session, so a single handler can serve
// many clusters (e.g. as a kubectl/oc exec credential plugin) without one
// cluster's token leaking into another's requests.
package main

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/go-logr/logr"

	"github.com/oakwood-commons/scafctl-plugin-sdk/auth"
	"github.com/oakwood-commons/scafctl-plugin-sdk/plugin"
)

const handlerName = "multicluster"

// session is the per-instance credential state.
type session struct {
	claims       *auth.Claims
	token        string
	expiresAt    time.Time
	callbackPort int
}

// MultiClusterAuthPlugin implements an auth handler that stores one session per
// instance hostname. Access is guarded by mu because go-plugin may dispatch
// handler calls concurrently.
type MultiClusterAuthPlugin struct {
	mu       sync.Mutex
	sessions map[string]*session
}

// New returns a ready-to-serve MultiClusterAuthPlugin.
func New() *MultiClusterAuthPlugin {
	return &MultiClusterAuthPlugin{sessions: make(map[string]*session)}
}

func (p *MultiClusterAuthPlugin) GetAuthHandlers(_ context.Context) ([]plugin.AuthHandlerInfo, error) {
	return []plugin.AuthHandlerInfo{
		{
			Name:        handlerName,
			DisplayName: "Multi-Cluster Auth Handler",
			Flows:       []auth.Flow{auth.FlowInteractive},
			Capabilities: []auth.Capability{
				auth.CapHostname,
				auth.CapTokenHostname,
				auth.CapInstanceHostname,
				auth.CapCallbackPort,
			},
		},
	}, nil
}

func (p *MultiClusterAuthPlugin) ConfigureAuthHandler(_ context.Context, name string, _ plugin.ProviderConfig) error {
	return checkHandler(name)
}

// Login authenticates against the instance named by req.Hostname and records a
// session keyed by that hostname. req.CallbackPort is the loopback port the host
// requested for the OAuth callback; a real handler would bind its callback
// server to it. An empty hostname addresses the default instance.
func (p *MultiClusterAuthPlugin) Login(ctx context.Context, name string, req plugin.LoginRequest, _ func(plugin.DeviceCodePrompt)) (*plugin.LoginResponse, error) {
	if err := checkHandler(name); err != nil {
		return nil, err
	}
	logr.FromContextOrDiscard(ctx).V(1).Info("login",
		"hostname", req.Hostname, "callbackPort", req.CallbackPort)

	now := time.Now()
	claims := &auth.Claims{
		Subject:  "user@" + instanceLabel(req.Hostname),
		Name:     "Cluster User",
		IssuedAt: now,
	}
	expiresAt := now.Add(time.Hour)

	p.mu.Lock()
	p.sessions[req.Hostname] = &session{
		claims:       claims,
		token:        "token-for-" + instanceLabel(req.Hostname),
		expiresAt:    expiresAt,
		callbackPort: req.CallbackPort,
	}
	p.mu.Unlock()

	return &plugin.LoginResponse{Claims: claims, ExpiresAt: expiresAt}, nil
}

// Logout clears the session for req.Hostname only. When Hostname is empty, all
// instances are cleared, preserving the pre-CapInstanceHostname behavior.
func (p *MultiClusterAuthPlugin) Logout(_ context.Context, name string, req plugin.LogoutRequest) error {
	if err := checkHandler(name); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if req.Hostname == "" {
		p.sessions = make(map[string]*session)
		return nil
	}
	delete(p.sessions, req.Hostname)
	return nil
}

// GetStatus reports the authentication state of the instance named by
// req.Hostname.
func (p *MultiClusterAuthPlugin) GetStatus(_ context.Context, name string, req plugin.StatusRequest) (*auth.Status, error) {
	if err := checkHandler(name); err != nil {
		return nil, err
	}
	p.mu.Lock()
	sess := p.sessions[req.Hostname]
	p.mu.Unlock()
	if sess == nil {
		return &auth.Status{Authenticated: false, Reason: "no session for instance", Hostname: req.Hostname}, nil
	}
	return &auth.Status{
		Authenticated: true,
		Claims:        sess.claims,
		ExpiresAt:     sess.expiresAt,
		IdentityType:  auth.IdentityTypeUser,
		Hostname:      req.Hostname,
	}, nil
}

// GetToken returns the access token for the instance named by req.Hostname.
func (p *MultiClusterAuthPlugin) GetToken(_ context.Context, name string, req plugin.TokenRequest) (*plugin.TokenResponse, error) {
	if err := checkHandler(name); err != nil {
		return nil, err
	}
	p.mu.Lock()
	sess := p.sessions[req.Hostname]
	p.mu.Unlock()
	if sess == nil {
		return nil, fmt.Errorf("not logged in for instance %q", req.Hostname)
	}
	return &plugin.TokenResponse{
		AccessToken: sess.token,
		TokenType:   "Bearer",
		ExpiresAt:   sess.expiresAt,
	}, nil
}

func (p *MultiClusterAuthPlugin) ListCachedTokens(_ context.Context, name string) ([]*auth.CachedTokenInfo, error) {
	if err := checkHandler(name); err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	hosts := make([]string, 0, len(p.sessions))
	for host := range p.sessions {
		hosts = append(hosts, host)
	}
	sort.Strings(hosts) // deterministic listing order
	tokens := make([]*auth.CachedTokenInfo, 0, len(hosts))
	for _, host := range hosts {
		sess := p.sessions[host]
		tokens = append(tokens, &auth.CachedTokenInfo{
			// Handler is the auth handler name; Hostname carries the per-instance
			// identifier for handlers advertising auth.CapInstanceHostname, so the
			// host can enumerate one entry per live cluster.
			Handler:   handlerName,
			Hostname:  host,
			TokenKind: "access",
			TokenType: "Bearer",
			IsExpired: time.Now().After(sess.expiresAt),
		})
	}
	return tokens, nil
}

func (p *MultiClusterAuthPlugin) PurgeExpiredTokens(_ context.Context, name string) (int, error) {
	if err := checkHandler(name); err != nil {
		return 0, err
	}
	now := time.Now()
	p.mu.Lock()
	defer p.mu.Unlock()
	purged := 0
	for host, sess := range p.sessions {
		if now.After(sess.expiresAt) {
			delete(p.sessions, host)
			purged++
		}
	}
	return purged, nil
}

func (p *MultiClusterAuthPlugin) DetectAvailableFlows(_ context.Context, name string) ([]plugin.FlowAvailability, error) {
	if err := checkHandler(name); err != nil {
		return nil, err
	}
	return []plugin.FlowAvailability{
		{Flow: auth.FlowInteractive, Available: true, Reason: "interactive login always available"},
	}, nil
}

func (p *MultiClusterAuthPlugin) StopAuthHandler(_ context.Context, name string) error {
	return checkHandler(name)
}

func checkHandler(name string) error {
	if name != handlerName {
		return fmt.Errorf("unknown handler: %s", name)
	}
	return nil
}

// instanceLabel returns a display label for an instance hostname, mapping the
// empty (default) instance to a readable name.
func instanceLabel(hostname string) string {
	if hostname == "" {
		return "default"
	}
	return hostname
}

func main() {
	plugin.ServeAuthHandler(New())
}
