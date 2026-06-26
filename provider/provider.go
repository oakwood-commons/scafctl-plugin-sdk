// Copyright 2025-2026 Oakwood Commons
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/Masterminds/semver/v3"
	"github.com/go-logr/logr"
	"github.com/google/jsonschema-go/jsonschema"
)

// Provider is the core interface that all providers must implement.
// Providers are stateless execution primitives that perform single, well-defined operations.
type Provider interface {
	// Descriptor returns the provider's metadata, schema, and capabilities.
	Descriptor() *Descriptor

	// Execute runs the provider logic with resolved inputs.
	// The input parameter is either:
	//   - map[string]any if Descriptor().Decode is nil
	//   - The decoded type if Descriptor().Decode is set and returns a typed struct
	// Resolver values can be accessed via ResolverContextFromContext(ctx).
	// Execution mode and dry-run flag are available via ExecutionModeFromContext(ctx) and DryRunFromContext(ctx).
	Execute(ctx context.Context, input any) (*Output, error)
}

// Descriptor contains provider identity, versioning, schemas, capabilities, and catalog metadata.
type Descriptor struct {
	// Name is the unique identifier for this provider. Must be lowercase with hyphens only.
	// Used to reference the provider in configurations and the registry.
	Name string `json:"name" yaml:"name" doc:"Unique provider identifier" minLength:"2" maxLength:"100" example:"http" pattern:"^[a-z][a-z0-9-]*$" required:"true"`

	// DisplayName is the human-readable name shown in UIs and documentation.
	// Optional - defaults to Name if not specified.
	DisplayName string `json:"displayName,omitempty" yaml:"displayName,omitempty" doc:"Human-readable display name" maxLength:"100" example:"HTTP Client"`

	// APIVersion indicates the provider API contract version (e.g., "v1").
	// Used for compatibility checking and migration support.
	APIVersion string `json:"apiVersion" yaml:"apiVersion" doc:"Provider API version" maxLength:"16" example:"v1" pattern:"^v[0-9]+$" required:"true"`

	// Version is the semantic version of this provider implementation.
	// Follows semver conventions for versioning provider releases.
	Version *semver.Version `json:"version" yaml:"version" doc:"Semantic version" required:"true"`

	// Description provides a concise explanation of what the provider does.
	// Displayed in catalogs, help text, and documentation.
	Description string `json:"description" yaml:"description" doc:"Provider description" minLength:"10" maxLength:"500" example:"Fetches data over HTTP" required:"true"`

	// Schema defines the structure and validation rules for provider inputs using JSON Schema.
	// Used for input validation, documentation generation, and UI form building.
	Schema *jsonschema.Schema `json:"schema" yaml:"schema" doc:"Input schema (JSON Schema)" required:"true"`

	// OutputSchemas defines the output structure for each supported capability using JSON Schema.
	// Each capability can produce different output shapes. Required for all declared capabilities.
	// Certain capabilities have required minimum fields:
	//   - validation: must include "valid" (boolean) and "errors" (array)
	//   - authentication: must include "authenticated" (boolean) and "token" (string)
	//   - action: must include "success" (boolean)
	//   - state: must include "success" (boolean)
	//   - kubeconfig: must include "success" (boolean)
	//   - from: no required fields
	//   - transform: no required fields
	OutputSchemas map[Capability]*jsonschema.Schema `json:"outputSchemas" yaml:"outputSchemas" doc:"Output schemas per capability (JSON Schema)" required:"true"`

	// SensitiveFields lists property names that contain sensitive data and should be redacted
	// in logs, errors, and snapshot output. Replaces the old per-property IsSecret flag.
	SensitiveFields []string `json:"sensitiveFields,omitempty" yaml:"sensitiveFields,omitempty" doc:"Property names containing sensitive data" maxItems:"50"`

	// Decode converts validated map[string]any inputs into strongly-typed structs for internal use.
	// Called after schema validation but before Execute(). Optional - providers can work with map[string]any directly.
	// When Decode is set, the Executor calls it and passes the result directly to Execute().
	Decode func(map[string]any) (any, error) `json:"-" yaml:"-"`

	// ExtractDependencies extracts resolver dependencies from the provider's inputs.
	// Called during dependency graph building to determine execution order.
	// Optional - if nil, the generic extraction logic is used (which handles common patterns like
	// CEL expressions with _.resolverName and Go templates with {{.resolverName}}).
	// Providers should implement this when they have custom input formats or need special handling
	// (e.g., go-template provider with custom delimiters).
	// The function receives the raw inputs map and returns a slice of resolver names that are referenced.
	ExtractDependencies func(inputs map[string]any) []string `json:"-" yaml:"-"`

	// WhatIf generates a human-readable description of what the provider would do
	// with the given inputs, without executing. Optional — if nil, falls back to
	// a generic message. In solution dry-run, receives the materialized inputs
	// map (map[string]any), not the decoded struct that Execute may receive.
	WhatIf func(ctx context.Context, input any) (string, error) `json:"-" yaml:"-"`

	// Capabilities declares the execution contexts this provider supports.
	// Determines where the provider can be used (from, transform, validation, etc.).
	Capabilities []Capability `json:"capabilities" yaml:"capabilities" doc:"Supported execution contexts" minItems:"1" maxItems:"10" required:"true"`

	// WriteOperations lists operation names that mutate external state.
	// When set, the executor rejects these operations in resolver (CapabilityFrom)
	// context to prevent unsafe side effects during DAG resolution, where retries
	// and parallelism can cause duplicate writes.
	//
	// Semantics:
	//   - nil: provider cannot classify operations (no enforcement occurs).
	//     Use this for providers like exec or http where the command/request
	//     determines read vs write behavior.
	//   - empty ([]string{}): all operations are reads (everything allowed in resolvers).
	//   - populated: listed operations are blocked in resolver context.
	//
	// Superseded by Operations: prefer declaring rich OperationDescriptor entries
	// with IsWrite set. WriteOperations is retained for backward compatibility and
	// may be removed in a future major version. Use EffectiveWriteOperations to
	// read the effective write set regardless of which field is populated.
	WriteOperations []string `json:"writeOperations" yaml:"writeOperations" doc:"Operation names that mutate state" maxItems:"200"`

	// Operations declares rich, per-operation metadata (descriptions, capabilities,
	// schemas, deprecation info) for providers that expose many named operations.
	// It is additive and optional: providers may continue to use WriteOperations
	// alone. When both are set they must be consistent (see ValidateDescriptor).
	// Operation-level Capabilities must be a subset of the provider's Capabilities.
	// Per-operation schemas are documentation/discovery metadata only; the
	// provider-level Schema remains the source of truth for runtime validation.
	Operations []OperationDescriptor `json:"operations,omitempty" yaml:"operations,omitempty" doc:"Per-operation metadata" maxItems:"500"`

	// Category classifies the provider for organization in catalogs and documentation.
	// Examples: "network", "storage", "security", "utility".
	Category string `json:"category,omitempty" yaml:"category,omitempty" doc:"Classification category" maxLength:"50" example:"network"`

	// Tags are searchable keywords for discovery and filtering.
	// Used in catalog searches and provider listings.
	Tags []string `json:"tags,omitempty" yaml:"tags,omitempty" doc:"Searchable keywords" maxItems:"20"`

	// Icon is a URL to an image representing the provider.
	// Displayed in UIs and documentation alongside the provider name.
	Icon string `json:"icon,omitempty" yaml:"icon,omitempty" doc:"Icon URL" format:"uri" maxLength:"500" example:"https://example.com/icon.svg"`

	// Links provides related resources such as documentation, source code, or tutorials.
	Links []Link `json:"links,omitempty" yaml:"links,omitempty" doc:"Related links" maxItems:"10"`

	// Examples contains sample configurations demonstrating provider usage.
	// Shown in documentation and can be used for testing.
	Examples []Example `json:"examples,omitempty" yaml:"examples,omitempty" doc:"Usage examples" maxItems:"10"`

	// IsDeprecated indicates the provider should no longer be used.
	// Deprecated providers may be removed in future versions.
	IsDeprecated bool `json:"deprecated,omitempty" yaml:"deprecated,omitempty" doc:"Deprecation status"`

	// Beta indicates the provider is experimental and may have breaking changes.
	// Beta providers are not recommended for production use.
	Beta bool `json:"beta,omitempty" yaml:"beta,omitempty" doc:"Beta status"`

	// Maintainers lists the people or teams responsible for this provider.
	// Used for contact and support information.
	Maintainers []Contact `json:"maintainers,omitempty" yaml:"maintainers,omitempty" doc:"Maintainer contacts" maxItems:"10"`
}

