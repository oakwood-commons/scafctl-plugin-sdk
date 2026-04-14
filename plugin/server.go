// Copyright 2025-2026 Oakwood Commons
// SPDX-License-Identifier: Apache-2.0

package plugin

import (
	goplugin "github.com/hashicorp/go-plugin"
)

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
		GRPCServer: goplugin.DefaultGRPCServer,
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
		GRPCServer: goplugin.DefaultGRPCServer,
	})
}
