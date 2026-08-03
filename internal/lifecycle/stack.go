// Package lifecycle owns process teardown: one registration point for every
// resource that must be released, released in the reverse of the order it was
// created.
//
// See docs/architectures/02-composition/lifecycle-and-shutdown.md. The
// arrangement this replaces closed each resource twice — once from a defer in
// main and once from a shutdown func registered on the server — and got away
// with it only because pgxpool.Close happened to be idempotent. Ownership was
// genuinely ambiguous, and the next resource added would not have been so
// forgiving.
package lifecycle

import (
	"context"
	"errors"
	"fmt"

	"github.com/rs/zerolog"
)

// Stack is a LIFO stack of closers.
//
// It is not safe for concurrent use, and deliberately so: everything that
// pushes onto it runs on the single startup path, and Shutdown runs once
// after that path has finished. A mutex here would suggest a concurrency this
// type does not have.
type Stack struct {
	entries []entry
	log     zerolog.Logger
}

type entry struct {
	name  string
	close func(context.Context) error
}

// New returns an empty stack that logs each resource as it is released.
func New(log zerolog.Logger) *Stack {
	return &Stack{log: log}
}

// Push registers a closer under a human-readable name. The name appears in
// shutdown logs and in any error, so it should say what the resource is
// ("database", "http server"), not what the function does.
//
// A resource is pushed exactly once, by whoever constructed it. Nothing on
// this stack should also have a `defer x.Close()` in main.
func (s *Stack) Push(name string, fn func(context.Context) error) {
	s.entries = append(s.entries, entry{name: name, close: fn})
}

// PushCloser adapts the common `func() error` shape (io.Closer-like, no
// context) onto the stack, so callers don't each write the same wrapper.
func (s *Stack) PushCloser(name string, fn func() error) {
	s.Push(name, func(context.Context) error { return fn() })
}

// Len reports how many resources are registered. Used by tests and by the
// shutdown log line.
func (s *Stack) Len() int { return len(s.entries) }

// Shutdown releases every registered resource in reverse registration order
// and returns all failures joined together.
//
// It keeps going after a failure rather than returning early: a resource that
// fails to close cleanly is not a reason to leak the ones registered before
// it, which are exactly the ones the failing resource was built on top of.
//
// ctx bounds the whole sequence; individual closers are expected to respect
// it. Shutdown does not abandon a closer that ignores its context — killing a
// drain mid-flight would defeat the point — so the process-level guarantee is
// the second-signal force-exit in cmd/api, not this function.
func (s *Stack) Shutdown(ctx context.Context) error {
	var errs []error

	for i := len(s.entries) - 1; i >= 0; i-- {
		e := s.entries[i]
		s.log.Info().Str("resource", e.name).Msg("closing")

		if err := e.close(ctx); err != nil {
			s.log.Error().Err(err).Str("resource", e.name).Msg("close failed")
			errs = append(errs, fmt.Errorf("%s: %w", e.name, err))
			continue
		}
		s.log.Debug().Str("resource", e.name).Msg("closed")
	}

	return errors.Join(errs...)
}
