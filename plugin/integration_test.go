// Copyright 2025-2026 Oakwood Commons
// SPDX-License-Identifier: Apache-2.0

package plugin_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/oakwood-commons/scafctl-plugin-sdk/auth"
	"github.com/oakwood-commons/scafctl-plugin-sdk/plugin"
	"github.com/oakwood-commons/scafctl-plugin-sdk/plugin/proto"
	"github.com/oakwood-commons/scafctl-plugin-sdk/provider"
	"github.com/oakwood-commons/scafctl-plugin-sdk/provider/schemahelper"
	"github.com/oakwood-commons/scafctl-plugin-sdk/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

// startProviderServer registers a GRPCServer backed by impl on a random TCP
// port and returns a connected PluginServiceClient plus a cleanup function.
func startProviderServer(t *testing.T, impl plugin.ProviderPlugin) (proto.PluginServiceClient, func()) {
	t.Helper()
	lis, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)

	s := grpc.NewServer()
	proto.RegisterPluginServiceServer(s, &plugin.GRPCServer{Impl: impl})
	go func() { _ = s.Serve(lis) }()

	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)

	client := proto.NewPluginServiceClient(conn)
	return client, func() {
		conn.Close()
		s.Stop()
	}
}

func TestIntegration_ProviderRoundTrip(t *testing.T) {
	mock := &testutil.MockProviderPlugin{
		GetProvidersFunc: func(_ context.Context) ([]string, error) {
			return []string{"echo", "http"}, nil
		},
		GetProviderDescriptorFunc: func(_ context.Context, name string) (*provider.Descriptor, error) {
			if name != "echo" {
				return nil, errors.New("unknown provider: " + name)
			}
			return &provider.Descriptor{
				Name: "echo", DisplayName: "Echo", Description: "Echoes input back",
				APIVersion: "v1", Version: semver.MustParse("1.2.3"),
				Category: "utility", Tags: []string{"test", "echo"},
				Capabilities:    []provider.Capability{provider.CapabilityTransform},
				SensitiveFields: []string{"secret"},
				Schema: schemahelper.ObjectSchema(
					[]string{"message"},
					map[string]*jsonschema.Schema{
						"message": schemahelper.StringProp("The message to echo",
							schemahelper.WithExample("hello"),
						),
					},
				),
				Links:       []provider.Link{{Name: "docs", URL: "https://example.com"}},
				Examples:    []provider.Example{{Name: "basic", Description: "A basic example", YAML: "message: hello"}},
				Maintainers: []provider.Contact{{Name: "Test", Email: "test@example.com"}},
			}, nil
		},
		ExecuteProviderFunc: func(_ context.Context, _ string, input map[string]any) (*provider.Output, error) {
			msg, _ := input["message"].(string)
			return &provider.Output{
				Data:     map[string]any{"echoed": msg},
				Warnings: []string{"test warning"},
				Metadata: map[string]any{"latency_ms": 42},
			}, nil
		},
		ConfigureProviderFunc: func(_ context.Context, _ string, cfg plugin.ProviderConfig) error {
			if cfg.BinaryName == "" {
				return errors.New("binary name is required")
			}
			return nil
		},
		DescribeWhatIfFunc: func(_ context.Context, _ string, input map[string]any) (string, error) {
			msg, _ := input["message"].(string)
			return "Would echo: " + msg, nil
		},
		ExtractDependenciesFunc: func(_ context.Context, _ string, inputs map[string]any) ([]string, error) {
			if tmpl, ok := inputs["template"].(string); ok && tmpl != "" {
				return []string{"resolver-a", "resolver-b"}, nil
			}
			return nil, nil
		},
	}

	client, cleanup := startProviderServer(t, mock)
	defer cleanup()

	ctx := context.Background()

	t.Run("GetProviders", func(t *testing.T) {
		resp, err := client.GetProviders(ctx, &proto.GetProvidersRequest{})
		require.NoError(t, err)
		assert.Equal(t, []string{"echo", "http"}, resp.ProviderNames)
	})

	t.Run("GetProviderDescriptor", func(t *testing.T) {
		resp, err := client.GetProviderDescriptor(ctx, &proto.GetProviderDescriptorRequest{ProviderName: "echo"})
		require.NoError(t, err)

		d := resp.Descriptor_
		assert.Equal(t, "echo", d.Name)
		assert.Equal(t, "Echo", d.DisplayName)
		assert.Equal(t, "Echoes input back", d.Description)
		assert.Equal(t, "v1", d.ApiVersion)
		assert.Equal(t, "1.2.3", d.Version)
		assert.Equal(t, "utility", d.Category)
		assert.Equal(t, []string{"test", "echo"}, d.Tags)
		assert.Equal(t, []string{"secret"}, d.SensitiveFields)
		assert.Contains(t, d.Capabilities, "transform")

		require.Len(t, d.Links, 1)
		assert.Equal(t, "docs", d.Links[0].Name)
		assert.Equal(t, "https://example.com", d.Links[0].Url)

		require.Len(t, d.Examples, 1)
		assert.Equal(t, "basic", d.Examples[0].Name)

		require.Len(t, d.Maintainers, 1)
		assert.Equal(t, "Test", d.Maintainers[0].Name)

		require.NotNil(t, d.Schema)
		require.Contains(t, d.Schema.Parameters, "message")
		assert.True(t, d.Schema.Parameters["message"].Required)
	})

	t.Run("GetProviderDescriptor_UnknownProvider", func(t *testing.T) {
		_, err := client.GetProviderDescriptor(ctx, &proto.GetProviderDescriptorRequest{ProviderName: "nope"})
		require.Error(t, err)
	})

	t.Run("ConfigureProvider_Success", func(t *testing.T) {
		resp, err := client.ConfigureProvider(ctx, &proto.ConfigureProviderRequest{
			ProviderName: "echo", BinaryName: "scafctl", Quiet: true, NoColor: true,
			HostServiceId: 99,
			Settings:      map[string][]byte{"key": []byte(`"value"`)},
		})
		require.NoError(t, err)
		assert.Empty(t, resp.Error)
		assert.Equal(t, plugin.PluginProtocolVersion, resp.ProtocolVersion)
	})

	t.Run("ConfigureProvider_Error", func(t *testing.T) {
		resp, err := client.ConfigureProvider(ctx, &proto.ConfigureProviderRequest{
			ProviderName: "echo",
		})
		require.NoError(t, err)
		assert.Equal(t, "binary name is required", resp.Error)
	})

	t.Run("ExecuteProvider", func(t *testing.T) {
		inputJSON, _ := json.Marshal(map[string]any{"message": "hello world"})
		resp, err := client.ExecuteProvider(ctx, &proto.ExecuteProviderRequest{
			ProviderName: "echo", Input: inputJSON,
		})
		require.NoError(t, err)
		assert.Empty(t, resp.Error)

		var output provider.Output
		require.NoError(t, json.Unmarshal(resp.Output, &output))
		assert.Equal(t, "hello world", output.Data.(map[string]any)["echoed"])
		assert.Equal(t, []string{"test warning"}, output.Warnings)
	})

	t.Run("ExecuteProvider_InvalidJSON", func(t *testing.T) {
		resp, err := client.ExecuteProvider(ctx, &proto.ExecuteProviderRequest{
			ProviderName: "echo", Input: []byte("{bad"),
		})
		require.NoError(t, err)
		assert.Contains(t, resp.Error, "failed to decode input")
	})

	t.Run("ExecuteProvider_WithContext", func(t *testing.T) {
		contextJSON, _ := json.Marshal(map[string]any{
			"resolverContext": map[string]any{"host": "example.com"},
		})
		paramsJSON, _ := json.Marshal(map[string]any{"env": "prod"})
		iterJSON, _ := json.Marshal("item-0")
		inputJSON, _ := json.Marshal(map[string]any{"message": "ctx-test"})

		resp, err := client.ExecuteProvider(ctx, &proto.ExecuteProviderRequest{
			ProviderName:     "echo",
			Input:            inputJSON,
			DryRun:           true,
			ExecutionMode:    "transform",
			Context:          contextJSON,
			Parameters:       paramsJSON,
			WorkingDirectory: "/tmp/work",
			OutputDirectory:  "/tmp/out",
			ConflictStrategy: "overwrite",
			Backup:           true,
			IterationContext: &proto.IterationContext{
				Item: iterJSON, Index: 5, ItemAlias: "srv", IndexAlias: "idx",
			},
			SolutionMetadata: &proto.SolutionMeta{
				Name: "my-sol", Version: "1.0.0", DisplayName: "My Solution",
				Description: "Test solution", Category: "infra", Tags: []string{"ci"},
			},
		})
		require.NoError(t, err)
		assert.Empty(t, resp.Error)
	})

	t.Run("DescribeWhatIf", func(t *testing.T) {
		inputJSON, _ := json.Marshal(map[string]any{"message": "test"})
		resp, err := client.DescribeWhatIf(ctx, &proto.DescribeWhatIfRequest{
			ProviderName: "echo", Input: inputJSON,
		})
		require.NoError(t, err)
		assert.Equal(t, "Would echo: test", resp.Description)
		assert.Empty(t, resp.Error)
	})

	t.Run("ExtractDependencies", func(t *testing.T) {
		inputJSON, _ := json.Marshal(map[string]any{"template": "{{.resolverA}}"})
		resp, err := client.ExtractDependencies(ctx, &proto.ExtractDependenciesRequest{
			ProviderName: "echo", Inputs: inputJSON,
		})
		require.NoError(t, err)
		assert.Equal(t, []string{"resolver-a", "resolver-b"}, resp.Dependencies)
	})

	t.Run("ExtractDependencies_NoDeps", func(t *testing.T) {
		inputJSON, _ := json.Marshal(map[string]any{"message": "plain"})
		resp, err := client.ExtractDependencies(ctx, &proto.ExtractDependenciesRequest{
			ProviderName: "echo", Inputs: inputJSON,
		})
		require.NoError(t, err)
		assert.Empty(t, resp.Dependencies)
	})

	t.Run("StopProvider", func(t *testing.T) {
		resp, err := client.StopProvider(ctx, &proto.StopProviderRequest{ProviderName: "echo"})
		require.NoError(t, err)
		assert.Empty(t, resp.Error)
	})
}

