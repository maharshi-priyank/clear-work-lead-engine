package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

type ctxKey string

const UserIDKey ctxKey = "user_id"

// InjectUserID validates the Supabase JWT from the Authorization header and
// injects the user's UUID (JWT "sub" claim) into the request context.
// Falls back to X-User-Id for internal proxy callers.
func InjectUserID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid := extractUserID(r)
		if uid == "" {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), UserIDKey, uid)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func extractUserID(r *http.Request) string {
	// 1. Internal proxy header (NestJS backend calling this service)
	if uid := r.Header.Get("X-User-Id"); uid != "" {
		return uid
	}

	// 2. Bearer JWT from the frontend
	authHeader := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return ""
	}
	tokenStr := strings.TrimPrefix(authHeader, "Bearer ")

	secret := strings.Trim(strings.TrimSpace(os.Getenv("SUPABASE_JWT_SECRET")), `"`)
	if secret == "" {
		slog.Warn("auth: SUPABASE_JWT_SECRET not set")
		return ""
	}
	slog.Debug("auth: attempting JWT parse", "secretLen", len(secret), "tokenLen", len(tokenStr))

	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			slog.Warn("auth: unexpected signing method", "alg", t.Header["alg"])
			return nil, jwt.ErrSignatureInvalid
		}
		return []byte(secret), nil
	}, jwt.WithExpirationRequired())
	if err != nil || !token.Valid {
		slog.Warn("auth: JWT validation failed", "err", err, "secretLen", len(secret))
		return ""
	}

	sub, err := token.Claims.GetSubject()
	if err != nil {
		return ""
	}
	return sub
}

func UserID(ctx context.Context) string {
	v, _ := ctx.Value(UserIDKey).(string)
	return v
}
