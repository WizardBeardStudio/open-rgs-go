package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type contextKey string

const actorContextKey contextKey = "actor"

type Actor struct {
	ID   string
	Type string
}

type JWTVerifier struct {
	secret []byte
}

func NewJWTVerifier(secret string) *JWTVerifier {
	return &JWTVerifier{secret: []byte(secret)}
}

func (v *JWTVerifier) ParseActor(tokenString string) (Actor, error) {
	claims := jwt.MapClaims{}
	tok, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, errors.New("unexpected signing method")
		}
		return v.secret, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithLeeway(5*time.Second))
	if err != nil || !tok.Valid {
		return Actor{}, errors.New("invalid token")
	}

	sub, _ := claims["sub"].(string)
	actorType, _ := claims["actor_type"].(string)
	if sub == "" || actorType == "" {
		return Actor{}, errors.New("missing actor claims")
	}
	return Actor{ID: sub, Type: actorType}, nil
}

func WithActor(ctx context.Context, actor Actor) context.Context {
	return context.WithValue(ctx, actorContextKey, actor)
}

func ActorFromContext(ctx context.Context) (Actor, bool) {
	v, ok := ctx.Value(actorContextKey).(Actor)
	return v, ok
}

func HTTPJWTMiddleware(verifier *JWTVerifier, next http.Handler) http.Handler {
	return HTTPJWTMiddlewareWithSkips(verifier, next, nil)
}

func HTTPJWTMiddlewareWithSkips(verifier *JWTVerifier, next http.Handler, skipPaths []string) http.Handler {
	skip := make(map[string]struct{}, len(skipPaths))
	for _, p := range skipPaths {
		skip[p] = struct{}{}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := skip[r.URL.Path]; ok {
			next.ServeHTTP(w, r)
			return
		}
		h := r.Header.Get("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			http.Error(w, "missing bearer token", http.StatusUnauthorized)
			return
		}
		tok := strings.TrimPrefix(h, "Bearer ")
		actor, err := verifier.ParseActor(tok)
		if err != nil {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r.WithContext(WithActor(r.Context(), actor)))
	})
}

type RefreshTokenAllowlist struct {
	mu     sync.RWMutex
	tokens map[string]struct{}
}

func NewRefreshTokenAllowlist() *RefreshTokenAllowlist {
	return &RefreshTokenAllowlist{tokens: make(map[string]struct{})}
}

func (l *RefreshTokenAllowlist) Add(token string) {
	if l == nil || strings.TrimSpace(token) == "" {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.tokens[token] = struct{}{}
}

func (l *RefreshTokenAllowlist) Remove(token string) {
	if l == nil || strings.TrimSpace(token) == "" {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.tokens, token)
}

func (l *RefreshTokenAllowlist) Contains(token string) bool {
	if l == nil || strings.TrimSpace(token) == "" {
		return false
	}
	l.mu.RLock()
	defer l.mu.RUnlock()
	_, ok := l.tokens[token]
	return ok
}