// --- Streaming Integration Tests ---

func TestIntegration_ExecuteProviderStream(t *testing.T) {
	t.Parallel()

	mock := &testutil.MockProviderPlugin{
		ExecuteProviderStreamFunc: func(_ context.Context, _ string, input map[string]any, cb func(plugin.StreamChunk)) error {
			msg, _ := input["message"].(string)
			cb(plugin.StreamChunk{Stdout: []byte("Processing: " + msg + "\n")})
			cb(plugin.StreamChunk{Stderr: []byte("debug info\n")})
			cb(plugin.StreamChunk{Result: &provider.Output{
				Data: map[string]any{"echoed": msg},
			}})
			return nil
		},
	}

	client, cleanup := startProviderServer(t, mock)
	defer cleanup()

	ctx := context.Background()
	inputJSON, _ := json.Marshal(map[string]any{"message": "stream-test"})
	stream, err := client.ExecuteProviderStream(ctx, &proto.ExecuteProviderRequest{
		ProviderName: "echo", Input: inputJSON,
	})
	require.NoError(t, err)

	var chunks []*proto.ExecuteProviderStreamChunk
	for {
		chunk, recvErr := stream.Recv()
		if errors.Is(recvErr, io.EOF) {
			break
		}
		require.NoError(t, recvErr)
		chunks = append(chunks, chunk)
	}

	require.Len(t, chunks, 3)
	assert.Equal(t, "Processing: stream-test\n", string(chunks[0].GetStdout()))
	assert.Equal(t, "debug info\n", string(chunks[1].GetStderr()))
	assert.NotNil(t, chunks[2].GetResult())
	assert.Empty(t, chunks[2].GetResult().Error)
}

