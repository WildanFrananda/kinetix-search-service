package mesh_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/WildanFrananda/kinetix-search-service/internal/mesh"
	"github.com/WildanFrananda/kinetix-search-service/internal/search"
)

func writePKI(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "kinetix-search-service"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		DNSNames:     []string{"kinetix-search-service"},
		IsCA:         true,
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

func settings(t *testing.T, endpoint, serverName string) mesh.Settings {
	t.Helper()

	return mesh.Settings{
		Endpoint:   endpoint,
		PKIDir:     writePKI(t),
		ServerName: serverName,
	}
}

func TestTheServerNameNeverReachesTheTLSConfig(t *testing.T) {
	creds, err := mesh.MutualTLS(
		"test",
		settings(t, "kinetix-order-service:50055", "kinetix-order-service"),
	)

	require.NoError(t, err)

	require.Empty(
		t,
		creds.Info().ServerName,
		"a non-empty ServerName becomes the :authority and strips the port",
	)
}

func TestAnEndpointThatIsNotTheExpectedHostIsRefused(t *testing.T) {
	_, err := mesh.MutualTLS(
		"test",
		settings(t, "kinetix-review-service:50057", "kinetix-order-service"),
	)

	require.Error(t, err, "pointing a client at another service must not pass quietly")
	require.Equal(t, search.KindDependencyUnavailable, search.KindOf(err))
}

func TestAnEndpointWithNoPortIsStillCheckedAgainstItsName(t *testing.T) {
	_, err := mesh.MutualTLS("test", settings(t, "kinetix-order-service", "kinetix-order-service"))
	require.NoError(t, err)

	_, err = mesh.MutualTLS("test", settings(t, "somewhere-else", "kinetix-order-service"))
	require.Error(t, err)
}

func TestNoServerNameIsRefused(t *testing.T) {
	_, err := mesh.MutualTLS("test", settings(t, "kinetix-order-service:50055", ""))

	require.Error(t, err)
}

func TestAMissingPKIDirectoryIsRefused(t *testing.T) {
	_, err := mesh.MutualTLS("test", mesh.Settings{
		Endpoint:   "kinetix-order-service:50055",
		ServerName: "kinetix-order-service",
	})

	require.Error(t, err)
	require.Equal(t, search.KindDependencyUnavailable, search.KindOf(err))
}
