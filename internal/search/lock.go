package search

import "context"

type Lock interface {
	Hold(ctx context.Context, c Collection) (func(), error)
}
