package catalogclient

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"os"
	"path/filepath"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"

	catalogv1 "github.com/WildanFrananda/kinetix-search-service/internal/contractgen/catalog/v1"
	"github.com/WildanFrananda/kinetix-search-service/internal/search"
)

type Settings struct {
	Endpoint   string
	PKIDir     string
	ServerName string
	Deadline   time.Duration
}

type Client struct {
	conn     *grpc.ClientConn
	stub     catalogv1.CatalogServiceClient
	deadline time.Duration
}

var _ search.Source[search.ProductDoc] = (*Client)(nil)

func Dial(s Settings) (*Client, error) {
	creds, err := mutualTLS(s)

	if err != nil {
		return nil, err
	}

	conn, err := grpc.NewClient(s.Endpoint, grpc.WithTransportCredentials(creds))

	if err != nil {
		return nil, search.Errf(
			search.KindDependencyUnavailable,
			"catalogclient.Dial",
			err,
			"dialing %s",
			s.Endpoint,
		)
	}

	deadline := s.Deadline

	if deadline <= 0 {
		deadline = 10 * time.Second
	}

	return &Client{
		conn:     conn,
		stub:     catalogv1.NewCatalogServiceClient(conn),
		deadline: deadline,
	}, nil
}

func (c *Client) Close() error {
	return c.conn.Close()
}

func (c *Client) ChangedSince(
	ctx context.Context,
	cur search.Cursor,
	limit int,
) (search.Changes[search.ProductDoc], error) {
	ctx, cancel := context.WithTimeout(ctx, c.deadline)
	defer cancel()

	response, err := c.stub.ChangedSince(ctx, &catalogv1.ChangedSinceRequest{
		Cursor: ToProtoCursor(cur),
		Limit:  int32(limit), //nolint:gosec // the use case caps limit far below int32
	})
	if err != nil {
		return search.Changes[search.ProductDoc]{}, fail(
			"catalogclient.ChangedSince",
			err,
			"paging from %q",
			cur.LastID,
		)
	}

	upserted := make([]search.ProductDoc, 0, len(response.GetUpserted()))
	for _, p := range response.GetUpserted() {
		doc, convErr := ToDoc(p)
		if convErr != nil {
			return search.Changes[search.ProductDoc]{}, convErr
		}
		upserted = append(upserted, doc)
	}

	return search.Changes[search.ProductDoc]{
		Upserted: upserted,
		Removed:  response.GetRemovedSkus(),
		Next:     FromProtoCursor(response.GetNext()),
		HasMore:  response.GetHasMore(),
	}, nil
}

func (c *Client) Count(ctx context.Context) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, c.deadline)
	defer cancel()

	response, err := c.stub.CountProducts(ctx, &catalogv1.CountProductsRequest{})
	if err != nil {
		return 0, fail("catalogclient.Count", err, "counting products")
	}
	return response.GetTotal(), nil
}

func mutualTLS(s Settings) (credentials.TransportCredentials, error) {
	if s.PKIDir == "" {
		return nil, search.Errf(
			search.KindDependencyUnavailable,
			"catalogclient.Dial",
			nil,
			"no PKI directory configured",
		)
	}

	certPEM, err := readPKI(s.PKIDir, "tls.crt")

	if err != nil {
		return nil, err
	}

	keyPEM, err := readPKI(s.PKIDir, "tls.key")
	if err != nil {
		return nil, err
	}

	caPEM, err := readPKI(s.PKIDir, "ca.pem")
	if err != nil {
		return nil, err
	}

	pair, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, search.Errf(
			search.KindDependencyUnavailable,
			"catalogclient.Dial",
			err,
			"tls.crt and tls.key are not a pair",
		)
	}

	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(caPEM) {
		return nil, search.Errf(
			search.KindDependencyUnavailable,
			"catalogclient.Dial",
			nil,
			"ca.pem holds no usable certificate",
		)
	}

	return credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{pair},
		RootCAs:      roots,
		ServerName:   s.ServerName,
		MinVersion:   tls.VersionTLS13,
	}), nil
}

func fail(op string, err error, format string, args ...any) error {
	kind := search.KindDependencyUnavailable
	if status.Code(err) == codes.InvalidArgument {
		kind = search.KindMalformedQuery
	}
	return search.Errf(kind, op, err, format, args...)
}

func readPKI(dir, name string) ([]byte, error) {
	path := filepath.Join(dir, name)
	data, err := os.ReadFile(path) //nolint:gosec // the path comes from configuration, not a request
	if err != nil {
		return nil, search.Errf(
			search.KindDependencyUnavailable,
			"catalogclient.Dial",
			err,
			"reading %s",
			path,
		)
	}
	if len(data) == 0 {
		return nil, search.Errf(
			search.KindDependencyUnavailable,
			"catalogclient.Dial",
			nil,
			"%s is empty",
			path,
		)
	}
	return data, nil
}
