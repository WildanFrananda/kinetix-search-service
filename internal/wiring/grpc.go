package wiring

import (
	"go.uber.org/dig"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	"github.com/WildanFrananda/kinetix-search-service/internal/config"
	searchv1 "github.com/WildanFrananda/kinetix-search-service/internal/contractgen/search/v1"
	"github.com/WildanFrananda/kinetix-search-service/internal/grpcapi"
	"github.com/WildanFrananda/kinetix-search-service/internal/querying"
	"github.com/WildanFrananda/kinetix-search-service/internal/search"
)

type grpcParams struct {
	dig.In
	Config   config.Config
	Products *querying.Finder[search.ProductDoc]
}

func provideGRPC(p grpcParams) (*grpc.Server, error) {
	creds, err := grpcapi.ServerCredentials(p.Config.PKIDir)
	if err != nil {
		return nil, err
	}

	server := grpc.NewServer(
		grpc.Creds(creds),
		grpc.UnaryInterceptor(grpcapi.Authorize(grpcapi.AllowedPeers(p.Config.AllowedPeers))),
	)
	searchv1.RegisterSearchServiceServer(server, grpcapi.NewServer(p.Products))

	checker := health.NewServer()
	healthpb.RegisterHealthServer(server, checker)
	checker.SetServingStatus(searchv1.SearchService_ServiceDesc.ServiceName, healthpb.HealthCheckResponse_SERVING)

	reflection.Register(server)

	return server, nil
}
