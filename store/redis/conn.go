package redis

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"strings"

	"github.com/redis/go-redis/v9"
	"github.com/twmb/tlscfg"
)

// ConnConfig describes how to connect to a standalone Redis or a Redis Cluster.
type ConnConfig struct {
	// Address lists the server addresses. More than one address always selects
	// Redis Cluster mode.
	Address    []string `cfg:"address"     json:"address"`
	Username   string   `cfg:"username"    json:"username"`
	Password   string   `cfg:"password"    json:"-"           log:"-"`
	ClientName string   `cfg:"client_name" json:"client_name"`
	// Cluster forces Redis Cluster mode, including for a single address. When
	// false and only one address is set, cluster mode is detected with INFO.
	Cluster bool      `cfg:"cluster" json:"cluster"`
	TLS     TLSConfig `cfg:"tls"     json:"tls"`
}

// NewClient returns a connected client for cfg. The caller owns the client and
// must close it.
//
// With a single address and Cluster unset, the server is asked whether cluster
// mode is enabled. If INFO is not permitted (for example by an ACL), a
// standalone client is used.
func NewClient(ctx context.Context, cfg ConnConfig) (redis.UniversalClient, error) {
	if len(cfg.Address) == 0 {
		return nil, errors.New("redis address is required")
	}
	for _, addr := range cfg.Address {
		if strings.TrimSpace(addr) == "" {
			return nil, errors.New("redis address must not be empty")
		}
	}

	tlsConfig, err := cfg.TLS.Generate()
	if err != nil {
		return nil, fmt.Errorf("redis tls config: %w", err)
	}

	client := newClient(ctx, cfg, tlsConfig)
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()

		return nil, fmt.Errorf("redis ping: %w", err)
	}

	return client, nil
}

func newClient(ctx context.Context, cfg ConnConfig, tlsConfig *tls.Config) redis.UniversalClient {
	if cfg.Cluster || len(cfg.Address) > 1 {
		return newClusterClient(cfg, tlsConfig)
	}

	client := redis.NewClient(&redis.Options{
		Addr:       cfg.Address[0],
		Username:   cfg.Username,
		Password:   cfg.Password,
		ClientName: cfg.ClientName,
		TLSConfig:  tlsConfig,
	})

	info, err := client.Info(ctx, "cluster").Result()
	if err != nil || !clusterEnabled(info) {
		return client
	}

	_ = client.Close()

	return newClusterClient(cfg, tlsConfig)
}

func newClusterClient(cfg ConnConfig, tlsConfig *tls.Config) redis.UniversalClient {
	return redis.NewClusterClient(&redis.ClusterOptions{
		Addrs:      cfg.Address,
		Username:   cfg.Username,
		Password:   cfg.Password,
		ClientName: cfg.ClientName,
		TLSConfig:  tlsConfig,
	})
}

func clusterEnabled(info string) bool {
	for line := range strings.Lines(info) {
		if strings.TrimSpace(line) == "cluster_enabled:1" {
			return true
		}
	}

	return false
}

// TLSConfig contains options for TLS connections.
type TLSConfig struct {
	Enabled bool `cfg:"enabled" json:"enabled"`
	// CertFile is the client certificate; use it together with KeyFile.
	CertFile string `cfg:"cert_file" json:"cert_file"`
	KeyFile  string `cfg:"key_file"  json:"key_file"`
	// CAFile is added to the system root CAs.
	CAFile string `cfg:"ca_file" json:"ca_file"`
}

// Generate returns nil when TLS is disabled.
func (t TLSConfig) Generate() (*tls.Config, error) {
	if !t.Enabled {
		return nil, nil
	}

	opts := []tlscfg.Opt{tlscfg.WithSystemCertPool()}
	if t.CertFile != "" && t.KeyFile != "" {
		opts = append(opts, tlscfg.WithDiskKeyPair(t.CertFile, t.KeyFile))
	}
	if t.CAFile != "" {
		opts = append(opts, tlscfg.WithDiskCA(t.CAFile, tlscfg.ForClient))
	}

	return tlscfg.New(opts...)
}
