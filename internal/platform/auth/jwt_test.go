package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestParseActor(t *testing.T) {
	verifier := NewJWTVerifier("test-secret")

	claims := jwt.MapClaims{
		"sub":        "player-1",
		"actor_type": "player",
		"exp":        time.Now().Add(time.Hour).Unix(),
		"iat":        time.Now().Add(-time.Minute).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte("test-secret"))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}

	actor, err := verifier.ParseActor(signed)
	if err != nil {
		t.Fatalf("parse actor: %v", err)
	}
	if actor.ID != "player-1" || actor.Type != "player" {
		t.Fatalf("unexpected actor: %+v", actor)
	}
}