func TestIntegration_ExecuteProviderStream_Unimplemented(t *testing.T) {
	t.Parallel()

	mock := &testutil.MockProviderPlugin{}
	client, cleanup := startProviderServer(t, mock)
	defer cleanup()

	stream, err := client.ExecuteProviderStream(context.Background(), &proto.ExecuteProviderRequest{
		ProviderName: "echo",
	})
	require.NoError(t, err)

	_, err = stream.Recv()
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unimplemented, st.Code())
}

func TestIntegration_ExecuteProviderStream_Error(t *testing.T) {
	t.Parallel()

	mock := &testutil.MockProviderPlugin{
		ExecuteProviderStreamFunc: func(_ context.Context, _ string, _ map[string]any, cb func(plugin.StreamChunk)) error {
			cb(plugin.StreamChunk{Stdout: []byte("partial output\n")})
			return errors.New("execution failed midway")
		},
	}

	client, cleanup := startProviderServer(t, mock)
	defer cleanup()

	inputJSON, _ := json.Marshal(map[string]any{"message": "fail"})
	stream, err := client.ExecuteProviderStream(context.Background(), &proto.ExecuteProviderRequest{
		ProviderName: "echo", Input: inputJSON,
	})
	require.NoError(t, err)

	var chunks []*proto.ExecuteProviderStreamChunk
	for {
		chunk, recvErr := stream.Recv()
		if errors.Is(recvErr, io.EOF) {
			break
		}
		require.NoError(t, recvErr)
		chunks = append(chunks, chunk)
	}

	require.Len(t, chunks, 2)
	assert.Equal(t, "partial output\n", string(chunks[0].GetStdout()))
	assert.Equal(t, "execution failed midway", chunks[1].GetResult().Error)
}

