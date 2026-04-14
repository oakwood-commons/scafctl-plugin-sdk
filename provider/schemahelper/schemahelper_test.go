// Copyright 2025-2026 Oakwood Commons
// SPDX-License-Identifier: Apache-2.0

package schemahelper

import (
	"encoding/json"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStringProp(t *testing.T) {
	s := StringProp("A description")
	assert.Equal(t, "string", s.Type)
	assert.Equal(t, "A description", s.Description)
}

func TestIntProp(t *testing.T) {
	s := IntProp("An integer")
	assert.Equal(t, "integer", s.Type)
	assert.Equal(t, "An integer", s.Description)
}

func TestNumberProp(t *testing.T) {
	s := NumberProp("A number")
	assert.Equal(t, "number", s.Type)
}

func TestBoolProp(t *testing.T) {
	s := BoolProp("A boolean")
	assert.Equal(t, "boolean", s.Type)
}

func TestArrayProp(t *testing.T) {
	s := ArrayProp("An array")
	assert.Equal(t, "array", s.Type)
}

func TestAnyProp(t *testing.T) {
	s := AnyProp("Any value")
	assert.Empty(t, s.Type)
	assert.Equal(t, "Any value", s.Description)
}

func TestObjectProp(t *testing.T) {
	s := ObjectProp("An object", []string{"name"}, map[string]*jsonschema.Schema{
		"name": StringProp("Name"),
	})
	assert.Equal(t, "object", s.Type)
	assert.Equal(t, []string{"name"}, s.Required)
	require.Contains(t, s.Properties, "name")
}

func TestObjectProp_NoRequired(t *testing.T) {
	s := ObjectProp("desc", nil, map[string]*jsonschema.Schema{"a": StringProp("a")})
	assert.Nil(t, s.Required)
}

func TestObjectSchema(t *testing.T) {
	s := ObjectSchema([]string{"f1"}, map[string]*jsonschema.Schema{
		"f1": StringProp("Field 1"),
		"f2": IntProp("Field 2"),
	})
	assert.Equal(t, "object", s.Type)
	assert.Equal(t, []string{"f1"}, s.Required)
	assert.Len(t, s.Properties, 2)
}

func TestObjectSchema_NoRequired(t *testing.T) {
	s := ObjectSchema(nil, map[string]*jsonschema.Schema{"a": StringProp("a")})
	assert.Nil(t, s.Required)
}

func TestWithDescription(t *testing.T) {
	s := StringProp("original", WithDescription("overridden"))
	assert.Equal(t, "overridden", s.Description)
}

func TestWithExample(t *testing.T) {
	s := StringProp("desc", WithExample("ex1", "ex2"))
	assert.Equal(t, []any{"ex1", "ex2"}, s.Examples)
}

func TestWithDefault(t *testing.T) {
	s := StringProp("desc", WithDefault("default_val"))
	require.NotNil(t, s.Default)
	var val string
	require.NoError(t, json.Unmarshal(s.Default, &val))
	assert.Equal(t, "default_val", val)
}

func TestWithEnum(t *testing.T) {
	s := StringProp("desc", WithEnum("a", "b", "c"))
	assert.Equal(t, []any{"a", "b", "c"}, s.Enum)
}

func TestWithPattern(t *testing.T) {
	s := StringProp("desc", WithPattern("^[a-z]+$"))
	assert.Equal(t, "^[a-z]+$", s.Pattern)
}

func TestWithMinLength(t *testing.T) {
	s := StringProp("desc", WithMinLength(5))
	require.NotNil(t, s.MinLength)
	assert.Equal(t, 5, *s.MinLength)
}

func TestWithMaxLength(t *testing.T) {
	s := StringProp("desc", WithMaxLength(100))
	require.NotNil(t, s.MaxLength)
	assert.Equal(t, 100, *s.MaxLength)
}

func TestWithMinimum(t *testing.T) {
	s := IntProp("desc", WithMinimum(0))
	require.NotNil(t, s.Minimum)
	assert.Equal(t, 0.0, *s.Minimum)
}

func TestWithMaximum(t *testing.T) {
	s := IntProp("desc", WithMaximum(100))
	require.NotNil(t, s.Maximum)
	assert.Equal(t, 100.0, *s.Maximum)
}

func TestWithMinItems(t *testing.T) {
	s := ArrayProp("desc", WithMinItems(1))
	require.NotNil(t, s.MinItems)
	assert.Equal(t, 1, *s.MinItems)
}

func TestWithMaxItems(t *testing.T) {
	s := ArrayProp("desc", WithMaxItems(10))
	require.NotNil(t, s.MaxItems)
	assert.Equal(t, 10, *s.MaxItems)
}

func TestWithFormat(t *testing.T) {
	s := StringProp("desc", WithFormat("uri"))
	assert.Equal(t, "uri", s.Format)
}

func TestWithDeprecated(t *testing.T) {
	s := StringProp("desc", WithDeprecated())
	assert.True(t, s.Deprecated)
}

func TestWithWriteOnly(t *testing.T) {
	s := StringProp("desc", WithWriteOnly())
	assert.True(t, s.WriteOnly)
}

func TestWithTitle(t *testing.T) {
	s := StringProp("desc", WithTitle("My Title"))
	assert.Equal(t, "My Title", s.Title)
}

func TestWithItems(t *testing.T) {
	itemSchema := StringProp("item")
	s := ArrayProp("desc", WithItems(itemSchema))
	assert.Equal(t, itemSchema, s.Items)
}

func TestWithAdditionalProperties(t *testing.T) {
	addlSchema := StringProp("additional")
	s := ObjectProp("desc", nil, nil, WithAdditionalProperties(addlSchema))
	assert.Equal(t, addlSchema, s.AdditionalProperties)
}

func TestMultipleOptions(t *testing.T) {
	s := StringProp("desc",
		WithMinLength(1),
		WithMaxLength(50),
		WithPattern("^[a-z]+$"),
		WithFormat("hostname"),
		WithExample("example.com"),
		WithDefault("localhost"),
	)
	assert.Equal(t, "string", s.Type)
	require.NotNil(t, s.MinLength)
	assert.Equal(t, 1, *s.MinLength)
	require.NotNil(t, s.MaxLength)
	assert.Equal(t, 50, *s.MaxLength)
	assert.Equal(t, "^[a-z]+$", s.Pattern)
	assert.Equal(t, "hostname", s.Format)
	assert.Len(t, s.Examples, 1)
	assert.NotNil(t, s.Default)
}
