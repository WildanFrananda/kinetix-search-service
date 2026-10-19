package search

import "time"

type Generation string

type Cursor struct {
	TransactionID  uint64
	LastID         string
	UpdatedThrough time.Time
}

type Changes[D any] struct {
	Upserted []D
	Removed  []string
	Refused  []Refusal
	Next     Cursor
	HasMore  bool
}
