package grpcapi

import (
	"context"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	"github.com/WildanFrananda/kinetix-search-service/internal/mesh"
)

var openMethods = []string{
	"/grpc.health.v1.Health/",
	"/grpc.reflection.",
}

func AllowedPeers(configured string) map[string]struct{} {
	allowed := make(map[string]struct{})

	for _, value := range strings.Split(configured, ",") {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			allowed[trimmed] = struct{}{}
		}
	}

	return allowed
}

func Authorize(allowed map[string]struct{}) grpc.UnaryServerInterceptor {
	domains := mesh.TrustDomains()

	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		for _, open := range openMethods {
			if strings.HasPrefix(info.FullMethod, open) {
				return handler(ctx, req)
			}
		}

		name, err := PeerName(ctx, domains)
		if err != nil {
			return nil, err
		}

		if _, ok := allowed[name]; !ok {
			return nil, status.Errorf(
				codes.PermissionDenied,
				"'%s' is not on this service's caller list",
				name,
			)
		}

		return handler(ctx, req)
	}
}

func PeerName(ctx context.Context, domains []string) (string, error) {
	caller, ok := peer.FromContext(ctx)
	if !ok {
		return "", status.Error(codes.PermissionDenied, "the caller presented no transport")
	}

	tls, ok := caller.AuthInfo.(credentials.TLSInfo)
	if !ok {
		return "", status.Error(codes.PermissionDenied, "the caller presented no certificate")
	}

	name := mesh.PeerService(tls.State.PeerCertificates, domains)
	if name == "" {
		return "", status.Error(
			codes.PermissionDenied,
			"the caller's certificate names no service in a trust domain this service accepts",
		)
	}

	return name, nil
}
