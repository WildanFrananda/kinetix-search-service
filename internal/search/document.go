package search

import "time"

type Collection string

const (
	Products  Collection = "products"
	Merchants Collection = "merchants"
	Orders    Collection = "orders"
)

type ProductDoc struct {
	ID          ProductID
	MerchantID  MerchantID
	Title       string
	Description string
	Categories  []string
	PriceMinor  int64
	Currency    string
	UpdatedAt   time.Time
}

type MerchantDoc struct {
	ID          MerchantID
	DisplayName string
	Categories  []string
	UpdatedAt   time.Time
}

type OrderDoc struct {
	ID         OrderID
	Buyer      MerchantID
	MerchantID MerchantID
	Number     string
	Status     string
	LineTitles []string
	PlacedAt   time.Time
	UpdatedAt  time.Time
}

func isControl(r rune) bool { return r < 0x20 || r == 0x7f }
