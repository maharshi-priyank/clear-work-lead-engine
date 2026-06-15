package middleware

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/golang-jwt/jwt/v5"
)

type ctxKey string

const UserIDKey ctxKey = "user_id"

// EC public key loaded once from Supabase JWKS endpoint.
var (
	ecKeyOnce sync.Once
	ecPubKey  *ecdsa.PublicKey
)

type jwksDoc struct {
	Keys []struct {
		Kty string `json:"kty"`
		Crv string `json:"crv"`
		X   string `json:"x"`
		Y   string `json:"y"`
	} `json:"keys"`
}

func loadECKey() *ecdsa.PublicKey {
	supabaseURL := strings.TrimRight(os.Getenv("SUPABASE_URL"), "/")
	if supabaseURL == "" {
		slog.Warn("auth: SUPABASE_URL not set — cannot load JWKS")
		return nil
	}
	resp, err := http.Get(supabaseURL + "/auth/v1/.well-known/jwks.json")
	if err != nil {
		slog.Warn("auth: JWKS fetch failed", "err", err)
		return nil
	}
	defer resp.Body.Close()

	var doc jwksDoc
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		slog.Warn("auth: JWKS decode failed", "err", err)
		return nil
	}

	for _, k := range doc.Keys {
		if k.Kty != "EC" || k.Crv != "P-256" {
			continue
		}
		xb, err1 := base64.RawURLEncoding.DecodeString(k.X)
		yb, err2 := base64.RawURLEncoding.DecodeString(k.Y)
		if err1 != nil || err2 != nil {
			continue
		}
		pub := &ecdsa.PublicKey{
			Curve: elliptic.P256(),
			X:     new(big.Int).SetBytes(xb),
			Y:     new(big.Int).SetBytes(yb),
		}
		slog.Info("auth: EC public key loaded from JWKS")
		return pub
	}
	slog.Warn("auth: no EC P-256 key found in JWKS")
	return nil
}

func ecKey() *ecdsa.PublicKey {
	ecKeyOnce.Do(func() { ecPubKey = loadECKey() })
	return ecPubKey
}

// InjectUserID validates the Supabase Bearer JWT (ES256) and injects the
// user UUID into the request context. Falls back to X-User-Id for internal
// NestJS proxy calls.
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
	// 1. Internal proxy header set by NestJS backend
	if uid := r.Header.Get("X-User-Id"); uid != "" {
		return uid
	}

	// 2. Bearer JWT from the frontend (ES256, signed by Supabase)
	authHeader := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return ""
	}
	tokenStr := strings.TrimPrefix(authHeader, "Bearer ")

	pub := ecKey()
	if pub == nil {
		slog.Warn("auth: EC public key unavailable")
		return ""
	}

	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodECDSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return pub, nil
	}, jwt.WithExpirationRequired())
	if err != nil || !token.Valid {
		slog.Warn("auth: JWT validation failed", "err", err)
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
