// Copyright 2025-2026 Oakwood Commons
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oakwood-commons/scafctl-plugin-sdk/auth"
	"github.com/oakwood-commons/scafctl-plugin-sdk/plugin"
)

func TestGetAuthHandlers_AdvertisesInstanceCapabilities(t *testing.T) {
	handlers, err := New().GetAuthHandlers(context.Background())
	require.NoError(t, err)
	require.Len(t, handlers, 1)
	assert.Equal(t, handlerName, handlers[0].Name)
	assert.Contains(t, handlers[0].Capabilities, auth.CapHostname)
	assert.Contains(t, handlers[0].Capabilities, auth.CapTokenHostname)
	assert.Contains(t, handlers[0].Capabilities, auth.CapInstanceHostname)
	assert.Contains(t, handlers[0].Capabilities, auth.CapCallbackPort)
}

func TestLogin_RecordsCallbackPortPerInstance(t *testing.T) {
	p := New()
	_, err := p.Login(context.Background(), handlerName, plugin.LoginRequest{
		Hostname:     "cluster-a",
		CallbackPort: 18080,
	}, nil)
	require.NoError(t, err)

	p.mu.Lock()
	sess := p.sessions["cluster-a"]
	p.mu.Unlock()
	require.NotNil(t, sess)
	assert.Equal(t, 18080, sess.callbackPort)
}

func TestGetToken_IsScopedByHostname(t *testing.T) {
	p := New()
	ctx := context.Background()
	_, err := p.Login(ctx, handlerName, plugin.LoginRequest{Hostname: "cluster-a"}, nil)
	require.NoError(t, err)
	_, err = p.Login(ctx, handlerName, plugin.LoginRequest{Hostname: "cluster-b"}, nil)
	require.NoError(t, err)

	tokA, err := p.GetToken(ctx, handlerName, plugin.TokenRequest{Hostname: "cluster-a"})
	require.NoError(t, err)
	tokB, err := p.GetToken(ctx, handlerName, plugin.TokenRequest{Hostname: "cluster-b"})
	require.NoError(t, err)

	assert.Equal(t, "token-for-cluster-a", tokA.AccessToken)
	assert.Equal(t, "token-for-cluster-b", tokB.AccessToken)
	assert.NotEqual(t, tokA.AccessToken, tokB.AccessToken)

	// An instance that was never logged in has no token.
	_, err = p.GetToken(ctx, handlerName, plugin.TokenRequest{Hostname: "cluster-c"})
	require.Error(t, err)
}

func TestGetStatus_IsScopedByHostname(t *testing.T) {
	p := New()
	ctx := context.Background()
	_, err := p.Login(ctx, handlerName, plugin.LoginRequest{Hostname: "cluster-a"}, nil)
	require.NoError(t, err)

	authed, err := p.GetStatus(ctx, handlerName, plugin.StatusRequest{Hostname: "cluster-a"})
	require.NoError(t, err)
	assert.True(t, authed.Authenticated)
	assert.Equal(t, "user@cluster-a", authed.Claims.Subject)
	assert.Equal(t, "cluster-a", authed.Hostname)

	unauthed, err := p.GetStatus(ctx, handlerName, plugin.StatusRequest{Hostname: "cluster-b"})
	require.NoError(t, err)
	assert.False(t, unauthed.Authenticated)
	assert.Equal(t, "cluster-b", unauthed.Hostname)
}

func TestLogout_ClearsOnlySelectedInstance(t *testing.T) {
	p := New()
	ctx := context.Background()
	_, err := p.Login(ctx, handlerName, plugin.LoginRequest{Hostname: "cluster-a"}, nil)
	require.NoError(t, err)
	_, err = p.Login(ctx, handlerName, plugin.LoginRequest{Hostname: "cluster-b"}, nil)
	require.NoError(t, err)

	require.NoError(t, p.Logout(ctx, handlerName, plugin.LogoutRequest{Hostname: "cluster-a"}))

	statusA, err := p.GetStatus(ctx, handlerName, plugin.StatusRequest{Hostname: "cluster-a"})
	require.NoError(t, err)
	assert.False(t, statusA.Authenticated, "cluster-a should be cleared")

	statusB, err := p.GetStatus(ctx, handlerName, plugin.StatusRequest{Hostname: "cluster-b"})
	require.NoError(t, err)
	assert.True(t, statusB.Authenticated, "cluster-b should be untouched")
}

