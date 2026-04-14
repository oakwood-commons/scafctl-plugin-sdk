// Copyright 2025-2026 Oakwood Commons
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"time"

	"github.com/oakwood-commons/scafctl-plugin-sdk/auth"
	"github.com/oakwood-commons/scafctl-plugin-sdk/plugin"
)

const handlerName = "static"

// StaticAuthPlugin implements a minimal auth handler that returns static tokens.
// It demonstrates how to implement the AuthHandlerPlugin interface.
type StaticAuthPlugin struct {
	loggedIn bool
	claims   *auth.Claims
}

func (p *StaticAuthPlugin) GetAuthHandlers(_ context.Context) ([]plugin.AuthHandlerInfo, error) {
	return []plugin.AuthHandlerInfo{
		{
			Name:         handlerName,
			DisplayName:  "Static Auth Handler",
			Flows:        []auth.Flow{auth.FlowPAT},
			Capabilities: []auth.Capability{auth.CapScopesOnLogin, auth.CapFlowOverride},
		},
	}, nil
}

func (p *StaticAuthPlugin) ConfigureAuthHandler(_ context.Context, name string, _ plugin.ProviderConfig) error {
	if name != handlerName {
		return fmt.Errorf("unknown handler: %s", name)
	}
	return nil
}

func (p *StaticAuthPlugin) Login(_ context.Context, name string, _ plugin.LoginRequest, _ func(plugin.DeviceCodePrompt)) (*plugin.LoginResponse, error) {
	if name != handlerName {
		return nil, fmt.Errorf("unknown handler: %s", name)
	}
	now := time.Now()
	p.claims = &auth.Claims{
		Subject:  "static-user",
		Email:    "user@example.com",
		Name:     "Static User",
		IssuedAt: now,
	}
	p.loggedIn = true
	return &plugin.LoginResponse{
		Claims:    p.claims,
		ExpiresAt: now.Add(time.Hour),
	}, nil
}

func (p *StaticAuthPlugin) Logout(_ context.Context, name string) error {
	if name != handlerName {
		return fmt.Errorf("unknown handler: %s", name)
	}
	p.loggedIn = false
	p.claims = nil
	return nil
}

func (p *StaticAuthPlugin) GetStatus(_ context.Context, name string) (*auth.Status, error) {
	if name != handlerName {
		return nil, fmt.Errorf("unknown handler: %s", name)
	}
	return &auth.Status{
		Authenticated: p.loggedIn,
		Claims:        p.claims,
		IdentityType:  auth.IdentityTypeUser,
	}, nil
}

func (p *StaticAuthPlugin) GetToken(_ context.Context, name string, _ plugin.TokenRequest) (*plugin.TokenResponse, error) {
	if name != handlerName {
		return nil, fmt.Errorf("unknown handler: %s", name)
	}
	if !p.loggedIn {
		return nil, fmt.Errorf("not logged in")
	}
	return &plugin.TokenResponse{
		AccessToken: "static-token-value",
		TokenType:   "Bearer",
		ExpiresAt:   time.Now().Add(time.Hour),
	}, nil
}

func (p *StaticAuthPlugin) ListCachedTokens(_ context.Context, name string) ([]*auth.CachedTokenInfo, error) {
	if name != handlerName {
		return nil, fmt.Errorf("unknown handler: %s", name)
	}
	return nil, nil
}

func (p *StaticAuthPlugin) PurgeExpiredTokens(_ context.Context, name string) (int, error) {
	if name != handlerName {
		return 0, fmt.Errorf("unknown handler: %s", name)
	}
	return 0, nil
}

func (p *StaticAuthPlugin) StopAuthHandler(_ context.Context, name string) error {
	if name != handlerName {
		return fmt.Errorf("unknown handler: %s", name)
	}
	return nil
}

func main() {
	plugin.ServeAuthHandler(&StaticAuthPlugin{})
}
