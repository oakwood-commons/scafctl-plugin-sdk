// Copyright 2025-2026 Oakwood Commons
// SPDX-License-Identifier: Apache-2.0

package plugin

import (
	"context"
	"fmt"

	"github.com/oakwood-commons/scafctl-plugin-sdk/plugin/proto"
	"google.golang.org/grpc"
)

// HostServiceClient wraps the HostService gRPC client (used by plugins).
type HostServiceClient struct {
	client proto.HostServiceClient
}

// NewHostServiceClient creates a HostServiceClient from a gRPC connection.
func NewHostServiceClient(conn *grpc.ClientConn) *HostServiceClient {
	return &HostServiceClient{client: proto.NewHostServiceClient(conn)}
}

// GetSecret retrieves a secret from the host's secret store.
func (c *HostServiceClient) GetSecret(ctx context.Context, name string) (string, bool, error) {
	resp, err := c.client.GetSecret(ctx, &proto.GetSecretRequest{Name: name})
	if err != nil {
		return "", false, fmt.Errorf("host GetSecret: %w", err)
	}
	if resp.Error != "" {
		return "", false, fmt.Errorf("host GetSecret: %s", resp.Error)
	}
	return resp.Value, resp.Found, nil
}

// SetSecret stores a secret in the host's secret store.
func (c *HostServiceClient) SetSecret(ctx context.Context, name, value string) error {
	resp, err := c.client.SetSecret(ctx, &proto.SetSecretRequest{Name: name, Value: value})
	if err != nil {
		return fmt.Errorf("host SetSecret: %w", err)
	}
	if resp.Error != "" {
		return fmt.Errorf("host SetSecret: %s", resp.Error)
	}
	return nil
}

// DeleteSecret removes a secret from the host's secret store.
func (c *HostServiceClient) DeleteSecret(ctx context.Context, name string) error {
	resp, err := c.client.DeleteSecret(ctx, &proto.DeleteSecretRequest{Name: name})
	if err != nil {
		return fmt.Errorf("host DeleteSecret: %w", err)
	}
	if resp.Error != "" {
		return fmt.Errorf("host DeleteSecret: %s", resp.Error)
	}
	return nil
}

// ListSecrets lists secret names from the host's secret store.
func (c *HostServiceClient) ListSecrets(ctx context.Context, pattern string) ([]string, error) {
	resp, err := c.client.ListSecrets(ctx, &proto.ListSecretsRequest{Pattern: pattern})
	if err != nil {
		return nil, fmt.Errorf("host ListSecrets: %w", err)
	}
	if resp.Error != "" {
		return nil, fmt.Errorf("host ListSecrets: %s", resp.Error)
	}
	return resp.Names, nil
}

// GetAuthIdentity retrieves identity claims from the host's auth registry.
func (c *HostServiceClient) GetAuthIdentity(ctx context.Context, handler, scope string) (*proto.Claims, error) {
	resp, err := c.client.GetAuthIdentity(ctx, &proto.GetAuthIdentityRequest{
		HandlerName: handler,
		Scope:       scope,
	})
	if err != nil {
		return nil, fmt.Errorf("host GetAuthIdentity: %w", err)
	}
	if resp.Error != "" {
		return nil, fmt.Errorf("host GetAuthIdentity: %s", resp.Error)
	}
	return resp.Claims, nil
}

// ListAuthHandlers lists available auth handlers on the host.
func (c *HostServiceClient) ListAuthHandlers(ctx context.Context) (handlers []string, defaultHandler string, err error) {
	resp, err := c.client.ListAuthHandlers(ctx, &proto.ListAuthHandlersRequest{})
	if err != nil {
		return nil, "", fmt.Errorf("host ListAuthHandlers: %w", err)
	}
	return resp.HandlerNames, resp.DefaultHandler, nil
}

// GetAuthToken retrieves a valid access token from the host's auth registry.
func (c *HostServiceClient) GetAuthToken(ctx context.Context, handler, scope string, minValidFor int64, forceRefresh bool) (*proto.GetAuthTokenResponse, error) {
	resp, err := c.client.GetAuthToken(ctx, &proto.GetAuthTokenRequest{
		HandlerName:        handler,
		Scope:              scope,
		MinValidForSeconds: minValidFor,
		ForceRefresh:       forceRefresh,
	})
	if err != nil {
		return nil, fmt.Errorf("host GetAuthToken: %w", err)
	}
	if resp.Error != "" {
		return nil, fmt.Errorf("host GetAuthToken: %s", resp.Error)
	}
	return resp, nil
}
