package middleware

import (
	"context"
	"net/http"
)

type ctxKey string

const UserIDKey ctxKey = "user_id"

// InjectUserID reads the X-User-Id header that NestJS sets after validating
// the JWT. Every lead-engine endpoint trusts this header — it is only
// reachable from the NestJS proxy, never from the public internet.
func InjectUserID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid := r.Header.Get("X-User-Id")
		if uid == "" {
			http.Error(w, `{"error":"missing X-User-Id"}`, http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), UserIDKey, uid)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func UserID(ctx context.Context) string {
	v, _ := ctx.Value(UserIDKey).(string)
	return v
}