// Output is the standardized return structure for all provider executions.
type Output struct {
	Data     any            `json:"data" yaml:"data" doc:"Provider output data" required:"true"`
	Warnings []string       `json:"warnings,omitempty" yaml:"warnings,omitempty" doc:"Non-fatal warning messages" maxItems:"50"`
	Metadata map[string]any `json:"metadata,omitempty" yaml:"metadata,omitempty" doc:"Execution metadata"`
	// Streamed indicates that the provider already wrote its primary output
	// (e.g., stdout/stderr) directly to the terminal via IOStreams from context.
	// When true, the CLI output layer should not re-print the streamed content.
	Streamed bool `json:"streamed,omitempty" yaml:"streamed,omitempty" doc:"Whether output was already streamed to terminal"`
}

// DescribeWhatIf returns a human-readable description of what the provider would do.
// Calls WhatIf if set, falls back to a generic message.
func (d *Descriptor) DescribeWhatIf(ctx context.Context, input any) string {
	if d.WhatIf != nil {
		msg, err := d.WhatIf(ctx, input)
		if err != nil {
			lgr := logr.FromContextOrDiscard(ctx)
			lgr.V(1).Info("WhatIf function returned error, falling back to generic message",
				"provider", d.Name, "error", err)
		} else if msg != "" {
			return msg
		}
		// On error or empty result, fall through to generic message
	}
	return fmt.Sprintf("Would execute %s provider", d.Name)
}

