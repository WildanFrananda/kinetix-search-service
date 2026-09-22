package grpcapi

import (
	"crypto/tls"
	"crypto/x509"
	"os"
	"path/filepath"

	"google.golang.org/grpc/credentials"

	"github.com/WildanFrananda/kinetix-search-service/internal/search"
)

func ServerCredentials(pkiDir string) (credentials.TransportCredentials, error) {
	if pkiDir == "" {
		return nil, search.Errf(
			search.KindDependencyUnavailable,
			"grpcapi.ServerCredentials",
			nil,
			"no PKI directory configured",
		)
	}

	certPEM, err := read(pkiDir, "tls.crt")

	if err != nil {
		return nil, err
	}

	keyPEM, err := read(pkiDir, "tls.key")

	if err != nil {
		return nil, err
	}

	caPEM, err := read(pkiDir, "ca.pem")

	if err != nil {
		return nil, err
	}

	pair, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, search.Errf(
			search.KindDependencyUnavailable,
			"grpcapi.ServerCredentials",
			err,
			"tls.crt and tls.key are not a pair",
		)
	}

	pool := x509.NewCertPool()

	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, search.Errf(
			search.KindDependencyUnavailable,
			"grpcapi.ServerCredentials",
			nil,
			"ca.pem holds no usable certificate",
		)
	}

	return credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{pair},
		ClientCAs:    pool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		MinVersion:   tls.VersionTLS13,
	}), nil
}

func read(dir, name string) ([]byte, error) {
	path := filepath.Join(dir, name)
	data, err := os.ReadFile(path) //nolint:gosec // the path is configuration, not a request

	if err != nil {
		return nil, search.Errf(
			search.KindDependencyUnavailable,
			"grpcapi.ServerCredentials",
			err,
			"reading %s",
			path,
		)
	}

	if len(data) == 0 {
		return nil, search.Errf(
			search.KindDependencyUnavailable,
			"grpcapi.ServerCredentials",
			nil,
			"%s is empty",
			path,
		)
	}
	return data, nil
}