// --- Auth Handler Plugin Integration Tests ---

type mockAuthPlugin struct {
	handlers         []plugin.AuthHandlerInfo
	configuredNames  []string
	loginResult      *plugin.LoginResponse
	loginErr         error
	statusResult     *auth.Status
	tokenResult      *plugin.TokenResponse
	cachedTokens     []*auth.CachedTokenInfo
	purgedCount      int
	stoppedHandlers  []string
	loggedOutHandler string
}

func (m *mockAuthPlugin) GetAuthHandlers(_ context.Context) ([]plugin.AuthHandlerInfo, error) {
	return m.handlers, nil
}

func (m *mockAuthPlugin) ConfigureAuthHandler(_ context.Context, name string, _ plugin.ProviderConfig) error {
	m.configuredNames = append(m.configuredNames, name)
	return nil
}

func (m *mockAuthPlugin) Login(_ context.Context, _ string, _ plugin.LoginRequest, cb func(plugin.DeviceCodePrompt)) (*plugin.LoginResponse, error) {
	if m.loginErr != nil {
		return nil, m.loginErr
	}
	cb(plugin.DeviceCodePrompt{
		UserCode:        "ABCD-1234",
		VerificationURI: "https://example.com/device",
		Message:         "Enter code",
	})
	return m.loginResult, nil
}

func (m *mockAuthPlugin) Logout(_ context.Context, name string) error {
	m.loggedOutHandler = name
	return nil
}

func (m *mockAuthPlugin) GetStatus(_ context.Context, _ string) (*auth.Status, error) {
	return m.statusResult, nil
}

func (m *mockAuthPlugin) GetToken(_ context.Context, _ string, _ plugin.TokenRequest) (*plugin.TokenResponse, error) {
	return m.tokenResult, nil
}

func (m *mockAuthPlugin) ListCachedTokens(_ context.Context, _ string) ([]*auth.CachedTokenInfo, error) {
	return m.cachedTokens, nil
}

func (m *mockAuthPlugin) PurgeExpiredTokens(_ context.Context, _ string) (int, error) {
	return m.purgedCount, nil
}

func (m *mockAuthPlugin) StopAuthHandler(_ context.Context, name string) error {
	m.stoppedHandlers = append(m.stoppedHandlers, name)
	return nil
}

func startAuthServer(t *testing.T, impl plugin.AuthHandlerPlugin) (proto.AuthHandlerServiceClient, func()) {
	t.Helper()
	lis, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)

	s := grpc.NewServer()
	proto.RegisterAuthHandlerServiceServer(s, &plugin.AuthHandlerGRPCServer{Impl: impl})
	go func() { _ = s.Serve(lis) }()

	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)

	client := proto.NewAuthHandlerServiceClient(conn)
	return client, func() {
		conn.Close()
		s.Stop()
	}
}

