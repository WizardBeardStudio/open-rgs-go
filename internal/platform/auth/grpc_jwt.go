package auth

import (
	"context"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func UnaryJWTInterceptor(verifier *JWTVerifier, allowUnauthenticatedMethods []string) grpc.UnaryServerInterceptor {
	allow := make(map[string]struct{}, len(allowUnauthenticatedMethods))
	for _, m := range allowUnauthenticatedMethods {
		allow[m] = struct{}{}
	}
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if _, ok := allow[info.FullMethod]; ok {
			return handler(ctx, req)
		}
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "missing metadata")
		}
		authz := md.Get("authorization")
		if len(authz) == 0 {
			return nil, status.Error(codes.Unauthenticated, "missing bearer token")
		}
		h := authz[0]
		if !strings.HasPrefix(h, "Bearer ") {
			return nil, status.Error(codes.Unauthenticated, "missing bearer token")
		}
		token := strings.TrimPrefix(h, "Bearer ")
		actor, err := verifier.ParseActor(token)
		if err != nil {
			return nil, status.Error(codes.Unauthenticated, "invalid token")
		}
		return handler(WithActor(ctx, actor), req)
	}
}
