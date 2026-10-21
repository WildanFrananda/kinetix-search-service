package search

const DefaultSuggestions = 10

type SuggestQuery struct {
	Prefix string
	Limit  int
}

func NewSuggestQuery(prefix string, limit int) (SuggestQuery, error) {
	const op = "search.NewSuggestQuery"

	trimmed, err := searchable(op, "prefix", prefix)
	if err != nil {
		return SuggestQuery{}, err
	}

	size, err := pageSize(op, limit, DefaultSuggestions)
	if err != nil {
		return SuggestQuery{}, err
	}

	return SuggestQuery{Prefix: trimmed, Limit: size}, nil
}
