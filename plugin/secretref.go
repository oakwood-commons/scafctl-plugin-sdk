// Copyright 2025-2026 Oakwood Commons
// SPDX-License-Identifier: Apache-2.0

package plugin

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
)

// SecretRef is a string that references a secret value via a scheme prefix.
// Supported schemes:
//   - env://VAR_NAME    -- reads the value from environment variable VAR_NAME.
//   - file:///path      -- reads and trims the contents of the file at /path.
//   - secret://NAME     -- reads the named secret from the host secret store via ResolveWithHost.
type SecretRef string

const (
	secretRefSchemeEnv    = "env://"
	secretRefSchemeFile   = "file://"
	secretRefSchemeSecret = "secret://"
	reservedNamePrefix    = "scafctl."
	maxSecretNameLength   = 255
)

// SecretGetter retrieves a named secret from a host or backing secret store.
// *HostServiceClient satisfies SecretGetter directly.
type SecretGetter interface {
	GetSecret(ctx context.Context, name string) (string, bool, error)
}

var _ SecretGetter = (*HostServiceClient)(nil)

// Validate checks that the SecretRef has a recognized scheme and a valid target.
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
	case strings.HasPrefix(str, secretRefSchemeSecret):
		return validateSecretName(strings.TrimPrefix(str, secretRefSchemeSecret))
	default:
		return fmt.Errorf("unsupported secret ref scheme %q, must start with env://, file://, or secret://", str)
	}
	return nil
}

// validateSecretName enforces the scafctl host secret-store naming rules plus
// the SDK's reserved "scafctl." prefix guard:
//   - Length 1..255 bytes
//   - Must not start with '.' or '-'
//   - Must not contain ".."
//   - Must match ^[a-zA-Z0-9][a-zA-Z0-9._-]{0,254}$
//   - Must not start with "scafctl."
func validateSecretName(name string) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("secret:// scheme requires a secret name")
	}
	if len(name) > maxSecretNameLength {
		return fmt.Errorf("secret:// name exceeds maximum length of %d bytes", maxSecretNameLength)
	}
	if name[0] == '.' || name[0] == '-' {
		return fmt.Errorf("secret:// name %q must not start with '.' or '-'", name)
	}
	if strings.Contains(name, "..") {
		return fmt.Errorf("secret:// name %q must not contain '..'", name)
	}
	if !isValidSecretNameChars(name) {
		return fmt.Errorf("secret:// name %q contains invalid characters; must match ^[a-zA-Z0-9][a-zA-Z0-9._-]{0,254}$", name)
	}
	if strings.HasPrefix(name, reservedNamePrefix) {
		return fmt.Errorf("secret:// name %q must not start with reserved prefix %q", name, reservedNamePrefix)
	}
	return nil
}

func isValidSecretNameChars(name string) bool {
	if len(name) == 0 || !isASCIIAlnum(name[0]) {
		return false
	}
	for i := 1; i < len(name); i++ {
		b := name[i]
		if !isASCIIAlnum(b) && b != '.' && b != '_' && b != '-' {
			return false
		}
	}
	return true
}

func isASCIIAlnum(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

// Resolve reads the secret value from a local source (env:// or file://).
// For secret:// references, callers must use ResolveWithHost instead.
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
	case strings.HasPrefix(str, secretRefSchemeSecret):
		return "", errors.New("secret:// scheme requires ResolveWithHost(ctx, hostClient)")
	default:
		return "", fmt.Errorf("unsupported secret ref scheme %q", str)
	}
}

// ResolveWithHost reads the secret value from env://, file://, or secret://
// (using getter to query the host secret store for secret:// references).
func (s SecretRef) ResolveWithHost(ctx context.Context, getter SecretGetter) (string, error) {
	if err := s.Validate(); err != nil {
		return "", err
	}
	str := string(s)
	if !strings.HasPrefix(str, secretRefSchemeSecret) {
		return s.Resolve()
	}
	name := strings.TrimPrefix(str, secretRefSchemeSecret)
	if isNilSecretGetter(getter) {
		return "", fmt.Errorf("resolving secret %q: host secret getter is nil", name)
	}
	val, found, err := getter.GetSecret(ctx, name)
	if err != nil {
		return "", fmt.Errorf("resolving secret %q: %w", name, err)
	}
	if !found {
		return "", fmt.Errorf("secret %q not found in host secret store", name)
	}
	if val == "" {
		return "", fmt.Errorf("secret %q is empty", name)
	}
	return val, nil
}

func isNilSecretGetter(getter SecretGetter) bool {
	if getter == nil {
		return true
	}
	rv := reflect.ValueOf(getter)
	k := rv.Kind()
	if k == reflect.Chan || k == reflect.Func || k == reflect.Interface || k == reflect.Map || k == reflect.Pointer || k == reflect.Slice {
		return rv.IsNil()
	}
	return false
}

// Scheme returns the scheme portion of the secret reference (e.g. "env", "file", "secret").
func (s SecretRef) Scheme() string {
	str := string(s)
	if idx := strings.Index(str, "://"); idx >= 0 {
		return str[:idx]
	}
	return ""
}
