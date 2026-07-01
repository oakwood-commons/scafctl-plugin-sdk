// Copyright 2025-2026 Oakwood Commons
// SPDX-License-Identifier: Apache-2.0

package plugin

import (
	"fmt"
	"os"
	"strings"
)

// SecretRef is a string that references a secret value via a scheme prefix.
// Supported schemes:
//   - env://VAR_NAME  — reads the value from environment variable VAR_NAME.
//   - file:///path    — reads and trims the contents of the file at /path.
type SecretRef string

const (
	secretRefSchemeEnv  = "env://"
	secretRefSchemeFile = "file://"
)

// Validate checks that the SecretRef has a recognized scheme and a non-empty value.
func (s SecretRef) Validate() error {
	str := string(s)
	switch {
	case strings.HasPrefix(str, secretRefSchemeEnv):
		if strings.TrimPrefix(str, secretRefSchemeEnv) == "" {
			return fmt.Errorf("env:// scheme requires a variable name")
		}
	case strings.HasPrefix(str, secretRefSchemeFile):
		if strings.TrimPrefix(str, secretRefSchemeFile) == "" {
			return fmt.Errorf("file:// scheme requires a path")
		}
	default:
		return fmt.Errorf("unsupported secret ref scheme %q, must start with env:// or file://", str)
	}
	return nil
}

// Resolve reads the secret value from the referenced source.
// For env:// it reads the environment variable; for file:// it reads and trims the file.
func (s SecretRef) Resolve() (string, error) {
	if err := s.Validate(); err != nil {
		return "", err
	}
	str := string(s)
	switch {
	case strings.HasPrefix(str, secretRefSchemeEnv):
		name := strings.TrimPrefix(str, secretRefSchemeEnv)
		val := os.Getenv(name)
		if val == "" {
			return "", fmt.Errorf("environment variable %q is empty or not set", name)
		}
		return val, nil
	case strings.HasPrefix(str, secretRefSchemeFile):
		path := strings.TrimPrefix(str, secretRefSchemeFile)
		data, err := os.ReadFile(path) //nolint:gosec // file:// scheme requires reading user-specified paths
		if err != nil {
			return "", fmt.Errorf("reading secret file %q: %w", path, err)
		}
		val := strings.TrimSpace(string(data))
		if val == "" {
			return "", fmt.Errorf("secret file %q is empty", path)
		}
		return val, nil
	default:
		return "", fmt.Errorf("unsupported secret ref scheme %q", str)
	}
}

// Scheme returns the scheme portion of the secret reference (e.g. "env", "file").
func (s SecretRef) Scheme() string {
	str := string(s)
	if idx := strings.Index(str, "://"); idx >= 0 {
		return str[:idx]
	}
	return ""
}
