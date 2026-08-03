package middleware

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog"
)

// RequestLogger emits one structured line per completed request and installs a
// request-scoped logger into the request context.
//
// Two changes from what this replaces, both from
// docs/architectures/06-quality/observability.md:
//
//   - The base logger is injected rather than read from zerolog's package
//     global. A global cannot be swapped per test, so nothing that logs could
//     be asserted on; and every line came from the same logger regardless of
//     which component emitted it.
//
//   - The per-request child logger, already carrying request_id and client_ip,
//     is put in the context. Any layer below — handler, application service,
//     repository — can then call zerolog.Ctx(ctx) and get correlation for
//     free. Before this, a database error and the request that caused it
//     appeared as two unrelated lines.
//
// The level follows the status: 5xx logs at error, 4xx at warn, everything
// else at info. A log pipeline alerting on error level therefore sees server
// faults and not client mistakes, which is the distinction that makes the
// alert worth having.
func RequestLogger(base zerolog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			reqLog := base.With().
				Str("request_id", middleware.GetReqID(r.Context())).
				Str("client_ip", ClientIP(r.Context())).
				Logger()

			ctx := reqLog.WithContext(r.Context())
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			next.ServeHTTP(ww, r.WithContext(ctx))

			// Status() is 0 when the handler returned without writing
			// anything; net/http sends 200 in that case, so report 200 rather
			// than a status that never went on the wire.
			status := ww.Status()
			if status == 0 {
				status = http.StatusOK
			}

			evt := reqLog.Info()
			switch {
			case status >= http.StatusInternalServerError:
				evt = reqLog.Error()
			case status >= http.StatusBadRequest:
				evt = reqLog.Warn()
			}

			evt.
				Str("method", r.Method).
				Str("path", r.URL.Path).
				Int("status", status).
				Int("bytes_written", ww.BytesWritten()).
				Dur("duration_ms", time.Since(start)).
				Msg("request completed")
		})
	}
}
