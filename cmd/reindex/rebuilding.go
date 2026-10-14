package main

import (
	"io"

	"go.uber.org/dig"

	"github.com/WildanFrananda/kinetix-search-service/internal/indexing"
)

type rebuilding[C io.Closer, D any] struct {
	dig.In
	Client    C
	Reindexer *indexing.Reindexer[D]
}
