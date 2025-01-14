package cache

import "time"

type option struct {
	Config map[string]interface{}
}

type Option func(*option)

// WithMaxItems sets the maximum number of items the cache can hold.
func WithMaxItems(maxItems int) Option {
	return func(o *option) {
		o.Config["max_items"] = maxItems
	}
}

// WithTTL sets the time to live for each item in the cache.
func WithTTL(ttl time.Duration) Option {
	return func(o *option) {
		o.Config["ttl"] = ttl
	}
}

func WithStoreConfig(v any) Option {
	return func(o *option) {
		o.Config["store"] = v
	}
}