// IsSensitiveField checks whether a field name is marked as sensitive in the descriptor.
func (d *Descriptor) IsSensitiveField(name string) bool {
	for _, f := range d.SensitiveFields {
		if f == name {
			return true
		}
	}
	return false
}

// IsWriteOperation reports whether the named operation is listed in WriteOperations.
// When WriteOperations is nil (provider cannot classify operations via the legacy
// field), it falls back to Operations, returning true if a matching operation has
// IsWrite set. Returns false when neither field classifies the operation.
func (d *Descriptor) IsWriteOperation(name string) bool {
	for _, op := range d.WriteOperations {
		if op == name {
			return true
		}
	}
	if d.WriteOperations == nil {
		if op := d.GetOperation(name); op != nil {
			return op.IsWrite
		}
	}
	return false
}

// EffectiveWriteOperations returns the names of operations that mutate external
// state. It returns a copy of WriteOperations when that field is non-nil
// (including an empty slice, which means "all operations are reads"). Otherwise
// it derives the list from Operations: a non-nil slice of the IsWrite operation
// names, or a non-nil empty slice when Operations is populated but classifies
// every operation as a read. Returns nil only when neither field is populated,
// preserving the documented nil ("cannot classify") versus empty ("all reads")
// distinction. The returned slice never aliases the descriptor's backing
// arrays, so callers may safely retain or mutate it.
func (d *Descriptor) EffectiveWriteOperations() []string {
	if d.WriteOperations != nil {
		out := make([]string, len(d.WriteOperations))
		copy(out, d.WriteOperations)
		return out
	}
	if len(d.Operations) == 0 {
		return nil
	}
	writes := make([]string, 0, len(d.Operations))
	for _, op := range d.Operations {
		if op.IsWrite {
			writes = append(writes, op.Name)
		}
	}
	return writes
}

// GetOperation returns the operation with the given name, or nil if not found.
func (d *Descriptor) GetOperation(name string) *OperationDescriptor {
	for i := range d.Operations {
		if d.Operations[i].Name == name {
			return &d.Operations[i]
		}
	}
	return nil
}

