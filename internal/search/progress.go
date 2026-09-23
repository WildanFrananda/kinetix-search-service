package search

import "time"

type Progress struct {
	Cursor  Cursor
	SavedAt time.Time
}
