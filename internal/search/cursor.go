package search

import "time"

type Generation string

type Cursor struct {
	UpdatedThrough time.Time
	LastID         string
}

type Changes[D any] struct {
	Upserted []D
	Removed  []string
	Refused  []Refusal
	Next     Cursor
	HasMore  bool
}