// OperationNames returns the names of all declared operations in declaration order.
func (d *Descriptor) OperationNames() []string {
	if len(d.Operations) == 0 {
		return nil
	}
	names := make([]string, len(d.Operations))
	for i, op := range d.Operations {
		names[i] = op.Name
	}
	return names
}

// Capability represents the types of operations a provider can perform.
type Capability string

const (
	CapabilityFrom           Capability = "from"
	CapabilityTransform      Capability = "transform"
	CapabilityValidation     Capability = "validation"
	CapabilityAuthentication Capability = "authentication"
	CapabilityAction         Capability = "action"
	CapabilityState          Capability = "state"
	CapabilityKubeconfig     Capability = "kubeconfig"
)

// IsValid checks if the capability is valid.
func (c Capability) IsValid() bool {
	switch c {
	case CapabilityFrom, CapabilityTransform, CapabilityValidation,
		CapabilityAuthentication, CapabilityAction, CapabilityState,
		CapabilityKubeconfig:
		return true
	default:
		return false
	}
}

// String returns the string representation.
func (c Capability) String() string {
	return string(c)
}

// OperationDescriptor describes a single named operation a provider exposes.
// It enriches the bare WriteOperations list with discovery metadata such as
// descriptions, per-operation capabilities, schemas, and deprecation info.
//
// Per-operation schemas are documentation/discovery metadata only and should
// contain operation-specific fields (excluding shared inputs like operation,
// owner, or repo). The provider-level Schema remains the source of truth for
// runtime input validation.
type OperationDescriptor struct {
	// Name is the unique operation identifier (e.g., "create_issue").
	Name string `json:"name" yaml:"name" doc:"Operation identifier" minLength:"1" maxLength:"100" required:"true"`

	// DisplayName is the human-readable operation name shown in UIs.
	DisplayName string `json:"displayName,omitempty" yaml:"displayName,omitempty" doc:"Human-readable operation name" maxLength:"100"`

	// Description explains what the operation does.
	Description string `json:"description,omitempty" yaml:"description,omitempty" doc:"Operation description" maxLength:"500"`

	// Capabilities lists the capabilities this operation supports.
	// Must be a subset of the provider's declared Capabilities.
	Capabilities []Capability `json:"capabilities,omitempty" yaml:"capabilities,omitempty" doc:"Operation capabilities (subset of provider capabilities)" maxItems:"10"`

	// IsWrite indicates the operation mutates external state.
	// Used to derive EffectiveWriteOperations when WriteOperations is unset.
	IsWrite bool `json:"isWrite,omitempty" yaml:"isWrite,omitempty" doc:"Whether the operation mutates external state"`

	// InputSchema documents operation-specific input fields (JSON Schema).
	// Documentation only; not used for runtime validation.
	InputSchema *jsonschema.Schema `json:"inputSchema,omitempty" yaml:"inputSchema,omitempty" doc:"Operation-specific input fields (JSON Schema)"`

	// OutputSchema documents the operation's output structure (JSON Schema).
	OutputSchema *jsonschema.Schema `json:"outputSchema,omitempty" yaml:"outputSchema,omitempty" doc:"Operation output (JSON Schema)"`

	// Examples contains sample usages for this operation.
	Examples []Example `json:"examples,omitempty" yaml:"examples,omitempty" doc:"Usage examples" maxItems:"10"`

	// Tags are searchable keywords for discovery and filtering.
	Tags []string `json:"tags,omitempty" yaml:"tags,omitempty" doc:"Searchable keywords" maxItems:"20"`

	// IsDeprecated indicates the operation should no longer be used.
	IsDeprecated bool `json:"deprecated,omitempty" yaml:"deprecated,omitempty" doc:"Deprecation status"`

	// DeprecationMessage provides guidance when the operation is deprecated.
	DeprecationMessage string `json:"deprecationMessage,omitempty" yaml:"deprecationMessage,omitempty" doc:"Deprecation guidance" maxLength:"300"`
}

