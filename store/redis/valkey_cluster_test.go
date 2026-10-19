package redis_test

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rakunlabs/cache"
	store "github.com/rakunlabs/cache/store/redis"
	"github.com/redis/go-redis/v9"
	"github.com/testcontainers/testcontainers-go"
	tcexec "github.com/testcontainers/testcontainers-go/exec"
)

const clusterNodes = 3

// startValkeyCluster runs a three-master Valkey Cluster in one container and
// returns the host-reachable node addresses. Nodes announce the Docker host and
// mapped ports, so the client can follow slot redirects from outside.
func startValkeyCluster(ctx context.Context, t *testing.T) []string {
	t.Helper()

	ports := make([]string, clusterNodes)
	for i := range ports {
		ports[i] = fmt.Sprintf("%d/tcp", 7000+i)
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "valkey/valkey:8.1.3-alpine",
			Entrypoint:   []string{"tail", "-f", "/dev/null"},
			ExposedPorts: ports,
		},
		Started: true,
	})
	if container != nil {
		t.Cleanup(func() {
			if err := testcontainers.TerminateContainer(container); err != nil {
				t.Errorf("terminate Valkey cluster: %v", err)
			}
		})
	}
	if err != nil {
		t.Fatalf("start Valkey cluster (Docker must be running): %v", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatal(err)
	}

	run := func(cmd ...string) string {
		t.Helper()
		code, out, err := execOutput(ctx, container, cmd...)
		if err != nil || code != 0 {
			t.Fatalf("exec %v: exit %d, %v: %s", cmd, code, err, out)
		}
		return out
	}

	addrs := make([]string, clusterNodes)
	internal := make([]string, clusterNodes)
	for i := range clusterNodes {
		port := 7000 + i
		mapped, err := container.MappedPort(ctx, ports[i])
		if err != nil {
			t.Fatal(err)
		}
		addrs[i] = fmt.Sprintf("%s:%s", host, mapped.Port())
		internal[i] = fmt.Sprintf("127.0.0.1:%d", port)

		run("valkey-server",
			"--port", strconv.Itoa(port),
			"--cluster-enabled", "yes",
			"--cluster-config-file", fmt.Sprintf("nodes-%d.conf", port),
			"--cluster-announce-ip", "127.0.0.1",
			"--cluster-announce-bus-port", strconv.Itoa(port+10000),
			"--cluster-announce-hostname", host,
			"--cluster-announce-port", mapped.Port(),
			"--cluster-preferred-endpoint-type", "hostname",
			"--save", "",
			"--appendonly", "no",
			"--daemonize", "yes",
		)
	}

	for i := range clusterNodes {
		waitFor(ctx, t, func() bool {
			code, _, err := container.Exec(ctx, []string{"valkey-cli", "-p", strconv.Itoa(7000 + i), "ping"})
			return err == nil && code == 0
		})
	}

	run(append(append([]string{"valkey-cli", "--cluster", "create"}, internal...), "--cluster-yes")...)

	for i := range clusterNodes {
		waitFor(ctx, t, func() bool {
			code, out, err := execOutput(ctx, container, "valkey-cli", "-p", strconv.Itoa(7000+i), "cluster", "info")
			return err == nil && code == 0 && strings.Contains(out, "cluster_state:ok")
		})
	}

	return addrs
}

func execOutput(ctx context.Context, container testcontainers.Container, cmd ...string) (int, string, error) {
	code, reader, err := container.Exec(ctx, cmd, tcexec.Multiplexed())
	if err != nil {
		return code, "", err
	}
	out, err := io.ReadAll(reader)

	return code, string(out), err
}

func waitFor(ctx context.Context, t *testing.T, ok func() bool) {
	t.Helper()
	deadline, stop := context.WithTimeout(ctx, 30*time.Second)
	defer stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for !ok() {
		select {
		case <-deadline.Done():
			t.Fatal("timed out waiting for Valkey cluster")
		case <-ticker.C:
		}
	}
}

func TestValkeyCluster(t *testing.T) {
	if testing.Short() {
		t.Skip("Valkey cluster integration test requires Docker")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()

	addrs := startValkeyCluster(ctx, t)

	newClient := func(t *testing.T, cfg store.ConnConfig) redis.UniversalClient {
		t.Helper()
		client, err := store.NewClient(ctx, cfg)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := client.Close(); err != nil {
				t.Errorf("close client: %v", err)
			}
		})
		if _, ok := client.(*redis.ClusterClient); !ok {
			t.Fatalf("client = %T, want *redis.ClusterClient", client)
		}
		return client
	}

	for _, tt := range []struct {
		name string
		cfg  store.ConnConfig
	}{
		{name: "detect-single-address", cfg: store.ConnConfig{Address: addrs[:1]}},
		{name: "forced-single-address", cfg: store.ConnConfig{Address: addrs[1:2], Cluster: true}},
		{name: "all-addresses", cfg: store.ConnConfig{Address: addrs}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := newClient(t, tt.cfg)
			c, err := cache.New(ctx, store.Store(client), cache.WithStoreConfig(store.Config{
				TTL:       time.Minute,
				KeyPrefix: tt.name + ":",
			}))
			if err != nil {
				t.Fatal(err)
			}

			// Enough keys to land on every master and force MOVED redirects.
			const keys = 64
			for i := range keys {
				if err := c.Set(ctx, strconv.Itoa(i), "value-"+strconv.Itoa(i)); err != nil {
					t.Fatal(err)
				}
			}

			cluster := client.(*redis.ClusterClient)
			// ForEachMaster runs the callback concurrently.
			var masters atomic.Int32
			err = cluster.ForEachMaster(ctx, func(ctx context.Context, node *redis.Client) error {
				n, err := node.Keys(ctx, tt.name+":*").Result()
				if err != nil {
					return err
				}
				if len(n) == 0 {
					return fmt.Errorf("master %s holds no keys", node.Options().Addr)
				}
				masters.Add(1)
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if got := masters.Load(); got != clusterNodes {
				t.Fatalf("masters = %d, want %d", got, clusterNodes)
			}

			for i := range keys {
				key, want := strconv.Itoa(i), "value-"+strconv.Itoa(i)
				if got, ok, err := c.Get(ctx, key); err != nil || !ok || got != want {
					t.Fatalf("Get(%s) = %q, %v, %v; want %q", key, got, ok, err, want)
				}
			}

			for i := range keys {
				key := strconv.Itoa(i)
				if i%2 == 0 {
					if got, ok, err := c.Take(ctx, key); err != nil || !ok || got != "value-"+key {
						t.Fatalf("Take(%s) = %q, %v, %v", key, got, ok, err)
					}
					if _, ok, err := c.Take(ctx, key); err != nil || ok {
						t.Fatalf("second Take(%s) = %v, %v; want miss", key, ok, err)
					}
				} else if err := c.Delete(ctx, key); err != nil {
					t.Fatal(err)
				}
				if _, ok, err := c.Get(ctx, key); err != nil || ok {
					t.Fatalf("Get(%s) after removal = %v, %v; want miss", key, ok, err)
				}
			}
		})
	}
}
