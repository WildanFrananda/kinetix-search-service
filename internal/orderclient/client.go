package orderclient

import (
	"context"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	orderv1 "github.com/WildanFrananda/kinetix-search-service/internal/contractgen/order/v1"
	"github.com/WildanFrananda/kinetix-search-service/internal/mesh"
	"github.com/WildanFrananda/kinetix-search-service/internal/search"
)

type Client struct {
	conn     *grpc.ClientConn
	stub     orderv1.OrderServiceClient
	deadline time.Duration
}

var _ search.Source[search.OrderDoc] = (*Client)(nil)

func Dial(s mesh.Settings) (*Client, error) {
	creds, err := mesh.MutualTLS("orderclient.Dial", s)

	if err != nil {
		return nil, err
	}

	conn, err := grpc.NewClient(s.Endpoint, grpc.WithTransportCredentials(creds))

	if err != nil {
		return nil, search.Errf(
			search.KindDependencyUnavailable,
			"orderclient.Dial",
			err,
			"dialing %s",
			s.Endpoint,
		)
	}

	return &Client{
		conn:     conn,
		stub:     orderv1.NewOrderServiceClient(conn),
		deadline: s.DeadlineOr(10 * time.Second),
	}, nil
}

func (c *Client) Close() error {
	return c.conn.Close()
}

func (c *Client) ChangedSince(
	ctx context.Context,
	cur search.Cursor,
	limit int,
) (search.Changes[search.OrderDoc], error) {
	ctx, cancel := context.WithTimeout(ctx, c.deadline)
	defer cancel()

	response, err := c.stub.OrdersChangedSince(ctx, &orderv1.OrdersChangedSinceRequest{
		Cursor: ToProtoCursor(cur),
		Limit:  int32(limit), //nolint:gosec // the use case caps limit far below int32
	})

	if err != nil {
		return search.Changes[search.OrderDoc]{}, fail(
			"orderclient.ChangedSince",
			err,
			"paging from %q",
			cur.LastID,
		)
	}

	upserted := make([]search.OrderDoc, 0, len(response.GetUpserted()))

	for _, o := range response.GetUpserted() {
		doc, convErr := ToDoc(o)

		if convErr != nil {
			return search.Changes[search.OrderDoc]{}, convErr
		}

		upserted = append(upserted, doc)
	}

	return search.Changes[search.OrderDoc]{
		Upserted: upserted,
		Next:     FromProtoCursor(response.GetNext()),
		HasMore:  response.GetHasMore(),
	}, nil
}

func fail(op string, err error, format string, args ...any) error {
	kind := search.KindDependencyUnavailable

	if status.Code(err) == codes.InvalidArgument {
		kind = search.KindMalformedQuery
	}

	return search.Errf(kind, op, err, format, args...)
}
