package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/sssilverhand/subforge/internal/db"
)

type contextKey string

const ClaimsKey contextKey = "claims"

// Claims is stored in the request context after successful auth.
type Claims struct {
	UserID   uuid.UUID
	Username string
	Role     string
}

// TokenLookup can validate an API token hash against the DB.
type TokenLookup interface {
	GetByHash(ctx context.Context, hash string) (*db.APIToken, error)
}

// Auth returns middleware that accepts either a JWT or a Bearer API token.
// On success it stores *Claims in the request context.
func Auth(secret string, tokens TokenLookup) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			bearer := extractBearer(r)
			if bearer == "" {
				jsonError(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			claims, err := parseJWT(bearer, secret)
			if err != nil {
				// Not a JWT — try API token.
				claims, err = lookupAPIToken(r.Context(), bearer, tokens)
				if err != nil {
					jsonError(w, "unauthorized", http.StatusUnauthorized)
					return
				}
			}

			ctx := context.WithValue(r.Context(), ClaimsKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireRole returns middleware that rejects requests if the caller's role
// is not in the allowed list. Must be used after Auth.
func RequireRole(roles ...string) func(http.Handler) http.Handler {
	allowed := make(map[string]bool, len(roles))
	for _, r := range roles {
		allowed[r] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := GetClaims(r)
			if claims == nil || !allowed[claims.Role] {
				jsonError(w, "forbidden", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// GetClaims retrieves auth claims from the request context.
func GetClaims(r *http.Request) *Claims {
	v, _ := r.Context().Value(ClaimsKey).(*Claims)
	return v
}

// IssueJWT creates a signed JWT for an admin user.
func IssueJWT(secret string, userID uuid.UUID, username, role string, expiry time.Duration) (string, error) {
	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"uid":  userID.String(),
		"sub":  username,
		"role": role,
		"iat":  now.Unix(),
		"exp":  now.Add(expiry).Unix(),
	})
	return token.SignedString([]byte(secret))
}

// HashAPIToken returns the SHA-256 hex of a raw token string.
// Only the hash is stored in DB; the raw value is shown once on creation.
func HashAPIToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// ─── internal helpers ────────────────────────────────────────────────────────

func parseJWT(raw, secret string) (*Claims, error) {
	tok, err := jwt.Parse(raw, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return []byte(secret), nil
	}, jwt.WithExpirationRequired())
	if err != nil {
		return nil, err
	}

	mc, ok := tok.Claims.(jwt.MapClaims)
	if !ok || !tok.Valid {
		return nil, jwt.ErrTokenInvalidClaims
	}

	idStr, _ := mc["uid"].(string)
	userID, err := uuid.Parse(idStr)
	if err != nil {
		return nil, jwt.ErrTokenInvalidClaims
	}

	return &Claims{
		UserID:   userID,
		Username: mc["sub"].(string),
		Role:     mc["role"].(string),
	}, nil
}

func lookupAPIToken(ctx context.Context, raw string, tokens TokenLookup) (*Claims, error) {
	hash := HashAPIToken(raw)
	tok, err := tokens.GetByHash(ctx, hash)
	if err != nil || tok == nil {
		return nil, jwt.ErrTokenInvalidClaims
	}
	return &Claims{
		UserID:   tok.ID, // treat token ID as the "user"
		Username: tok.Name,
		Role:     tok.Role,
	}, nil
}

func extractBearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if after, ok := strings.CutPrefix(h, "Bearer "); ok {
		return strings.TrimSpace(after)
	}
	return ""
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_, _ = w.Write([]byte(`{"error":"` + msg + `"}`))
}
