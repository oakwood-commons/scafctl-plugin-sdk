// Copyright 2025-2026 Oakwood Commons
// SPDX-License-Identifier: Apache-2.0

package plugin

import (
	"context"

	goplugin "github.com/hashicorp/go-plugin"
	"github.com/oakwood-commons/scafctl-plugin-sdk/plugin/proto"
	"google.golang.org/grpc"
)

// GRPCPlugin implements plugin.GRPCPlugin from hashicorp/go-plugin.
type GRPCPlugin struct {
	goplugin.Plugin
	Impl ProviderPlugin
}

// GRPCServer registers the gRPC server (plugin side).
func (p *GRPCPlugin) GRPCServer(broker *goplugin.GRPCBroker, s *grpc.Server) error {
	proto.RegisterPluginServiceServer(s, &GRPCServer{Impl: p.Impl, broker: broker})
	return nil
}

// GRPCClient returns a minimal stub client. The host wraps this with its own
// extended GRPCClient that adds broker wiring and HostService startup.
//
//nolint:revive // ctx and broker are required by go-plugin interface
func (p *GRPCPlugin) GRPCClient(_ context.Context, _ *goplugin.GRPCBroker, c *grpc.ClientConn) (any, error) {
	return &GRPCClient{
		client: proto.NewPluginServiceClient(c),
	}, nil
}

// GRPCClient is a minimal stub provider plugin client.
type GRPCClient struct {
	client proto.PluginServiceClient
}

// Client returns the underlying PluginServiceClient for host-side wrapping.
func (c *GRPCClient) Client() proto.PluginServiceClient {
	return c.client
}

// AuthHandlerGRPCPlugin implements plugin.GRPCPlugin for auth handler plugins.
type AuthHandlerGRPCPlugin struct {
	goplugin.Plugin
	Impl AuthHandlerPlugin
}

// GRPCServer registers the auth handler gRPC server.
//
//nolint:revive // broker is required by go-plugin interface
func (p *AuthHandlerGRPCPlugin) GRPCServer(broker *goplugin.GRPCBroker, s *grpc.Server) error {
	proto.RegisterAuthHandlerServiceServer(s, &AuthHandlerGRPCServer{Impl: p.Impl, broker: broker})
	return nil
}

// GRPCClient returns a minimal stub client for auth handler plugins.
//
//nolint:revive // ctx and broker are required by go-plugin interface
func (p *AuthHandlerGRPCPlugin) GRPCClient(_ context.Context, _ *goplugin.GRPCBroker, c *grpc.ClientConn) (any, error) {
	return &AuthHandlerGRPCClient{
		client: proto.NewAuthHandlerServiceClient(c),
	}, nil
}

// AuthHandlerGRPCClient is a minimal stub auth handler client.
type AuthHandlerGRPCClient struct {
	client proto.AuthHandlerServiceClient
}

// Client returns the underlying AuthHandlerServiceClient for host-side wrapping.
func (c *AuthHandlerGRPCClient) Client() proto.AuthHandlerServiceClient {
	return c.client
}
