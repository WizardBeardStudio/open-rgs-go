package auth

import (
	"context"
	"errors"
	"fmt"
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

type HMACKeyset struct {
	ActiveKID string
	Keys      map[string][]byte
}

func ParseHMACKeyset(legacySecret, keysetSpec, activeKID string) (HMACKeyset, error) {
	out := HMACKeyset{
		ActiveKID: activeKID,
		Keys:      make(map[string][]byte),
	}
	if strings.TrimSpace(out.ActiveKID) == "" {
		out.ActiveKID = "default"
	}
	if strings.TrimSpace(keysetSpec) != "" {
		parts := strings.Split(keysetSpec, ",")
		for _, part := range parts {
			entry := strings.TrimSpace(part)
			if entry == "" {
				continue
			}
			pair := strings.SplitN(entry, ":", 2)
			if len(pair) != 2 {
				return HMACKeyset{}, fmt.Errorf("invalid keyset entry %q", entry)
			}
			kid := strings.TrimSpace(pair[0])
			secret := strings.TrimSpace(pair[1])
			if kid == "" || secret == "" {
				return HMACKeyset{}, fmt.Errorf("invalid keyset entry %q", entry)
			}
			out.Keys[kid] = []byte(secret)
		}
	} else {
		if strings.TrimSpace(legacySecret) == "" {
			return HMACKeyset{}, errors.New("jwt secret is required")
		}
		out.Keys[out.ActiveKID] = []byte(legacySecret)
	}
	if len(out.Keys) == 0 {
		return HMACKeyset{}, errors.New("jwt keyset is empty")
	}
	if _, ok := out.Keys[out.ActiveKID]; !ok {
		return HMACKeyset{}, fmt.Errorf("active kid %q not found in keyset", out.ActiveKID)
	}
	return out, nil
}

type JWTSigner struct {
	activeKID string
	keys      map[string][]byte
}

func NewJWTSigner(secret string) *JWTSigner {
	keyset, err := ParseHMACKeyset(secret, "", "default")
	if err != nil {
		panic(err)
	}
	return NewJWTSignerWithKeyset(keyset)
}

func NewJWTSignerWithKeyset(keyset HMACKeyset) *JWTSigner {
	return &JWTSigner{
		activeKID: keyset.ActiveKID,
		keys:      keyset.Keys,
	}
}

func (s *JWTSigner) SignActor(actor Actor, now time.Time, ttl time.Duration) (string, time.Time, error) {
	if s == nil {
		return "", time.Time{}, errors.New("signer is nil")
	}
	secret := s.keys[s.activeKID]
	if len(secret) == 0 {
		return "", time.Time{}, errors.New("active jwt key is missing")
	}
	expiresAt := now.UTC().Add(ttl)
	claims := jwt.MapClaims{
		"sub":        actor.ID,
		"actor_type": actor.Type,
		"iat":        now.UTC().Unix(),
		"exp":        expiresAt.Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	token.Header["kid"] = s.activeKID
	signed, err := token.SignedString(secret)
	if err != nil {
		return "", time.Time{}, err
	}
	return signed, expiresAt, nil
}

type JWTVerifier struct {
	activeKID string
	keys      map[string][]byte
}

func NewJWTVerifier(secret string) *JWTVerifier {
	keyset, err := ParseHMACKeyset(secret, "", "default")
	if err != nil {
		panic(err)
	}
	return NewJWTVerifierWithKeyset(keyset)
}

func NewJWTVerifierWithKeyset(keyset HMACKeyset) *JWTVerifier {
	return &JWTVerifier{activeKID: keyset.ActiveKID, keys: keyset.Keys}
}

func (v *JWTVerifier) ParseActor(tokenString string) (Actor, error) {
	claims := jwt.MapClaims{}
	tok, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, errors.New("unexpected signing method")
		}
		kid, _ := token.Header["kid"].(string)
		if strings.TrimSpace(kid) == "" {
			kid = v.activeKID
		}
		secret := v.keys[kid]
		if len(secret) == 0 {
			return nil, errors.New("unknown key id")
		}
		return secret, nil
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
