module github.com/rakunlabs/cache/store/redis/gcpiam

go 1.26.0

replace (
	github.com/rakunlabs/cache => ../../..
	github.com/rakunlabs/cache/store/redis => ..
)

require (
	cloud.google.com/go/auth v0.24.1-0.20261001053825-dbc26066f70a
	github.com/rakunlabs/cache/store/redis v0.5.2
)

require (
	cloud.google.com/go/compute/metadata v0.9.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.3.17 // indirect
	github.com/googleapis/gax-go/v2 v2.26.2 // indirect
	github.com/rakunlabs/cache v0.5.0 // indirect
	github.com/redis/go-redis/v9 v9.22.0 // indirect
	github.com/twmb/tlscfg v1.3.0 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
)
