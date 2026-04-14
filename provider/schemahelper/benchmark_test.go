// Copyright 2025-2026 Oakwood Commons
// SPDX-License-Identifier: Apache-2.0

package schemahelper

import (
	"encoding/json"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
)

func BenchmarkStringProp(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = StringProp("A string property",
			WithExample("hello"),
			WithMinLength(1),
			WithMaxLength(100),
			WithPattern("^[a-z]+$"),
		)
	}
}

func BenchmarkIntProp(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = IntProp("An integer property",
			WithMinimum(0),
			WithMaximum(1000),
		)
	}
}

func BenchmarkBoolProp(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = BoolProp("A boolean property",
			WithDefault(json.RawMessage("false")),
		)
	}
}

func BenchmarkArrayProp(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = ArrayProp("An array property",
			WithMinItems(1),
			WithMaxItems(10),
			WithItems(&jsonschema.Schema{Type: "string"}),
		)
	}
}

func BenchmarkObjectSchema_Small(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = ObjectSchema(
			[]string{"name"},
			map[string]*jsonschema.Schema{
				"name":  StringProp("Name"),
				"count": IntProp("Count"),
			},
		)
	}
}

func BenchmarkObjectSchema_Large(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = ObjectSchema(
			[]string{"name", "url", "count"},
			map[string]*jsonschema.Schema{
				"name":        StringProp("Name", WithMinLength(1), WithMaxLength(100)),
				"url":         StringProp("URL", WithFormat("uri")),
				"count":       IntProp("Count", WithMinimum(0), WithMaximum(1000)),
				"enabled":     BoolProp("Enabled", WithDefault(json.RawMessage("true"))),
				"tags":        ArrayProp("Tags", WithMinItems(0), WithMaxItems(20)),
				"description": StringProp("Description", WithMaxLength(500)),
				"priority":    NumberProp("Priority", WithMinimum(0), WithMaximum(10)),
				"data":        AnyProp("Arbitrary data"),
			},
		)
	}
}

func BenchmarkWithEnum(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = StringProp("Status",
			WithEnum("active", "inactive", "pending", "archived"),
		)
	}
}

func BenchmarkAllOptions(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = StringProp("Full property",
			WithTitle("Full Property"),
			WithDescription("A property with all options"),
			WithExample("example"),
			WithDefault(json.RawMessage(`"default"`)),
			WithMinLength(1),
			WithMaxLength(100),
			WithPattern("^[a-z]+$"),
			WithFormat("hostname"),
			WithEnum("a", "b", "c"),
			WithDeprecated(),
			WithWriteOnly(),
		)
	}
}