func TestIntegration_AuthHandlerRoundTrip(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	mock := &mockAuthPlugin{
		handlers: []plugin.AuthHandlerInfo{
			{
				Name: "github", DisplayName: "GitHub",
				Flows:        []auth.Flow{auth.FlowDeviceCode, auth.FlowClientCredentials},
				Capabilities: []auth.Capability{auth.CapScopesOnLogin, auth.CapHostname},
			},
			{
				Name: "azure", DisplayName: "Azure AD",
				Flows:        []auth.Flow{auth.FlowInteractive},
				Capabilities: []auth.Capability{auth.CapTenantID},
			},
		},
		loginResult: &plugin.LoginResponse{
			Claims: &auth.Claims{
				Issuer:  "https://github.com",
				Subject: "user123",
				Name:    "Test User",
				Email:   "test@example.com",
			},
			ExpiresAt: now.Add(time.Hour),
		},
		statusResult: &auth.Status{
			Authenticated: true,
			Claims: &auth.Claims{
				Subject: "user123",
				Name:    "Test User",
			},
			ExpiresAt:    now.Add(time.Hour),
			TenantID:     "tenant-1",
			IdentityType: auth.IdentityTypeUser,
			Scopes:       []string{"read", "write"},
		},
		tokenResult: &plugin.TokenResponse{
			AccessToken: "tok-abc",
			TokenType:   "Bearer",
			ExpiresAt:   now.Add(30 * time.Minute),
			Scope:       "read write",
			Flow:        auth.FlowDeviceCode,
			SessionID:   "sess-1",
		},
		cachedTokens: []*auth.CachedTokenInfo{
			{
				Handler: "github", TokenKind: "access_token", Scope: "read",
				TokenType: "Bearer", Flow: auth.FlowDeviceCode,
				ExpiresAt: now.Add(time.Hour), CachedAt: now,
				IsExpired: false, SessionID: "sess-1",
			},
		},
		purgedCount: 3,
	}

	client, cleanup := startAuthServer(t, mock)
	defer cleanup()

	ctx := context.Background()

	t.Run("GetAuthHandlers", func(t *testing.T) {
		resp, err := client.GetAuthHandlers(ctx, &proto.GetAuthHandlersRequest{})
		require.NoError(t, err)
		require.Len(t, resp.Handlers, 2)

		gh := resp.Handlers[0]
		assert.Equal(t, "github", gh.Name)
		assert.Equal(t, "GitHub", gh.DisplayName)
		assert.Equal(t, []string{"device_code", "client_credentials"}, gh.Flows)
		assert.Equal(t, []string{"scopes_on_login", "hostname"}, gh.Capabilities)

		az := resp.Handlers[1]
		assert.Equal(t, "azure", az.Name)
	})

	t.Run("ConfigureAuthHandler", func(t *testing.T) {
		resp, err := client.ConfigureAuthHandler(ctx, &proto.ConfigureAuthHandlerRequest{
			HandlerName: "github", BinaryName: "scafctl", Quiet: true,
			Settings: map[string][]byte{"org": []byte(`"my-org"`)},
		})
		require.NoError(t, err)
		assert.Empty(t, resp.Error)
		assert.Equal(t, plugin.PluginProtocolVersion, resp.ProtocolVersion)
	})

	t.Run("Login", func(t *testing.T) {
		stream, err := client.Login(ctx, &proto.LoginRequest{
			HandlerName:    "github",
			Flow:           "device_code",
			Scopes:         []string{"repo", "user"},
			TenantId:       "tenant-1",
			TimeoutSeconds: 300,
		})
		require.NoError(t, err)

		var messages []*proto.LoginStreamMessage
		for {
			msg, recvErr := stream.Recv()
			if errors.Is(recvErr, io.EOF) {
				break
			}
			require.NoError(t, recvErr)
			messages = append(messages, msg)
		}

		require.Len(t, messages, 2)

		prompt := messages[0].GetDeviceCodePrompt()
		require.NotNil(t, prompt)
		assert.Equal(t, "ABCD-1234", prompt.UserCode)
		assert.Equal(t, "https://example.com/device", prompt.VerificationUri)

		result := messages[1].GetResult()
		require.NotNil(t, result)
		assert.Equal(t, "user123", result.Claims.Subject)
		assert.Equal(t, "Test User", result.Claims.Name)
	})

	t.Run("Login_Error", func(t *testing.T) {
		errMock := &mockAuthPlugin{
			loginErr: errors.New("auth failed"),
		}
		errClient, errCleanup := startAuthServer(t, errMock)
		defer errCleanup()

		stream, err := errClient.Login(ctx, &proto.LoginRequest{
			HandlerName: "test", Flow: "device_code",
		})
		require.NoError(t, err)

		var messages []*proto.LoginStreamMessage
		for {
			msg, recvErr := stream.Recv()
			if errors.Is(recvErr, io.EOF) {
				break
			}
			require.NoError(t, recvErr)
			messages = append(messages, msg)
		}

		require.Len(t, messages, 1)
		assert.Equal(t, "auth failed", messages[0].GetError())
	})

	t.Run("Logout", func(t *testing.T) {
		resp, err := client.Logout(ctx, &proto.LogoutRequest{HandlerName: "github"})
		require.NoError(t, err)
		assert.NotNil(t, resp)
	})

	t.Run("GetStatus", func(t *testing.T) {
		resp, err := client.GetStatus(ctx, &proto.GetStatusRequest{HandlerName: "github"})
		require.NoError(t, err)
		assert.True(t, resp.Authenticated)
		assert.Equal(t, "user123", resp.Claims.Subject)
		assert.Equal(t, "tenant-1", resp.TenantId)
		assert.Equal(t, "user", resp.IdentityType)
		assert.Equal(t, []string{"read", "write"}, resp.Scopes)
	})

	t.Run("GetToken", func(t *testing.T) {
		resp, err := client.GetToken(ctx, &proto.GetTokenRequest{
			HandlerName:        "github",
			Scope:              "read",
			MinValidForSeconds: 60,
			ForceRefresh:       true,
		})
		require.NoError(t, err)
		assert.Equal(t, "tok-abc", resp.AccessToken)
		assert.Equal(t, "Bearer", resp.TokenType)
		assert.Equal(t, "read write", resp.Scope)
		assert.Equal(t, "device_code", resp.Flow)
		assert.Equal(t, "sess-1", resp.SessionId)
	})

	t.Run("ListCachedTokens", func(t *testing.T) {
		resp, err := client.ListCachedTokens(ctx, &proto.ListCachedTokensRequest{HandlerName: "github"})
		require.NoError(t, err)
		require.Len(t, resp.Tokens, 1)
		assert.Equal(t, "github", resp.Tokens[0].Handler)
		assert.Equal(t, "access_token", resp.Tokens[0].TokenKind)
		assert.Equal(t, "device_code", resp.Tokens[0].Flow)
		assert.False(t, resp.Tokens[0].IsExpired)
	})

	t.Run("PurgeExpiredTokens", func(t *testing.T) {
		resp, err := client.PurgeExpiredTokens(ctx, &proto.PurgeExpiredTokensRequest{HandlerName: "github"})
		require.NoError(t, err)
		assert.Equal(t, int32(3), resp.PurgedCount)
	})

	t.Run("StopAuthHandler", func(t *testing.T) {
		resp, err := client.StopAuthHandler(ctx, &proto.StopAuthHandlerRequest{HandlerName: "github"})
		require.NoError(t, err)
		assert.Empty(t, resp.Error)
	})
}

