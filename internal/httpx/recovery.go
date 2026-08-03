package httpx

import (
	"fmt"
	"net/http"
	"runtime/debug"

	"github.com/rs/zerolog"
)

// Recovery turns a panic in any downstream handler into a logged 500 and an
// RFC 7807 response.
//
// It replaces chi's middleware.Recoverer, which writes a bare 500 with an
// empty body and prints a stack trace to os.Stderr in plain text. That trace
// is not JSON, carries no request_id, and is invisible to a log pipeline that
// reads structured records — so in production a panic showed up as a 500 with
// nothing to correlate it against. See
// docs/architectures/06-quality/observability.md, Problem 3.
//
// Two properties matter and are tested:
//
//   - The client is told nothing. The panic value and the stack go to the log;
//     the response body is a fixed string. A panic message routinely contains
//     a query, a path, or a struct dump.
//
//   - If the handler already wrote a status, no second write is attempted.
//     Calling WriteHeader twice logs "superfluous WriteHeader" and corrupts
//     the response, so a panic *after* a partial write is logged and the
//     connection is left to break, which is the honest signal to the client.
func Recovery(log zerolog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ww := newStatusRecorder(w)

			defer func() {
				rec := recover()
				if rec == nil {
					return
				}

				// http.ErrAbortHandler is the documented way for a handler to
				// abandon a response without it being an error — httputil's
				// reverse proxy uses it. net/http suppresses its own logging
				// for it, and swallowing it here would turn an intentional
				// abort into a spurious 500.
				if rec == http.ErrAbortHandler {
					panic(rec)
				}

				// zerolog.Ctx picks up the request-scoped logger installed by
				// middleware.RequestLogger, so this line already carries
				// request_id and client_ip. It falls back to the logger passed
				// in if Recovery is mounted without it.
				reqLog := zerolog.Ctx(r.Context())
				if reqLog.GetLevel() == zerolog.Disabled {
					reqLog = &log
				}

				reqLog.Error().
					Str("panic", fmt.Sprint(rec)).
					Str("stack", string(debug.Stack())).
					Str("method", r.Method).
					Str("path", r.URL.Path).
					Msg("panic recovered")

				if ww.wrote {
					// Response already committed — see the doc comment.
					return
				}

				WriteProblem(ww, r, http.StatusInternalServerError,
					"internal_error", "Internal Server Error",
					"An unexpected error occurred.")
			}()

			next.ServeHTTP(ww, r)
		})
	}
}

// statusRecorder records only whether a status has been written. It is
// deliberately not chi's WrapResponseWriter: this one is mounted outermost and
// wrapping again would hide the inner wrapper's accounting from the request
// logger.
type statusRecorder struct {
	http.ResponseWriter
	wrote bool
}

func newStatusRecorder(w http.ResponseWriter) *statusRecorder {
	return &statusRecorder{ResponseWriter: w}
}

func (s *statusRecorder) WriteHeader(code int) {
	s.wrote = true
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	// An implicit 200 counts as written: net/http commits the status on the
	// first Write, so a panic after this point cannot be turned into a 500
	// either.
	s.wrote = true
	return s.ResponseWriter.Write(b)
}

// Unwrap lets http.ResponseController reach the underlying writer, so
// Flush/SetWriteDeadline still work through this wrapper (Go 1.20+).
func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }
