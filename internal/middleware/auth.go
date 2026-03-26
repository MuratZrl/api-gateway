package middleware

import (
	"context"
	"net/http"
	"strings"

	"api-gateway/internal/gateway"
	"api-gateway/internal/repository"

	"github.com/golang-jwt/jwt/v5"
)

type contextKey string

const UserClaimsKey contextKey = "user_claims"

type Auth struct {
	jwtSecret []byte
	gateway   *gateway.Gateway
	repo      *repository.MongoRepository
}

func NewAuth(jwtSecret string, gw *gateway.Gateway, repo *repository.MongoRepository) *Auth {
	return &Auth{
		jwtSecret: []byte(jwtSecret),
		gateway:   gw,
		repo:      repo,
	}
}

func (a *Auth) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		route, _ := a.gateway.FindRoute(r.URL.Path)

		// Skip auth for admin routes and unprotected routes
		if route == nil || !route.Protected {
			next.ServeHTTP(w, r)
			return
		}

		// Check for API key first
		apiKey := r.Header.Get("X-API-Key")
		if apiKey != "" {
			_, err := a.repo.GetApiKeyByKey(r.Context(), apiKey)
			if err == nil {
				next.ServeHTTP(w, r)
				return
			}
		}

		// Check for JWT token
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error": "missing authorization header"}`))
			return
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		if tokenString == authHeader {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error": "invalid authorization format, use Bearer <token>"}`))
			return
		}

		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return a.jwtSecret, nil
		})

		if err != nil || !token.Valid {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error": "invalid or expired token"}`))
			return
		}

		// Add claims to context
		ctx := context.WithValue(r.Context(), UserClaimsKey, token.Claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
