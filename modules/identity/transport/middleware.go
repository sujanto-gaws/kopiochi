package transport

import (
	"context"
	"net/http"
	"strings"

	domain "github.com/sujanto-gaws/kopiochi/modules/identity/domain"
)

func AuthRequired(tokenIssuer domain.TokenIssuer) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			if !strings.HasPrefix(header, "Bearer ") {
				http.Error(w, `{"error":"missing token"}`, http.StatusUnauthorized)
				return
			}
			tokenStr := strings.TrimPrefix(header, "Bearer ")
			// want=domain.ClassAccess makes it structurally impossible for a
			// token minted for any other purpose (e.g. the short-lived MFA
			// token) to be accepted here -- Validate rejects a class
			// mismatch itself, rather than this middleware inspecting a
			// convention-only field.
			claims, err := tokenIssuer.Validate(tokenStr, domain.ClassAccess)
			if err != nil {
				http.Error(w, `{"error":"invalid token"}`, http.StatusUnauthorized)
				return
			}
			ctx := context.WithValue(r.Context(), domain.ClaimsKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