// Contact represents maintainer contact information.
type Contact struct {
	Name  string `json:"name,omitempty" yaml:"name,omitempty" doc:"Maintainer name" maxLength:"60" example:"Jane Doe"`
	Email string `json:"email,omitempty" yaml:"email,omitempty" doc:"Maintainer email" format:"email" maxLength:"100"`
}

// Link represents a named hyperlink.
type Link struct {
	Name string `json:"name,omitempty" yaml:"name,omitempty" doc:"Link name" maxLength:"30" example:"Documentation"`
	URL  string `json:"url,omitempty" yaml:"url,omitempty" doc:"Link URL" format:"uri" maxLength:"500"`
}

// Example represents a usage example for a provider.
type Example struct {
	Name        string `json:"name,omitempty" yaml:"name,omitempty" doc:"Example name" maxLength:"50" example:"Basic usage"`
	Description string `json:"description,omitempty" yaml:"description,omitempty" doc:"Example description" maxLength:"300" example:"Basic HTTP GET request"`
	YAML        string `json:"yaml" yaml:"yaml" doc:"YAML example" minLength:"10" maxLength:"2000" required:"true"`
}

// getCapabilityRequiredFields returns the required output fields and expected JSON Schema types
// for the given capability.
func getCapabilityRequiredFields(capability Capability) map[string]string {
	switch capability {
	case CapabilityValidation:
		return map[string]string{
			"valid":  "boolean",
			"errors": "array",
		}
	case CapabilityAuthentication:
		return map[string]string{
			"authenticated": "boolean",
			"token":         "string",
		}
	case CapabilityAction, CapabilityState, CapabilityKubeconfig:
		return map[string]string{
			"success": "boolean",
		}
	case CapabilityFrom, CapabilityTransform:
		return nil
	}
	return nil
}

// ValidateDescriptor validates that a Descriptor meets all requirements.
// Returns an error if:
//   - OutputSchemas is missing for any declared capability
//   - Required fields are missing for capabilities that mandate them
//   - Field types don't match the expected JSON Schema types
func ValidateDescriptor(desc *Descriptor) error {
	if desc == nil {
		return errors.New("descriptor is nil")
	}

	if len(desc.Capabilities) == 0 {
		return errors.New("descriptor must declare at least one capability")
	}

	if desc.OutputSchemas == nil {
		return errors.New("descriptor must define OutputSchemas")
	}

	var errs []error

	for _, cap := range desc.Capabilities {
		if !cap.IsValid() {
			errs = append(errs, fmt.Errorf("unknown capability %q", cap))
			continue
		}

		schema, exists := desc.OutputSchemas[cap]
		if !exists {
			errs = append(errs, fmt.Errorf("missing output schema for capability %q", cap))
			continue
		}

		requiredFields := getCapabilityRequiredFields(cap)
		fieldNames := make([]string, 0, len(requiredFields))
		for fieldName := range requiredFields {
			fieldNames = append(fieldNames, fieldName)
		}
		sort.Strings(fieldNames)
		for _, fieldName := range fieldNames {
			expectedType := requiredFields[fieldName]
			if schema == nil || schema.Properties == nil {
				errs = append(errs, fmt.Errorf("capability %q requires output field %q", cap, fieldName))
				continue
			}
			prop, found := schema.Properties[fieldName]
			if !found || prop == nil {
				errs = append(errs, fmt.Errorf("capability %q requires output field %q", cap, fieldName))
				continue
			}
			if prop.Type != expectedType {
				errs = append(errs, fmt.Errorf("capability %q field %q must be type %q, got %q", cap, fieldName, expectedType, prop.Type))
			}
		}
	}

	seen := make(map[string]struct{}, len(desc.WriteOperations))
	for _, op := range desc.WriteOperations {
		if op == "" {
			errs = append(errs, errors.New("WriteOperations must not contain empty strings"))
			break
		}
		if _, dup := seen[op]; dup {
			errs = append(errs, fmt.Errorf("WriteOperations contains duplicate entry %q", op))
		}
		seen[op] = struct{}{}
	}

	errs = append(errs, validateOperations(desc)...)

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	return nil
}

