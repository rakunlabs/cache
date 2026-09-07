package cache

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type testStore struct {
	values sync.Map
	get    func(context.Context, string) (int, bool, error)
	setErr error
}

func (s *testStore) Get(ctx context.Context, key string) (int, bool, error) {
	if s.get != nil {
		return s.get(ctx, key)
	}
	v, ok := s.values.Load(key)
	if !ok {
		return 0, false, nil
	}
	return v.(int), true, nil
}

func (s *testStore) Set(_ context.Context, key string, value int) error {
	if s.setErr != nil {
		return s.setErr
	}
	s.values.Store(key, value)
	return nil
}

func (s *testStore) Delete(_ context.Context, key string) error {
	s.values.Delete(key)
	return nil
}

func TestGetSetSameKey(t *testing.T) {
	c := &Cache[string, int]{Cacher: &testStore{}}
	var calls atomic.Int32
	var wg sync.WaitGroup
	for range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, err := c.GetSet(t.Context(), "key", func() (int, error) {
				calls.Add(1)
				return 42, nil
			})
			if err != nil || v != 42 {
				t.Errorf("GetSet = %d, %v", v, err)
			}
		}()
	}
	wg.Wait()
	if calls.Load() != 1 || len(c.loading) != 0 {
		t.Fatalf("calls = %d, pending keys = %d", calls.Load(), len(c.loading))
	}
}

func TestGetSetDifferentKeysAndCancellation(t *testing.T) {
	s := &testStore{}
	c := &Cache[string, int]{Cacher: s}
	started, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	defer func() { close(release); <-finished }()
	go func() {
		defer close(finished)
		_, err := c.GetSet(t.Context(), "slow", func() (int, error) {
			close(started)
			<-release
			return 42, nil
		})
		if err != nil {
			t.Errorf("slow GetSet: %v", err)
		}
	}()
	<-started

	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	v, err := c.GetSet(ctx, "fast", func() (int, error) {
		return c.GetSet(ctx, "nested", func() (int, error) { return 7, nil })
	})
	if err != nil || v != 7 {
		t.Fatalf("independent/nested GetSet = %d, %v", v, err)
	}

	// The owner is blocked in fn, so replacing the read hook here is safe.
	read := make(chan struct{})
	s.get = func(context.Context, string) (int, bool, error) {
		close(read)
		return 0, false, nil
	}
	waitCtx, stopWait := context.WithCancel(t.Context())
	defer stopWait()
	result := make(chan error, 1)
	go func() {
		_, err := c.GetSet(waitCtx, "slow", func() (int, error) {
			t.Error("canceled waiter ran fn")
			return 0, nil
		})
		result <- err
	}()
	<-read
	stopWait()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("wait error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled waiter remained blocked")
	}
}

func TestGetSetErrorsReleaseKey(t *testing.T) {
	want := errors.New("store/load failure")
	for _, stage := range []string{"get", "recheck", "load", "set", "cancel"} {
		t.Run(stage, func(t *testing.T) {
			s := &testStore{}
			c := &Cache[string, int]{Cacher: s}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			fn := func() (int, error) { return 42, nil }
			expected := want
			switch stage {
			case "get", "recheck":
				calls := 0
				s.get = func(context.Context, string) (int, bool, error) {
					calls++
					if stage == "get" || calls == 2 {
						return 0, false, want
					}
					return 0, false, nil
				}
			case "load":
				fn = func() (int, error) { return 0, want }
			case "set":
				s.setErr = want
			case "cancel":
				expected = context.Canceled
				fn = func() (int, error) { cancel(); return 42, nil }
			}
			if _, err := c.GetSet(ctx, "key", fn); !errors.Is(err, expected) {
				t.Fatalf("error = %v, want %v", err, expected)
			}
			if len(c.loading) != 0 {
				t.Fatal("failed load retained key reservation")
			}
			s.get, s.setErr = nil, nil
			v, err := c.GetSet(t.Context(), "key", func() (int, error) { return 7, nil })
			if err != nil || v != 7 {
				t.Fatalf("retry = %d, %v", v, err)
			}
		})
	}
}

func TestGetSetPanicReleasesKey(t *testing.T) {
	c := &Cache[string, int]{Cacher: &testStore{}}
	func() {
		defer func() {
			if recover() == nil {
				t.Error("expected callback panic")
			}
		}()
		_, _ = c.GetSet(t.Context(), "key", func() (int, error) { panic("load") })
	}()
	if len(c.loading) != 0 {
		t.Fatal("panic retained key reservation")
	}
}

func TestGetSetCanceledContext(t *testing.T) {
	c := &Cache[string, int]{Cacher: &testStore{}}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := c.GetSet(ctx, "key", func() (int, error) {
		t.Fatal("callback ran with canceled context")
		return 0, nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v", err)
	}
}

func TestGetSetRechecksBeforeLoad(t *testing.T) {
	for _, cancelOnRead := range []bool{false, true} {
		t.Run(map[bool]string{false: "hit", true: "canceled"}[cancelOnRead], func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			calls := 0
			s := &testStore{get: func(context.Context, string) (int, bool, error) {
				calls++
				if calls == 1 {
					if cancelOnRead {
						cancel()
					}
					return 0, false, nil
				}
				return 42, true, nil
			}}
			c := &Cache[string, int]{Cacher: s}
			v, err := c.GetSet(ctx, "key", func() (int, error) {
				t.Fatal("callback should not run")
				return 0, nil
			})
			if cancelOnRead {
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("error = %v", err)
				}
			} else if err != nil || v != 42 {
				t.Fatalf("GetSet = %d, %v", v, err)
			}
			if len(c.loading) != 0 {
				t.Fatal("recheck retained key reservation")
			}
		})
	}
}

type closingStore struct {
	testStore
	err error
}

func (s *closingStore) Close() error { return s.err }

func TestNewAndClose(t *testing.T) {
	if _, err := New[string, int, int](t.Context(), nil); !errors.Is(err, ErrStoreNotExist) {
		t.Fatalf("nil store error = %v", err)
	}
	want := errors.New("store error")
	_, err := New(t.Context(), func(context.Context, int) (Cacher[string, int], error) {
		return nil, want
	})
	if !errors.Is(err, want) {
		t.Fatalf("constructor error = %v", err)
	}
	c, err := New(t.Context(), func(_ context.Context, cfg int) (Cacher[string, int], error) {
		if cfg != 42 {
			t.Errorf("config = %d", cfg)
		}
		return &closingStore{err: want}, nil
	}, WithStoreConfig(42))
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); !errors.Is(err, want) {
		t.Fatalf("Close error = %v", err)
	}
	c.Cacher = &testStore{}
	if err := c.Close(); err != nil {
		t.Fatalf("non-closing store: %v", err)
	}
}
