package cache

import "time"

type option struct {
	MaxItems int
	TTL      time.Duration

	TypeStorage Type
}

type Option func(*option)

// WithMaxItems sets the maximum number of items the cache can hold.
func WithMaxItems(maxItems int) Option {
	return func(o *option) {
		o.MaxItems = maxItems
	}
}

// WithTTL sets the time to live for each item in the cache.
func WithTTL(ttl time.Duration) Option {
	return func(o *option) {
		o.TTL = ttl
	}
}

// WithTypeStorage sets the type of cache to use.
func WithTypeStorage(t Type) Option {
	return func(o *option) {
		o.TypeStorage = t
	}
}