func TestLogout_EmptyHostnameClearsAll(t *testing.T) {
	p := New()
	ctx := context.Background()
	_, err := p.Login(ctx, handlerName, plugin.LoginRequest{Hostname: "cluster-a"}, nil)
	require.NoError(t, err)
	_, err = p.Login(ctx, handlerName, plugin.LoginRequest{Hostname: "cluster-b"}, nil)
	require.NoError(t, err)

	require.NoError(t, p.Logout(ctx, handlerName, plugin.LogoutRequest{}))

	p.mu.Lock()
	remaining := len(p.sessions)
	p.mu.Unlock()
	assert.Equal(t, 0, remaining)
}

func TestUnknownHandlerRejected(t *testing.T) {
	p := New()
	ctx := context.Background()
	_, err := p.GetStatus(ctx, "nope", plugin.StatusRequest{})
	require.EqualError(t, err, "unknown handler: nope")
	err = p.Logout(ctx, "nope", plugin.LogoutRequest{})
	require.EqualError(t, err, "unknown handler: nope")
}

func TestListAndPurge(t *testing.T) {
	p := New()
	ctx := context.Background()
	_, err := p.Login(ctx, handlerName, plugin.LoginRequest{Hostname: "cluster-a"}, nil)
	require.NoError(t, err)
	_, err = p.Login(ctx, handlerName, plugin.LoginRequest{Hostname: "cluster-b"}, nil)
	require.NoError(t, err)

	tokens, err := p.ListCachedTokens(ctx, handlerName)
	require.NoError(t, err)
	// One entry per live instance, keyed by Hostname (deterministic order).
	require.Len(t, tokens, 2)
	assert.Equal(t, handlerName, tokens[0].Handler)
	assert.Equal(t, "cluster-a", tokens[0].Hostname)
	assert.Equal(t, "cluster-b", tokens[1].Hostname)

	// Nothing is expired yet.
	purged, err := p.PurgeExpiredTokens(ctx, handlerName)
	require.NoError(t, err)
	assert.Equal(t, 0, purged)
}

func TestPurgeExpiredTokens_RemovesExpired(t *testing.T) {
	p := New()
	ctx := context.Background()
	_, err := p.Login(ctx, handlerName, plugin.LoginRequest{Hostname: "cluster-a"}, nil)
	require.NoError(t, err)

	// Force the session to be expired.
	p.mu.Lock()
	p.sessions["cluster-a"].expiresAt = time.Now().Add(-time.Minute)
	p.mu.Unlock()

	purged, err := p.PurgeExpiredTokens(ctx, handlerName)
	require.NoError(t, err)
	assert.Equal(t, 1, purged)

	status, err := p.GetStatus(ctx, handlerName, plugin.StatusRequest{Hostname: "cluster-a"})
	require.NoError(t, err)
	assert.False(t, status.Authenticated)
}

func TestConfigureDetectAndStop(t *testing.T) {
	p := New()
	ctx := context.Background()

	require.NoError(t, p.ConfigureAuthHandler(ctx, handlerName, plugin.ProviderConfig{}))
	require.Error(t, p.ConfigureAuthHandler(ctx, "nope", plugin.ProviderConfig{}))

	flows, err := p.DetectAvailableFlows(ctx, handlerName)
	require.NoError(t, err)
	require.Len(t, flows, 1)
	assert.Equal(t, auth.FlowInteractive, flows[0].Flow)
	assert.True(t, flows[0].Available)

	require.NoError(t, p.StopAuthHandler(ctx, handlerName))
	require.Error(t, p.StopAuthHandler(ctx, "nope"))
}
