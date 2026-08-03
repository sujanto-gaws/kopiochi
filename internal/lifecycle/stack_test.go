package lifecycle

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

func newTestStack() *Stack { return New(zerolog.Nop()) }

// TestShutdown_ReleasesInReverseOrder is the whole point of the type. A
// resource is built on top of the ones registered before it — the router on
// the database, the server on the router — so releasing in registration order
// would tear out the foundation while something is still standing on it.
func TestShutdown_ReleasesInReverseOrder(t *testing.T) {
	var order []string
	s := newTestStack()

	s.PushCloser("first", func() error { order = append(order, "first"); return nil })
	s.PushCloser("second", func() error { order = append(order, "second"); return nil })
	s.PushCloser("third", func() error { order = append(order, "third"); return nil })

	if err := s.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() = %v, want nil", err)
	}

	want := []string{"third", "second", "first"}
	if len(order) != len(want) {
		t.Fatalf("closed %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("closed %v, want %v (teardown must be strict LIFO)", order, want)
		}
	}
}

// TestShutdown_ContinuesAfterAFailure guards the failure mode that would make
// this type worse than the defers it replaced: a closer that errors must not
// strand the resources registered beneath it, which are exactly the ones it
// was built on.
func TestShutdown_ContinuesAfterAFailure(t *testing.T) {
	var closed []string
	s := newTestStack()

	s.PushCloser("database", func() error { closed = append(closed, "database"); return nil })
	s.PushCloser("broken", func() error { return errors.New("boom") })
	s.PushCloser("server", func() error { closed = append(closed, "server"); return nil })

	err := s.Shutdown(context.Background())
	if err == nil {
		t.Fatal("Shutdown() = nil, want the failing closer's error")
	}
	if !strings.Contains(err.Error(), "broken") {
		t.Errorf("error = %v, want it to name the failing resource", err)
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("error = %v, want it to wrap the underlying cause", err)
	}

	if len(closed) != 2 || closed[0] != "server" || closed[1] != "database" {
		t.Errorf("closed %v, want [server database]: a failure must not stop the teardown", closed)
	}
}

// TestShutdown_JoinsEveryFailure proves failures accumulate rather than the
// first one masking the rest — with several resources down, an operator needs
// all of the reasons, not the last one to be pushed.
func TestShutdown_JoinsEveryFailure(t *testing.T) {
	s := newTestStack()
	s.PushCloser("alpha", func() error { return errors.New("alpha failed") })
	s.PushCloser("beta", func() error { return errors.New("beta failed") })

	err := s.Shutdown(context.Background())
	if err == nil {
		t.Fatal("Shutdown() = nil, want both errors")
	}
	for _, want := range []string{"alpha failed", "beta failed"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %v, want it to contain %q", err, want)
		}
	}
}

// TestShutdown_PassesContextToClosers proves the shutdown deadline actually
// reaches the closers, which is what lets a drain respect it.
func TestShutdown_PassesContextToClosers(t *testing.T) {
	type ctxKey string
	const key ctxKey = "marker"

	var got any
	s := newTestStack()
	s.Push("observer", func(ctx context.Context) error {
		got = ctx.Value(key)
		return nil
	})

	ctx := context.WithValue(context.Background(), key, "present")
	if err := s.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown() = %v", err)
	}
	if got != "present" {
		t.Errorf("closer received context value %v, want %q", got, "present")
	}
}

// TestShutdown_EmptyStackIsNotAnError covers the early-failure path: a boot
// that fails before anything is registered still calls Shutdown.
func TestShutdown_EmptyStackIsNotAnError(t *testing.T) {
	s := newTestStack()
	if s.Len() != 0 {
		t.Fatalf("Len() = %d, want 0", s.Len())
	}
	if err := s.Shutdown(context.Background()); err != nil {
		t.Errorf("Shutdown() on an empty stack = %v, want nil", err)
	}
}