// validateOperations checks the optional Operations field for structural and
// consistency problems:
//   - each operation must have a non-empty Name
//   - operation names must be unique
//   - operation capabilities must be valid and a subset of provider capabilities
//   - when both WriteOperations and Operations are set, every operation marked
//     IsWrite must also be listed in WriteOperations (so the legacy enforcement
//     field does not under-report writes), and every WriteOperations entry that
//     has a matching OperationDescriptor must be marked IsWrite (so discovery
//     metadata does not contradict the legacy enforcement list)
func validateOperations(desc *Descriptor) []error {
	if len(desc.Operations) == 0 {
		return nil
	}

	providerCaps := make(map[Capability]struct{}, len(desc.Capabilities))
	for _, c := range desc.Capabilities {
		providerCaps[c] = struct{}{}
	}

	var writeSet map[string]struct{}
	if desc.WriteOperations != nil {
		writeSet = make(map[string]struct{}, len(desc.WriteOperations))
		for _, op := range desc.WriteOperations {
			writeSet[op] = struct{}{}
		}
	}

	var errs []error
	seen := make(map[string]struct{}, len(desc.Operations))
	opIsWrite := make(map[string]bool, len(desc.Operations))
	for _, op := range desc.Operations {
		if op.Name == "" {
			errs = append(errs, errors.New("operations must not contain entries with empty names"))
			continue
		}
		if _, dup := seen[op.Name]; dup {
			errs = append(errs, fmt.Errorf("operations contains duplicate entry %q", op.Name))
		}
		seen[op.Name] = struct{}{}
		opIsWrite[op.Name] = op.IsWrite

		for _, c := range op.Capabilities {
			if !c.IsValid() {
				errs = append(errs, fmt.Errorf("operation %q declares unknown capability %q", op.Name, c))
				continue
			}
			if _, ok := providerCaps[c]; !ok {
				errs = append(errs, fmt.Errorf("operation %q declares capability %q not supported by the provider", op.Name, c))
			}
		}

		if writeSet != nil && op.IsWrite {
			if _, ok := writeSet[op.Name]; !ok {
				errs = append(errs, fmt.Errorf("operation %q is marked IsWrite but is missing from WriteOperations", op.Name))
			}
		}
	}

	// Reverse direction: a WriteOperations entry that has a matching operation
	// must be marked IsWrite, otherwise discovery metadata contradicts the
	// legacy enforcement list. Iterate the slice for deterministic ordering.
	if writeSet != nil {
		for _, name := range desc.WriteOperations {
			if isWrite, ok := opIsWrite[name]; ok && !isWrite {
				errs = append(errs, fmt.Errorf("operation %q is listed in WriteOperations but its descriptor is not marked IsWrite", name))
			}
		}
	}

	return errs
}

// DefaultOutputSchemas returns a minimal valid output schema map for the given
// capabilities, pre-populated with the required fields that ValidateDescriptor
// enforces. Providers may extend the returned schemas with additional properties.
//
// Capabilities that have no required fields (CapabilityFrom, CapabilityTransform)
// receive an empty object schema. Callers must add any provider-specific output
// fields on top of the returned map.
func DefaultOutputSchemas(caps ...Capability) map[Capability]*jsonschema.Schema {
	schemas := make(map[Capability]*jsonschema.Schema, len(caps))
	for _, c := range caps {
		schemas[c] = defaultSchemaForCapability(c)
	}
	return schemas
}

// defaultSchemaForCapability builds the minimal JSON Schema for a given
// capability, including only the fields that ValidateDescriptor requires.
func defaultSchemaForCapability(c Capability) *jsonschema.Schema {
	required := getCapabilityRequiredFields(c)
	props := make(map[string]*jsonschema.Schema, len(required))
	for field, typ := range required {
		props[field] = &jsonschema.Schema{Type: typ}
	}
	s := &jsonschema.Schema{
		Type:       "object",
		Properties: props,
	}
	return s
}
