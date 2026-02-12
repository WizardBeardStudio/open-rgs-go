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

func TestParseActorWithKeyRotation(t *testing.T) {
	keyset, err := ParseHMACKeyset("", "old:old-secret,new:new-secret", "new")
	if err != nil {
		t.Fatalf("parse keyset: %v", err)
	}
	signerOld := NewJWTSignerWithKeyset(HMACKeyset{
		ActiveKID: "old",
		Keys:      keyset.Keys,
	})
	signerNew := NewJWTSignerWithKeyset(HMACKeyset{
		ActiveKID: "new",
		Keys:      keyset.Keys,
	})
	verifier := NewJWTVerifierWithKeyset(keyset)

	now := time.Now().UTC()
	oldToken, _, err := signerOld.SignActor(Actor{ID: "player-1", Type: "ACTOR_TYPE_PLAYER"}, now, time.Hour)
	if err != nil {
		t.Fatalf("sign old token: %v", err)
	}
	newToken, _, err := signerNew.SignActor(Actor{ID: "player-2", Type: "ACTOR_TYPE_PLAYER"}, now, time.Hour)
	if err != nil {
		t.Fatalf("sign new token: %v", err)
	}

	oldActor, err := verifier.ParseActor(oldToken)
	if err != nil {
		t.Fatalf("verify old token: %v", err)
	}
	newActor, err := verifier.ParseActor(newToken)
	if err != nil {
		t.Fatalf("verify new token: %v", err)
	}
	if oldActor.ID != "player-1" || newActor.ID != "player-2" {
		t.Fatalf("unexpected actors after rotation: old=%+v new=%+v", oldActor, newActor)
	}
}

func TestJWTLiveKeysetReload(t *testing.T) {
	initial, err := ParseHMACKeyset("", "old:old-secret", "old")
	if err != nil {
		t.Fatalf("parse initial keyset: %v", err)
	}
	rotated, err := ParseHMACKeyset("", "new:new-secret", "new")
	if err != nil {
		t.Fatalf("parse rotated keyset: %v", err)
	}
	signer := NewJWTSignerWithKeyset(initial)
	verifier := NewJWTVerifierWithKeyset(initial)

	oldToken, _, err := signer.SignActor(Actor{ID: "player-old", Type: "ACTOR_TYPE_PLAYER"}, time.Now().UTC(), time.Hour)
	if err != nil {
		t.Fatalf("sign old token: %v", err)
	}
	if _, err := verifier.ParseActor(oldToken); err != nil {
		t.Fatalf("verify old token before reload: %v", err)
	}

	if err := signer.SetKeyset(rotated); err != nil {
		t.Fatalf("reload signer keyset: %v", err)
	}
	if err := verifier.SetKeyset(rotated); err != nil {
		t.Fatalf("reload verifier keyset: %v", err)
	}

	if _, err := verifier.ParseActor(oldToken); err == nil {
		t.Fatalf("expected old token to fail after keyset rotation")
	}
	newToken, _, err := signer.SignActor(Actor{ID: "player-new", Type: "ACTOR_TYPE_PLAYER"}, time.Now().UTC(), time.Hour)
	if err != nil {
		t.Fatalf("sign new token: %v", err)
	}
	if _, err := verifier.ParseActor(newToken); err != nil {
		t.Fatalf("verify new token after reload: %v", err)
	}
}
