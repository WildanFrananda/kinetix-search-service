package search

import "time"

type Hit[D any] struct {
	Score float64
	Doc   D
}

type FacetValue struct {
	Value string
	Count int64
}

type Freshness struct {
	IndexedThrough time.Time
	Age            time.Duration
	Stale          bool
}

type Results[D any] struct {
	Hits      []Hit[D]
	Total     int64
	Facets    map[string][]FacetValue
	Freshness Freshness
}
