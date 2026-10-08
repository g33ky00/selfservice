package auth

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"strings"
)

// Middleware validates HMAC-SHA256 request signatures.
// Expected header: Authorization: Bearer <hex(HMAC-SHA256(body, secret))>
// If secret is empty, all requests are allowed (useful for loopback-only deployments).
func Middleware(secret string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if secret == "" {
			next.ServeHTTP(w, r)
			return
		}
		auth := r.Header.Get("Authorization")
		sig := strings.TrimPrefix(auth, "Bearer ")
		if sig == auth || sig == "" {
			http.Error(w, `{"status":"error","details":"missing Authorization"}`, http.StatusUnauthorized)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, `{"status":"error","details":"read body"}`, http.StatusBadRequest)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))

		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(body)
		expected := hex.EncodeToString(mac.Sum(nil))
		if !hmac.Equal([]byte(sig), []byte(expected)) {
			http.Error(w, `{"status":"error","details":"invalid signature"}`, http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
