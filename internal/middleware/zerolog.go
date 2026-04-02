package middleware

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog/log"
)

// ZerologRequestLogger replaces chi's default logger with structured zerolog output
func ZerologRequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		start := time.Now()

		next.ServeHTTP(ww, r)

		log.Info().
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Str("request_id", middleware.GetReqID(r.Context())).
			Int("status", ww.Status()).
			Int64("bytes_written", int64(ww.BytesWritten())).
			Dur("duration_ms", time.Since(start)).
			Msg("request completed")
	})
}
