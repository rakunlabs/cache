// Package gcpiam authenticates Redis connections with Google Cloud IAM, for
// Memorystore with IAM authentication.
package gcpiam

import (
	"context"
	"errors"
	"fmt"

	"cloud.google.com/go/auth/credentials"
	storeredis "github.com/rakunlabs/cache/store/redis"
)

const cloudPlatformScope = "https://www.googleapis.com/auth/cloud-platform"

// CredentialsProvider returns a provider for ConnConfig.CredentialsProvider
// that uses an access token from Application Default Credentials.
func CredentialsProvider() (func(ctx context.Context) (string, string, error), error) {
	creds, err := credentials.DetectDefault(&credentials.DetectOptions{
		Scopes: []string{cloudPlatformScope},
	})
	if err != nil {
		return nil, fmt.Errorf("gcp_iam credentials: %w", err)
	}

	// The auth library caches and refreshes the token, so it is not cached
	// again here; every new connection asks for the current one.
	return func(ctx context.Context) (string, string, error) {
		token, err := creds.Token(ctx)
		if err != nil {
			return "", "", fmt.Errorf("gcp_iam token: %w", err)
		}

		return "default", token.Value, nil
	}, nil
}

// Apply makes cfg authenticate with Google Cloud IAM. Static username and
// password are rejected since the token replaces them.
func Apply(cfg *storeredis.ConnConfig) error {
	if cfg.Username != "" || cfg.Password != "" {
		return errors.New("gcp_iam cannot be combined with username or password")
	}

	provider, err := CredentialsProvider()
	if err != nil {
		return err
	}

	cfg.CredentialsProvider = provider

	return nil
}
