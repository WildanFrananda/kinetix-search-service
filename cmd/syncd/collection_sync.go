package main

import (
	"context"

	"github.com/WildanFrananda/kinetix-search-service/internal/search"
)

type collectionSync struct {
	collection search.Collection
	once       func(context.Context) error
}
