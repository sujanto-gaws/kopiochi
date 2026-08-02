package httpx

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
)

type stubPinger struct {
	err error
}

func (s stubPinger) Ping(ctx context.Context) error {
	return s.err
}

func TestHealthz_NeverChecksDependencies(t *testing.T) {
	r := chi.NewRouter()
	Mount(r, nil, Deps{Pinger: stubPinger{err: errors.New("db is down")}})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "liveness must not fail because of a broken dependency")
}

func TestReadyz_OkWhenDBReachable(t *testing.T) {
	r := chi.NewRouter()
	Mount(r, nil, Deps{Pinger: stubPinger{err: nil}})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
}

func TestReadyz_UnavailableWhenDBUnreachable(t *testing.T) {
	r := chi.NewRouter()
	Mount(r, nil, Deps{Pinger: stubPinger{err: errors.New("db is down")}})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

func TestReadyz_UnavailableWhenPingerNil(t *testing.T) {
	r := chi.NewRouter()
	Mount(r, nil, Deps{Pinger: nil})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusServiceUnavailable, rec.Code, "a missing pinger is a wiring bug and must fail closed")
}
