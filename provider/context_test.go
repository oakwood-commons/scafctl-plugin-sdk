// Copyright 2025-2026 Oakwood Commons
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithExecutionMode(t *testing.T) {
	ctx := context.Background()
	mode, ok := ExecutionModeFromContext(ctx)
	assert.False(t, ok)
	assert.Equal(t, Capability(""), mode)

	ctx = WithExecutionMode(ctx, CapabilityTransform)
	mode, ok = ExecutionModeFromContext(ctx)
	assert.True(t, ok)
	assert.Equal(t, CapabilityTransform, mode)
}

func TestWithDryRun(t *testing.T) {
	ctx := context.Background()
	assert.False(t, DryRunFromContext(ctx))

	ctx = WithDryRun(ctx, true)
	assert.True(t, DryRunFromContext(ctx))

	ctx = WithDryRun(ctx, false)
	assert.False(t, DryRunFromContext(ctx))
}

func TestWithResolverContext(t *testing.T) {
	ctx := context.Background()
	rc, ok := ResolverContextFromContext(ctx)
	assert.False(t, ok)
	assert.Nil(t, rc)

	resolverCtx := map[string]any{"key": "value"}
	ctx = WithResolverContext(ctx, resolverCtx)
	rc, ok = ResolverContextFromContext(ctx)
	assert.True(t, ok)
	assert.Equal(t, "value", rc["key"])
}

func TestWithParameters(t *testing.T) {
	ctx := context.Background()
	params, ok := ParametersFromContext(ctx)
	assert.False(t, ok)
	assert.Nil(t, params)

	params = map[string]any{"foo": "bar"}
	ctx = WithParameters(ctx, params)
	got, ok := ParametersFromContext(ctx)
	assert.True(t, ok)
	assert.Equal(t, "bar", got["foo"])
}

func TestWithIterationContext(t *testing.T) {
	ctx := context.Background()
	iter, ok := IterationContextFromContext(ctx)
	assert.False(t, ok)
	assert.Nil(t, iter)

	ic := &IterationContext{Item: "item1", Index: 3, ItemAlias: "server", IndexAlias: "i"}
	ctx = WithIterationContext(ctx, ic)
	iter, ok = IterationContextFromContext(ctx)
	assert.True(t, ok)
	require.NotNil(t, iter)
	assert.Equal(t, "item1", iter.Item)
	assert.Equal(t, 3, iter.Index)
	assert.Equal(t, "server", iter.ItemAlias)
	assert.Equal(t, "i", iter.IndexAlias)
}

func TestWithIOStreams(t *testing.T) {
	ctx := context.Background()
	streams, ok := IOStreamsFromContext(ctx)
	assert.False(t, ok)
	assert.Nil(t, streams)

	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	ios := &IOStreams{Out: out, ErrOut: errOut}
	ctx = WithIOStreams(ctx, ios)
	streams, ok = IOStreamsFromContext(ctx)
	assert.True(t, ok)
	require.NotNil(t, streams)
	assert.Equal(t, out, streams.Out)
	assert.Equal(t, errOut, streams.ErrOut)
}

func TestWithSolutionMetadata(t *testing.T) {
	ctx := context.Background()
	meta, ok := SolutionMetadataFromContext(ctx)
	assert.False(t, ok)
	assert.Nil(t, meta)

	sm := &SolutionMeta{
		Name: "test-sol", Version: "1.0.0", DisplayName: "Test",
		Description: "A test solution", Category: "infra", Tags: []string{"tag1"},
	}
	ctx = WithSolutionMetadata(ctx, sm)
	meta, ok = SolutionMetadataFromContext(ctx)
	assert.True(t, ok)
	require.NotNil(t, meta)
	assert.Equal(t, "test-sol", meta.Name)
	assert.Equal(t, "1.0.0", meta.Version)
	assert.Equal(t, []string{"tag1"}, meta.Tags)
}

func TestWithOutputDirectory(t *testing.T) {
	ctx := context.Background()
	dir, ok := OutputDirectoryFromContext(ctx)
	assert.False(t, ok)
	assert.Empty(t, dir)

	ctx = WithOutputDirectory(ctx, "/tmp/output")
	dir, ok = OutputDirectoryFromContext(ctx)
	assert.True(t, ok)
	assert.Equal(t, "/tmp/output", dir)
}

func TestWithWorkingDirectory(t *testing.T) {
	ctx := context.Background()
	dir, ok := WorkingDirectoryFromContext(ctx)
	assert.False(t, ok)
	assert.Empty(t, dir)

	ctx = WithWorkingDirectory(ctx, "/tmp/work")
	dir, ok = WorkingDirectoryFromContext(ctx)
	assert.True(t, ok)
	assert.Equal(t, "/tmp/work", dir)
}

func TestWithConflictStrategy(t *testing.T) {
	ctx := context.Background()
	s, ok := ConflictStrategyFromContext(ctx)
	assert.False(t, ok)
	assert.Empty(t, s)

	ctx = WithConflictStrategy(ctx, "overwrite")
	s, ok = ConflictStrategyFromContext(ctx)
	assert.True(t, ok)
	assert.Equal(t, "overwrite", s)
}

func TestWithBackup(t *testing.T) {
	ctx := context.Background()
	b, ok := BackupFromContext(ctx)
	assert.False(t, ok)
	assert.False(t, b)

	ctx = WithBackup(ctx, true)
	b, ok = BackupFromContext(ctx)
	assert.True(t, ok)
	assert.True(t, b)
}
