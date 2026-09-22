package search

import "strings"

type ProductID struct {
	v string
}

func NewProductID(raw string) (ProductID, error) {
	return newID[ProductID](raw, "search.NewProductID")
}

func (p ProductID) String() string {
	return p.v
}

func (p ProductID) IsZero() bool {
	return p.v == ""
}

func (p ProductID) set(s string) ProductID {
	p.v = s; return p
}

type MerchantID struct {
	v string
}

func NewMerchantID(raw string) (MerchantID, error) {
	return newID[MerchantID](raw, "search.NewMerchantID")
}

func (m MerchantID) String() string {
	return m.v
}

func (m MerchantID) IsZero() bool {
	return m.v == ""
}

func (m MerchantID) set(s string) MerchantID {
	m.v = s; return m
}

type OrderID struct {
	v string
}

func NewOrderID(raw string) (OrderID, error) {
	return newID[OrderID](raw, "search.NewOrderID")
}

func (o OrderID) String() string {
	return o.v
}
func (o OrderID) IsZero() bool {
	return o.v == ""
}

func (o OrderID) set(s string) OrderID {
	o.v = s; return o
}

type identifier[T any] interface {
	set(string) T
}

const maxIDLength = 64

func newID[T identifier[T]](raw, op string) (T, error) {
	var zero T
	trimmed := strings.TrimSpace(raw)
	switch {
	case trimmed == "":
		return zero, Errf(KindMalformedQuery, op, nil, "empty identifier")
	case len(trimmed) > maxIDLength:
		return zero, Errf(KindMalformedQuery, op, nil, "identifier too long")
	case strings.ContainsFunc(trimmed, isControl):
		return zero, Errf(KindMalformedQuery, op, nil, "control byte in identifier")
	}
	return zero.set(trimmed), nil
}