// --- GRPCPlugin / AuthHandlerGRPCPlugin round-trip tests ---

func TestIntegration_GRPCPlugin_ServerRegistration(t *testing.T) {
	t.Parallel()

	mock := &testutil.MockProviderPlugin{
		GetProvidersFunc: func(_ context.Context) ([]string, error) {
			return []string{"test-provider"}, nil
		},
	}

	grpcPlugin := &plugin.GRPCPlugin{Impl: mock}

	lis, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)

	s := grpc.NewServer()
	require.NoError(t, grpcPlugin.GRPCServer(nil, s))
	go func() { _ = s.Serve(lis) }()
	defer s.Stop()

	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer conn.Close()

	raw, err := grpcPlugin.GRPCClient(context.Background(), nil, conn)
	require.NoError(t, err)

	grpcClient, ok := raw.(*plugin.GRPCClient)
	require.True(t, ok)

	resp, err := grpcClient.Client().GetProviders(context.Background(), &proto.GetProvidersRequest{})
	require.NoError(t, err)
	assert.Equal(t, []string{"test-provider"}, resp.ProviderNames)
}

func TestIntegration_AuthHandlerGRPCPlugin_ServerRegistration(t *testing.T) {
	t.Parallel()

	mock := &mockAuthPlugin{
		handlers: []plugin.AuthHandlerInfo{
			{Name: "test-handler", DisplayName: "Test Handler"},
		},
	}

	grpcPlugin := &plugin.AuthHandlerGRPCPlugin{Impl: mock}

	lis, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)

	s := grpc.NewServer()
	require.NoError(t, grpcPlugin.GRPCServer(nil, s))
	go func() { _ = s.Serve(lis) }()
	defer s.Stop()

	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer conn.Close()

	raw, err := grpcPlugin.GRPCClient(context.Background(), nil, conn)
	require.NoError(t, err)

	authClient, ok := raw.(*plugin.AuthHandlerGRPCClient)
	require.True(t, ok)

	resp, err := authClient.Client().GetAuthHandlers(context.Background(), &proto.GetAuthHandlersRequest{})
	require.NoError(t, err)
	require.Len(t, resp.Handlers, 1)
	assert.Equal(t, "test-handler", resp.Handlers[0].Name)
}

