package config_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/WildanFrananda/kinetix-search-service/internal/config"
)

func setMinimum(t *testing.T) {
	t.Helper()
	t.Setenv("SEARCH_DATABASE_URL", "postgres://search:pw@db:5432/search")
	t.Setenv("SEARCH_TYPESENSE_URL", "http://typesense:8108")
	t.Setenv("SEARCH_TYPESENSE_API_KEY", "key")
	t.Setenv("SEARCH_CATALOG_ENDPOINT", "kinetix-catalog-service:50058")
	t.Setenv("SEARCH_IDENTITY_ENDPOINT", "kinetix-identity-service:50051")
	t.Setenv("SEARCH_ORDER_ENDPOINT", "kinetix-order-service:50053")
	t.Setenv("KINETIX_PKI_DIR", "/pki")
}

func TestEveryMissingVariableIsNamedAtOnce(t *testing.T) {
	for _, name := range []string{
		"SEARCH_DATABASE_URL",
		"SEARCH_TYPESENSE_URL",
		"SEARCH_TYPESENSE_API_KEY",
		"SEARCH_CATALOG_ENDPOINT",
		"SEARCH_IDENTITY_ENDPOINT",
		"SEARCH_ORDER_ENDPOINT",
		"KINETIX_PKI_DIR",
	} {
		t.Setenv(name, "")
	}

	_, err := config.FromEnvironment()

	require.Error(t, err)
	for _, name := range []string{
		"SEARCH_DATABASE_URL",
		"SEARCH_TYPESENSE_URL",
		"SEARCH_TYPESENSE_API_KEY",
		"SEARCH_CATALOG_ENDPOINT",
		"SEARCH_IDENTITY_ENDPOINT",
		"SEARCH_ORDER_ENDPOINT",
		"KINETIX_PKI_DIR",
	} {
		require.Containsf(
			t,
			err.Error(),
			name,
			"a misconfigured deploy should be one restart to fix, not five",
		)
	}
}

func TestNothingThatNamesAnotherMachineHasADefault(t *testing.T) {
	setMinimum(t)
	t.Setenv("SEARCH_CATALOG_ENDPOINT", "")

	_, err := config.FromEnvironment()

	require.Error(t, err, "a default endpoint is a service that starts against the wrong catalog")
}

func TestWhitespaceIsNotAValue(t *testing.T) {
	setMinimum(t)
	t.Setenv("SEARCH_TYPESENSE_API_KEY", "   ")

	_, err := config.FromEnvironment()

	require.Error(t, err)
}

func TestTimeoutsAndSizesHaveSensibleDefaults(t *testing.T) {
	setMinimum(t)

	cfg, err := config.FromEnvironment()
	require.NoError(t, err)

	require.Equal(t, 30*time.Second, cfg.SyncInterval)
	require.Equal(t, 10*time.Minute, cfg.StaleAfter)
	require.Equal(t, 10*time.Second, cfg.CatalogDeadline)
	require.Equal(t, 200, cfg.SyncPageSize)
	require.Equal(t, "kinetix-catalog-service", cfg.CatalogServerName)
}

func TestDurationsAreReadInMilliseconds(t *testing.T) {
	setMinimum(t)
	t.Setenv("SEARCH_SYNC_INTERVAL_MS", "1500")

	cfg, err := config.FromEnvironment()
	require.NoError(t, err)
	require.Equal(t, 1500*time.Millisecond, cfg.SyncInterval)
}

func TestARubbishNumberFallsBackRatherThanRefusingToStart(t *testing.T) {
	setMinimum(t)
	t.Setenv("SEARCH_SYNC_PAGE_SIZE", "banyak")
	t.Setenv("SEARCH_SYNC_INTERVAL_MS", "-5")

	cfg, err := config.FromEnvironment()

	require.NoError(t, err, "a typo in a page size is not worth an outage")
	require.Equal(t, 200, cfg.SyncPageSize)
	require.Equal(t, 30*time.Second, cfg.SyncInterval)
}
