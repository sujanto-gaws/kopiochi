package testsupport

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

// JSONRequest builds a request with a JSON body and the matching Content-Type.
// A nil body produces a request with no body at all, not an empty one — some
// handlers distinguish.
func JSONRequest(t *testing.T, method, target string, body any) *http.Request {
	t.Helper()

	var r io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		require.NoError(t, err)
		r = bytes.NewReader(encoded)
	}

	req := httptest.NewRequest(method, target, r)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req
}

// WithBearer attaches an Authorization header and returns the same request,
// so it composes: WithBearer(JSONRequest(...), tok).
func WithBearer(req *http.Request, token string) *http.Request {
	req.Header.Set("Authorization", "Bearer "+token)
	return req
}

// Do serves req against h and returns the recorder.
func Do(t *testing.T, h http.Handler, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// DecodeJSON unmarshals a recorded response body into v, failing the test with
// the raw body on error — without that, a handler returning an unexpected
// error page produces an opaque "invalid character '<'" and nothing to go on.
func DecodeJSON(t *testing.T, rec *httptest.ResponseRecorder, v any) {
	t.Helper()

	if err := json.Unmarshal(rec.Body.Bytes(), v); err != nil {
		t.Fatalf("response body is not valid JSON: %v\nstatus: %d\nbody: %s",
			err, rec.Code, rec.Body.String())
	}
}

// RequireStatus asserts the status code and, on mismatch, prints the body —
// which is where the reason almost always is.
func RequireStatus(t *testing.T, rec *httptest.ResponseRecorder, want int) {
	t.Helper()

	if rec.Code != want {
		t.Fatalf("status = %d, want %d\nbody: %s", rec.Code, want, rec.Body.String())
	}
}
