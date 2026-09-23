package mesh_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/WildanFrananda/kinetix-search-service/internal/mesh"
	"github.com/WildanFrananda/kinetix-search-service/internal/search"
)

func writePKI(t *testing.T, name string) string {
	t.Helper()
	dir := t.TempDir()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: name},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		DNSNames:              []string{name},
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
			x509.ExtKeyUsageClientAuth,
		},
	}

	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	require.NoError(t, err)

	pkcs8, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8})

	require.NoError(t, os.WriteFile(filepath.Join(dir, "tls.crt"), certPEM, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "tls.key"), keyPEM, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ca.pem"), certPEM, 0o600))

	return dir
}

func TestTheAuthorityKeepsItsPort(t *testing.T) {
	dir := writePKI(t, "kinetix-order-service")
	creds, err := mesh.MutualTLS("test", mesh.Settings{
		Endpoint: "kinetix-order-service:50055",
		PKIDir:   dir,
	})
	require.NoError(t, err)

	seen := serveAndReport(t, dir)

	callThrough(t, creds, "passthrough:///kinetix-order-service:50055", seen.addr)

	select {
	case authority := <-seen.authority:
		require.Equal(t, "kinetix-order-service:50055", authority,
			"order's RequireHost(\"*:50055\") matches on the port; without it the call 404s")
	case <-time.After(10 * time.Second):
		t.Fatal("the server was never reached")
	}
}

func TestTheHandshakeVerifiesTheHostBeingDialled(t *testing.T) {
	dir := writePKI(t, "kinetix-order-service")
	creds, err := mesh.MutualTLS("test", mesh.Settings{
		Endpoint: "kinetix-review-service:50057",
		PKIDir:   dir,
	})
	require.NoError(t, err)

	seen := serveAndReport(t, dir)
	err = call(t, creds, "passthrough:///kinetix-review-service:50057", seen.addr)

	require.Error(t, err, "a leaf for another name must not satisfy this dial")
	require.Equal(t, codes.Unavailable, status.Code(err))
}

func TestAMissingPKIDirectoryIsRefused(t *testing.T) {
	_, err := mesh.MutualTLS("test", mesh.Settings{Endpoint: "kinetix-order-service:50055"})

	require.Error(t, err)
	require.Equal(t, search.KindDependencyUnavailable, search.KindOf(err))
}

func TestAnEmptyLeafIsRefused(t *testing.T) {
	dir := writePKI(t, "kinetix-order-service")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "tls.crt"), nil, 0o600))

	_, err := mesh.MutualTLS("test", mesh.Settings{
		Endpoint: "kinetix-order-service:50055",
		PKIDir:   dir,
	})

	require.Error(t, err, "an empty file is not a certificate")
	require.Equal(t, search.KindDependencyUnavailable, search.KindOf(err))
}

type reported struct {
	addr      string
	authority chan string
}

func serveAndReport(t *testing.T, pkiDir string) reported {
	t.Helper()

	pair, err := tls.LoadX509KeyPair(
		filepath.Join(pkiDir, "tls.crt"),
		filepath.Join(pkiDir, "tls.key"),
	)
	require.NoError(t, err)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	out := reported{addr: listener.Addr().String(), authority: make(chan string, 1)}

	server := grpc.NewServer(
		grpc.Creds(credentials.NewTLS(&tls.Config{
			Certificates: []tls.Certificate{pair},
			MinVersion:   tls.VersionTLS13,
		})),
		grpc.UnknownServiceHandler(func(_ any, stream grpc.ServerStream) error {
			if md, ok := metadata.FromIncomingContext(stream.Context()); ok {
				if values := md.Get(":authority"); len(values) > 0 {
					out.authority <- values[0]
				}
			}

			return status.Error(codes.Unimplemented, "probe")
		}),
	)

	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)

	return out
}

func call(t *testing.T, creds credentials.TransportCredentials, target, addr string) error {
	t.Helper()

	conn, err := grpc.NewClient(target,
		grpc.WithTransportCredentials(creds),
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "tcp", addr)
		}),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	return conn.Invoke(ctx, "/probe.Probe/Ping", &emptypb.Empty{}, &emptypb.Empty{})
}

func callThrough(t *testing.T, creds credentials.TransportCredentials, target, addr string) {
	t.Helper()

	err := call(t, creds, target, addr)
	require.Error(t, err)
	require.Equalf(
		t,
		codes.Unimplemented,
		status.Code(err),
		"the call never reached the server: %v",
		err,
	)
}
