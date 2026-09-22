package integration_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"

	searchv1 "github.com/WildanFrananda/kinetix-search-service/internal/contractgen/search/v1"
)

func grpcTarget(t *testing.T) (string, string) {
	t.Helper()
	target := os.Getenv("KINETIX_SEARCH_GRPC")
	pki := os.Getenv("KINETIX_PKI_DIR")
	if target == "" || pki == "" {
		t.Skip("set KINETIX_SEARCH_GRPC and KINETIX_PKI_DIR to run the gRPC edge tests")
	}
	return target, pki
}

func readPEM(t *testing.T, dir, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, name))
	require.NoError(t, err)
	return data
}

func mutualClient(t *testing.T, target, pki string) searchv1.SearchServiceClient {
	t.Helper()

	pair, err := tls.X509KeyPair(readPEM(t, pki, "tls.crt"), readPEM(t, pki, "tls.key"))
	require.NoError(t, err)
	pool := x509.NewCertPool()
	require.True(t, pool.AppendCertsFromPEM(readPEM(t, pki, "ca.pem")))

	conn, err := grpc.NewClient(target, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{pair},
		RootCAs:      pool,
		ServerName:   "kinetix-search-service",
		MinVersion:   tls.VersionTLS13,
	})))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	return searchv1.NewSearchServiceClient(conn)
}

func TestTheGRPCEdgeAnswersOverMutualTLS(t *testing.T) {
	target, pki := grpcTarget(t)
	client := mutualClient(t, target, pki)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	response, err := client.SearchProducts(ctx, &searchv1.SearchProductsRequest{
		Text: "sepato kulet",
		Page: &searchv1.Page{Limit: 3},
	})
	require.NoError(t, err)

	require.Positive(t, response.GetTotal(), "a one-character typo must still find products")
	require.NotEmpty(t, response.GetHits())
	require.NotNil(t, response.GetFreshness(), "a gRPC caller is told how current the index is too")

	first := response.GetHits()[0]
	require.NotEmpty(t, first.GetSku())
	require.Equal(t, "IDR", first.GetPrice().GetCurrency())
	require.Positive(t, first.GetPrice().GetAmountMinor())
}

func TestSuggestOverGRPC(t *testing.T) {
	target, pki := grpcTarget(t)
	client := mutualClient(t, target, pki)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	response, err := client.SuggestProducts(
		ctx,
		&searchv1.SuggestProductsRequest{Prefix: "Sep", Limit: 3},
	)

	require.NoError(t, err)
	require.NotEmpty(t, response.GetSuggestions())
}

func TestHealthIsServing(t *testing.T) {
	target, pki := grpcTarget(t)

	pair, err := tls.X509KeyPair(readPEM(t, pki, "tls.crt"), readPEM(t, pki, "tls.key"))
	require.NoError(t, err)
	pool := x509.NewCertPool()
	require.True(t, pool.AppendCertsFromPEM(readPEM(t, pki, "ca.pem")))

	conn, err := grpc.NewClient(target, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{pair},
		RootCAs:      pool,
		ServerName:   "kinetix-search-service",
		MinVersion:   tls.VersionTLS13,
	})))
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	response, err := grpc_health_v1.NewHealthClient(conn).Check(
		ctx,
		&grpc_health_v1.HealthCheckRequest{Service: "search.v1.SearchService"},
	)

	require.NoError(t, err)
	require.Equal(t, grpc_health_v1.HealthCheckResponse_SERVING, response.GetStatus())
}

func TestTheEdgeRefusesACallerWithNoCertificate(t *testing.T) {
	target, pki := grpcTarget(t)

	pool := x509.NewCertPool()
	require.True(t, pool.AppendCertsFromPEM(readPEM(t, pki, "ca.pem")))

	conn, err := grpc.NewClient(target, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{
		RootCAs:    pool,
		ServerName: "kinetix-search-service",
		MinVersion: tls.VersionTLS13,
	})))
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = searchv1.NewSearchServiceClient(conn).SuggestProducts(ctx,
		&searchv1.SuggestProductsRequest{Prefix: "x"})

	require.Error(t, err, "TLS alone is not enough; the port requires a client certificate")
	require.Equal(t, codes.Unavailable, status.Code(err))
}

func TestTheEdgeRefusesPlaintext(t *testing.T) {
	target, _ := grpcTarget(t)

	conn, err := grpc.NewClient(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = searchv1.NewSearchServiceClient(conn).SuggestProducts(
		ctx,
		&searchv1.SuggestProductsRequest{Prefix: "x"},
	)

	require.Error(t, err)
}
