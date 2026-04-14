// Copyright 2025-2026 Oakwood Commons
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"testing"

	"github.com/oakwood-commons/scafctl-plugin-sdk/auth"
	"github.com/oakwood-commons/scafctl-plugin-sdk/plugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newStaticAuth() *StaticAuthPlugin { return &StaticAuthPlugin{} }

func TestGetAuthHandlers(t *testing.T) {
	handlers, err := newStaticAuth().GetAuthHandlers(context.Background())
	require.NoError(t, err)
	require.Len(t, handlers, 1)
	assert.Equal(t, "static", handlers[0].Name)
	assert.Equal(t, "Static Auth Handler", handlers[0].DisplayName)
	assert.Contains(t, handlers[0].Flows, auth.FlowPAT)
	assert.Contains(t, handlers[0].Capabilities, auth.CapScopesOnLogin)
	assert.Contains(t, handlers[0].Capabilities, auth.CapFlowOverride)
}

func TestConfigureAuthHandler(t *testing.T) {
	tests := []struct {
		name    string
		handler string
		wantErr string
	}{
		{
			name:    "valid handler",
			handler: "static",
		},
		{
			name:    "unknown handler",
			handler: "nope",
			wantErr: "unknown handler: nope",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := newStaticAuth().ConfigureAuthHandler(context.Background(), tc.handler, plugin.ProviderConfig{})
			if tc.wantErr != "" {
				require.EqualError(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestLogin(t *testing.T) {
	tests := []struct {
		name    string
		handler string
		wantErr string
	}{
		{
			name:    "successful login",
			handler: "static",
		},
		{
			name:    "unknown handler",
			handler: "nope",
			wantErr: "unknown handler: nope",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := newStaticAuth()
			resp, err := p.Login(context.Background(), tc.handler, plugin.LoginRequest{}, nil)
			if tc.wantErr != "" {
				require.EqualError(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, "static-user", resp.Claims.Subject)
			assert.Equal(t, "user@example.com", resp.Claims.Email)
			assert.False(t, resp.ExpiresAt.IsZero())
			assert.True(t, p.loggedIn)
		})
	}
}

func TestLogout(t *testing.T) {
	tests := []struct {
		name    string
		handler string
		wantErr string
	}{
		{
			name:    "successful logout",
			handler: "static",
		},
		{
			name:    "unknown handler",
			handler: "nope",
			wantErr: "unknown handler: nope",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := newStaticAuth()
			// Login first, then logout.
			_, _ = p.Login(context.Background(), "static", plugin.LoginRequest{}, nil)
			err := p.Logout(context.Background(), tc.handler)
			if tc.wantErr != "" {
				require.EqualError(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.False(t, p.loggedIn)
			assert.Nil(t, p.claims)
		})
	}
}

func TestGetStatus(t *testing.T) {
	tests := []struct {
		name       string
		handler    string
		login      bool
		wantErr    string
		wantAuthed bool
	}{
		{
			name:       "not logged in",
			handler:    "static",
			wantAuthed: false,
		},
		{
			name:       "logged in",
			handler:    "static",
			login:      true,
			wantAuthed: true,
		},
		{
			name:    "unknown handler",
			handler: "nope",
			wantErr: "unknown handler: nope",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := newStaticAuth()
			if tc.login {
				_, _ = p.Login(context.Background(), "static", plugin.LoginRequest{}, nil)
			}
			status, err := p.GetStatus(context.Background(), tc.handler)
			if tc.wantErr != "" {
				require.EqualError(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantAuthed, status.Authenticated)
			assert.Equal(t, auth.IdentityTypeUser, status.IdentityType)
		})
	}
}

func TestGetToken(t *testing.T) {
	tests := []struct {
		name    string
		handler string
		login   bool
		wantErr string
	}{
		{
			name:    "not logged in",
			handler: "static",
			wantErr: "not logged in",
		},
		{
			name:    "logged in",
			handler: "static",
			login:   true,
		},
		{
			name:    "unknown handler",
			handler: "nope",
			wantErr: "unknown handler: nope",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := newStaticAuth()
			if tc.login {
				_, _ = p.Login(context.Background(), "static", plugin.LoginRequest{}, nil)
			}
			resp, err := p.GetToken(context.Background(), tc.handler, plugin.TokenRequest{})
			if tc.wantErr != "" {
				require.EqualError(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, "static-token-value", resp.AccessToken)
			assert.Equal(t, "Bearer", resp.TokenType)
			assert.False(t, resp.ExpiresAt.IsZero())
		})
	}
}

func TestListCachedTokens(t *testing.T) {
	tokens, err := newStaticAuth().ListCachedTokens(context.Background(), "static")
	require.NoError(t, err)
	assert.Nil(t, tokens)
}

func TestPurgeExpiredTokens(t *testing.T) {
	n, err := newStaticAuth().PurgeExpiredTokens(context.Background(), "static")
	require.NoError(t, err)
	assert.Equal(t, 0, n)
}

func TestStopAuthHandler(t *testing.T) {
	err := newStaticAuth().StopAuthHandler(context.Background(), "static")
	require.NoError(t, err)
}
