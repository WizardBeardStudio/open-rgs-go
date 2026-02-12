package server

import (
	"context"
	"strings"

	rgsv1 "github.com/wizardbeard/open-rgs-go/gen/rgs/v1"
	platformauth "github.com/wizardbeard/open-rgs-go/internal/platform/auth"
)

func actorTypeFromString(v string) rgsv1.ActorType {
	switch strings.ToUpper(strings.TrimSpace(v)) {
	case "ACTOR_TYPE_PLAYER", "PLAYER":
		return rgsv1.ActorType_ACTOR_TYPE_PLAYER
	case "ACTOR_TYPE_OPERATOR", "OPERATOR":
		return rgsv1.ActorType_ACTOR_TYPE_OPERATOR
	case "ACTOR_TYPE_SERVICE", "SERVICE":
		return rgsv1.ActorType_ACTOR_TYPE_SERVICE
	default:
		return rgsv1.ActorType_ACTOR_TYPE_UNSPECIFIED
	}
}

func resolveActor(ctx context.Context, meta *rgsv1.RequestMeta) (*rgsv1.Actor, string) {
	if ctx != nil {
		if a, ok := platformauth.ActorFromContext(ctx); ok {
			ctxActor := &rgsv1.Actor{ActorId: a.ID, ActorType: actorTypeFromString(a.Type)}
			if ctxActor.ActorId == "" || ctxActor.ActorType == rgsv1.ActorType_ACTOR_TYPE_UNSPECIFIED {
				return nil, "actor context is invalid"
			}
			if meta != nil && meta.Actor != nil {
				if meta.Actor.ActorId != ctxActor.ActorId || meta.Actor.ActorType != ctxActor.ActorType {
					return nil, "actor mismatch with token"
				}
			}
			return ctxActor, ""
		}
	}
	if meta == nil || meta.Actor == nil {
		return nil, "actor is required"
	}
	if meta.Actor.ActorId == "" || meta.Actor.ActorType == rgsv1.ActorType_ACTOR_TYPE_UNSPECIFIED {
		return nil, "actor binding is required"
	}
	return meta.Actor, ""
}
