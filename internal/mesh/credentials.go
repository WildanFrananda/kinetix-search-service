package mesh

import (
	"crypto/tls"
	"crypto/x509"
	"net"
	"os"
	"path/filepath"
	"time"

	"google.golang.org/grpc/credentials"

	"github.com/WildanFrananda/kinetix-search-service/internal/search"
)

type Settings struct {
	Endpoint   string
	PKIDir     string
	ServerName string
	Deadline   time.Duration
}

func (s Settings) DeadlineOr(fallback time.Duration) time.Duration {
	if s.Deadline <= 0 {
		return fallback
	}

	return s.Deadline
}

func MutualTLS(op string, s Settings) (credentials.TransportCredentials, error) {
	if s.PKIDir == "" {
		return nil, search.Errf(
			search.KindDependencyUnavailable,
			op,
			nil,
			"no PKI directory configured",
		)
	}

	if s.ServerName == "" {
		return nil, search.Errf(
			search.KindDependencyUnavailable,
			op,
			nil,
			"no server name to verify the peer against",
		)
	}

	if host := hostOf(s.Endpoint); host != s.ServerName {
		return nil, search.Errf(
			search.KindDependencyUnavailable,
			op,
			nil,
			"endpoint %q is host %q, which is not the %q this caller expects to verify",
			s.Endpoint,
			host,
			s.ServerName,
		)
	}

	certPEM, err := read(op, s.PKIDir, "tls.crt")

	if err != nil {
		return nil, err
	}

	keyPEM, err := read(op, s.PKIDir, "tls.key")

	if err != nil {
		return nil, err
	}

	caPEM, err := read(op, s.PKIDir, "ca.pem")

	if err != nil {
		return nil, err
	}

	pair, err := tls.X509KeyPair(certPEM, keyPEM)

	if err != nil {
		return nil, search.Errf(
			search.KindDependencyUnavailable,
			op,
			err,
			"tls.crt and tls.key are not a pair",
		)
	}

	roots := x509.NewCertPool()

	if !roots.AppendCertsFromPEM(caPEM) {
		return nil, search.Errf(
			search.KindDependencyUnavailable,
			op,
			nil,
			"ca.pem holds no usable certificate",
		)
	}

	return credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{pair},
		RootCAs:      roots,
		MinVersion:   tls.VersionTLS13,
	}), nil
}

func hostOf(endpoint string) string {
	host, _, err := net.SplitHostPort(endpoint)

	if err != nil {
		return endpoint
	}

	return host
}

func read(op, dir, name string) ([]byte, error) {
	path := filepath.Join(dir, name)
	data, err := os.ReadFile(path) //nolint:gosec // the path comes from configuration, not a request

	if err != nil {
		return nil, search.Errf(search.KindDependencyUnavailable, op, err, "reading %s", path)
	}

	if len(data) == 0 {
		return nil, search.Errf(search.KindDependencyUnavailable, op, nil, "%s is empty", path)
	}

	return data, nil
}
