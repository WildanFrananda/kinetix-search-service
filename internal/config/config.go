package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	DatabaseURL      string
	TypesenseURL     string
	TypesenseAPIKey  string
	TypesenseTimeout time.Duration
	CatalogEndpoint  string
	CatalogDeadline  time.Duration
	IdentityEndpoint string
	IdentityDeadline time.Duration
	OrderEndpoint    string
	OrderDeadline    time.Duration
	PKIDir           string
	HTTPAddr         string
	GRPCAddr         string
	SyncInterval     time.Duration
	SyncPageSize     int
	StaleAfter       time.Duration
}

func FromEnvironment() (Config, error) {
	var missing []string

	cfg := Config{
		DatabaseURL:      required("SEARCH_DATABASE_URL", &missing),
		TypesenseURL:     required("SEARCH_TYPESENSE_URL", &missing),
		TypesenseAPIKey:  required("SEARCH_TYPESENSE_API_KEY", &missing),
		CatalogEndpoint:  required("SEARCH_CATALOG_ENDPOINT", &missing),
		IdentityEndpoint: required("SEARCH_IDENTITY_ENDPOINT", &missing),
		OrderEndpoint:    required("SEARCH_ORDER_ENDPOINT", &missing),
		PKIDir:           required("KINETIX_PKI_DIR", &missing),
		HTTPAddr:         optional("SEARCH_HTTP_ADDR", ":8088"),
		GRPCAddr:         optional("SEARCH_GRPC_ADDR", ":50058"),
		TypesenseTimeout: duration("SEARCH_TYPESENSE_TIMEOUT_MS", 5*time.Second),
		CatalogDeadline:  duration("SEARCH_CATALOG_DEADLINE_MS", 10*time.Second),
		IdentityDeadline: duration("SEARCH_IDENTITY_DEADLINE_MS", 10*time.Second),
		OrderDeadline:    duration("SEARCH_ORDER_DEADLINE_MS", 10*time.Second),
		SyncInterval:     duration("SEARCH_SYNC_INTERVAL_MS", 30*time.Second),
		StaleAfter:       duration("SEARCH_STALE_AFTER_MS", 10*time.Minute),
		SyncPageSize:     number("SEARCH_SYNC_PAGE_SIZE", 200),
	}

	if len(missing) > 0 {
		return Config{}, fmt.Errorf(
			"these environment variables must be set:\n  %s",
			strings.Join(missing, "\n  "),
		)
	}

	return cfg, nil
}

func required(name string, missing *[]string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		*missing = append(*missing, name)
	}
	return value
}

func optional(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}

	return fallback
}

func number(name string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func duration(name string, fallback time.Duration) time.Duration {
	return time.Duration(number(name, int(fallback/time.Millisecond))) * time.Millisecond
}
