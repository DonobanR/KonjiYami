package api

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"konjiyami/server/assets"
)

type contextKey string

const claimsContextKey contextKey = "jwt_claims"

var (
	ErrTokenExpired   = errors.New("token expired")
	ErrTokenInvalid   = errors.New("token invalid")
	ErrTokenMalformed = errors.New("token malformed")
)

// JWTClaims holds the payload of a KonjiYami JWT.
type JWTClaims struct {
	Username  string `json:"sub"`
	Role      string `json:"role"`
	Type      string `json:"type"` // "access" or "refresh"
	IssuedAt  int64  `json:"iat"`
	ExpiresAt int64  `json:"exp"`
}

const (
	accessTokenExpiry  = 15 * time.Minute
	refreshTokenExpiry = 24 * time.Hour
)

var jwtSecret []byte

func initJWTSecret() error {
	secretPath := filepath.Join(assets.GetRootAppDir(), "jwt_secret")
	data, err := os.ReadFile(secretPath)
	if err == nil && len(data) >= 32 {
		jwtSecret = data
		return nil
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return fmt.Errorf("generate jwt secret: %w", err)
	}
	if err := os.WriteFile(secretPath, secret, 0600); err != nil {
		return fmt.Errorf("save jwt secret: %w", err)
	}
	jwtSecret = secret
	return nil
}

func b64URLEncode(data []byte) string {
	return strings.TrimRight(base64.URLEncoding.EncodeToString(data), "=")
}

func b64URLDecode(s string) ([]byte, error) {
	switch len(s) % 4 {
	case 2:
		s += "=="
	case 3:
		s += "="
	}
	return base64.URLEncoding.DecodeString(s)
}

func signToken(claims *JWTClaims) (string, error) {
	header := `{"alg":"HS256","typ":"JWT"}`
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	headerB64 := b64URLEncode([]byte(header))
	claimsB64 := b64URLEncode(claimsJSON)
	signingInput := headerB64 + "." + claimsB64

	mac := hmac.New(sha256.New, jwtSecret)
	mac.Write([]byte(signingInput))
	sig := b64URLEncode(mac.Sum(nil))

	return signingInput + "." + sig, nil
}

func validateToken(tokenStr string) (*JWTClaims, error) {
	parts := strings.Split(tokenStr, ".")
	if len(parts) != 3 {
		return nil, ErrTokenMalformed
	}

	signingInput := parts[0] + "." + parts[1]
	mac := hmac.New(sha256.New, jwtSecret)
	mac.Write([]byte(signingInput))
	expectedSig := b64URLEncode(mac.Sum(nil))

	if !hmac.Equal([]byte(parts[2]), []byte(expectedSig)) {
		return nil, ErrTokenInvalid
	}

	claimsJSON, err := b64URLDecode(parts[1])
	if err != nil {
		return nil, ErrTokenMalformed
	}
	var claims JWTClaims
	if err := json.Unmarshal(claimsJSON, &claims); err != nil {
		return nil, ErrTokenMalformed
	}
	if time.Now().Unix() > claims.ExpiresAt {
		return nil, ErrTokenExpired
	}
	return &claims, nil
}

func generateAccessToken(username, role string) (string, error) {
	now := time.Now()
	return signToken(&JWTClaims{
		Username:  username,
		Role:      role,
		Type:      "access",
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(accessTokenExpiry).Unix(),
	})
}

func generateRefreshToken(username, role string) (string, error) {
	now := time.Now()
	return signToken(&JWTClaims{
		Username:  username,
		Role:      role,
		Type:      "refresh",
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(refreshTokenExpiry).Unix(),
	})
}

// getClaims extracts JWT claims stored in the request context by jwtRequired.
func getClaims(r *http.Request) *JWTClaims {
	claims, _ := r.Context().Value(claimsContextKey).(*JWTClaims)
	return claims
}

func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	if t := r.URL.Query().Get("token"); t != "" {
		return t
	}
	return ""
}

// jwtRequired is middleware that validates JWT Bearer tokens on protected routes.
func (s *HTTPServer) jwtRequired(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenStr := extractBearerToken(r)
		if tokenStr == "" {
			writeError(w, http.StatusUnauthorized, "missing authorization token")
			return
		}
		claims, err := validateToken(tokenStr)
		if err != nil {
			msg := "invalid token"
			if errors.Is(err, ErrTokenExpired) {
				msg = "token expired"
			}
			writeError(w, http.StatusUnauthorized, msg)
			return
		}
		if claims.Type != "access" {
			writeError(w, http.StatusUnauthorized, "invalid token type")
			return
		}
		ctx := context.WithValue(r.Context(), claimsContextKey, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
