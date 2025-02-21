package redis

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

type Client interface {
	Close() error
	Get(ctx context.Context, key string) *redis.StringCmd
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd
	Del(ctx context.Context, keys ...string) *redis.IntCmd
}

type Connection struct {
	ClientName string   `cfg:"client_name" json:"client_name"`
	Addr       []string `cfg:"addr"        json:"addr"`
	UserName   string   `cfg:"username"    json:"username"`
	Password   string   `cfg:"password"    json:"password"`

	TLS TLSConfig `cfg:"tls" json:"tls"`
}

type Redis struct {
	Client
}

func New(cfg Connection) (*Redis, error) {
	r := Redis{}

	tlsConfig, err := cfg.TLS.Generate()
	if err != nil {
		return nil, err
	}

	if len(cfg.Addr) > 1 {
		r.Client = redis.NewClusterClient(&redis.ClusterOptions{
			Addrs:      cfg.Addr,
			Username:   cfg.UserName,
			Password:   cfg.Password,
			ClientName: cfg.ClientName,
			TLSConfig:  tlsConfig,
		})
	} else {
		r.Client = redis.NewClient(&redis.Options{
			Addr:       cfg.Addr[0],
			Username:   cfg.UserName,
			Password:   cfg.Password,
			ClientName: cfg.ClientName,
			TLSConfig:  tlsConfig,
		})
	}

	return &r, nil
}
