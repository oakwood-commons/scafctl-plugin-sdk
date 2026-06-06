// Copyright 2025-2026 Oakwood Commons
// SPDX-License-Identifier: Apache-2.0

package plugin

import (
	goplugin "github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
)

// maxRecvMsgSize is the maximum gRPC message size the plugin server will
// accept. Matches the host's MaxCallSendMsgSize (64 MB).
const maxRecvMsgSize = 64 << 20 // 64 MB

// newGRPCServer creates a gRPC server with MaxRecvMsgSize set to match the
// host's send limit. The default is prepended so caller-supplied options can
// override it.
func newGRPCServer(opts []grpc.ServerOption) *grpc.Server {
	opts = append([]grpc.ServerOption{grpc.MaxRecvMsgSize(maxRecvMsgSize)}, opts...)
	return grpc.NewServer(opts...)
}

// Serve is a helper function for plugin implementers to serve their provider plugins.
func Serve(impl ProviderPlugin) {
	hc := HandshakeConfig()
	goplugin.Serve(&goplugin.ServeConfig{
		HandshakeConfig: goplugin.HandshakeConfig{
			ProtocolVersion:  hc.ProtocolVersion,
			MagicCookieKey:   hc.MagicCookieKey,
			MagicCookieValue: hc.MagicCookieValue,
		},
		Plugins: map[string]goplugin.Plugin{
			PluginName: &GRPCPlugin{Impl: impl},
		},
		GRPCServer: newGRPCServer,
	})
}

// ServeAuthHandler is a helper function for plugin implementers to serve their
// auth handler plugins.
func ServeAuthHandler(impl AuthHandlerPlugin) {
	hc := AuthHandlerHandshakeConfig()
	goplugin.Serve(&goplugin.ServeConfig{
		HandshakeConfig: goplugin.HandshakeConfig{
			ProtocolVersion:  hc.ProtocolVersion,
			MagicCookieKey:   hc.MagicCookieKey,
			MagicCookieValue: hc.MagicCookieValue,
		},
		Plugins: map[string]goplugin.Plugin{
			AuthHandlerPluginName: &AuthHandlerGRPCPlugin{Impl: impl},
		},
		GRPCServer: newGRPCServer,
	})
}