// --- Echo Plugin End-to-End Integration Test ---

type echoPlugin struct{}

func (p *echoPlugin) GetProviders(_ context.Context) ([]string, error) {
	return []string{"echo"}, nil
}

func (p *echoPlugin) GetProviderDescriptor(_ context.Context, name string) (*provider.Descriptor, error) {
	if name != "echo" {
		return nil, errors.New("unknown provider: " + name)
	}
	return &provider.Descriptor{
		Name: "echo", DisplayName: "Echo", Description: "Echoes input back",
		APIVersion: "v1", Version: semver.MustParse("1.0.0"),
		Capabilities: []provider.Capability{provider.CapabilityTransform},
		Schema: schemahelper.ObjectSchema(
			[]string{"message"},
			map[string]*jsonschema.Schema{
				"message": schemahelper.StringProp("The message"),
			},
		),
	}, nil
}

func (p *echoPlugin) ConfigureProvider(_ context.Context, _ string, _ plugin.ProviderConfig) error {
	return nil
}

func (p *echoPlugin) ExecuteProvider(_ context.Context, _ string, input map[string]any) (*provider.Output, error) {
	msg, _ := input["message"].(string)
	return &provider.Output{Data: map[string]any{"echoed": msg}}, nil
}

func (p *echoPlugin) ExecuteProviderStream(_ context.Context, _ string, _ map[string]any, _ func(plugin.StreamChunk)) error {
	return plugin.ErrStreamingNotSupported
}

func (p *echoPlugin) DescribeWhatIf(_ context.Context, _ string, _ map[string]any) (string, error) {
	return "Would echo message", nil
}

func (p *echoPlugin) ExtractDependencies(_ context.Context, _ string, _ map[string]any) ([]string, error) {
	return nil, nil
}

func (p *echoPlugin) StopProvider(_ context.Context, _ string) error {
	return nil
}

func TestIntegration_EchoPlugin_EndToEnd(t *testing.T) {
	t.Parallel()

	client, cleanup := startProviderServer(t, &echoPlugin{})
	defer cleanup()

	ctx := context.Background()

	// 1. List providers
	listResp, err := client.GetProviders(ctx, &proto.GetProvidersRequest{})
	require.NoError(t, err)
	assert.Equal(t, []string{"echo"}, listResp.ProviderNames)

	// 2. Get descriptor
	descResp, err := client.GetProviderDescriptor(ctx, &proto.GetProviderDescriptorRequest{ProviderName: "echo"})
	require.NoError(t, err)
	assert.Equal(t, "echo", descResp.Descriptor_.Name)
	assert.Equal(t, "1.0.0", descResp.Descriptor_.Version)

	// 3. Configure
	cfgResp, err := client.ConfigureProvider(ctx, &proto.ConfigureProviderRequest{
		ProviderName: "echo", BinaryName: "scafctl",
	})
	require.NoError(t, err)
	assert.Empty(t, cfgResp.Error)

	// 4. Execute
	inputJSON, _ := json.Marshal(map[string]any{"message": "integration test"})
	execResp, err := client.ExecuteProvider(ctx, &proto.ExecuteProviderRequest{
		ProviderName: "echo", Input: inputJSON,
	})
	require.NoError(t, err)
	assert.Empty(t, execResp.Error)

	var output provider.Output
	require.NoError(t, json.Unmarshal(execResp.Output, &output))
	assert.Equal(t, "integration test", output.Data.(map[string]any)["echoed"])

	// 5. WhatIf
	whatIfResp, err := client.DescribeWhatIf(ctx, &proto.DescribeWhatIfRequest{ProviderName: "echo"})
	require.NoError(t, err)
	assert.Equal(t, "Would echo message", whatIfResp.Description)

	// 6. Streaming not supported
	stream, err := client.ExecuteProviderStream(ctx, &proto.ExecuteProviderRequest{ProviderName: "echo"})
	require.NoError(t, err)
	_, recvErr := stream.Recv()
	require.Error(t, recvErr)
	st, ok := status.FromError(recvErr)
	require.True(t, ok)
	assert.Equal(t, codes.Unimplemented, st.Code())

	// 7. Stop
	stopResp, err := client.StopProvider(ctx, &proto.StopProviderRequest{ProviderName: "echo"})
	require.NoError(t, err)
	assert.Empty(t, stopResp.Error)
}
