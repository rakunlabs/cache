package cache

import "time"

type Config struct {
	// The maximum number of items the cache can hold.
	MaxItems int `cfg:"max_items" json:"max_items"`
	// TTL is the time to live for each item in the cache.
	TTL time.Duration `cfg:"ttl" json:"ttl"`
}

func (c *Config) SetDefault(maxItems int, ttl time.Duration) {
	if c.MaxItems == 0 {
		c.MaxItems = maxItems
	}

	if c.TTL == 0 {
		c.TTL = ttl
	}
}

func (c *Config) ToOption() Option {
	return func(o *option) {
		o.MaxItems = c.MaxItems
		o.TTL = c.TTL
	}
}
