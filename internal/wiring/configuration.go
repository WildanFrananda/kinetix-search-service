package wiring

import (
	"github.com/WildanFrananda/kinetix-search-service/internal/config"
	"github.com/WildanFrananda/kinetix-search-service/internal/search"
	"github.com/WildanFrananda/kinetix-search-service/internal/system"
)

func provideConfig() (config.Config, error) {
	return config.FromEnvironment()
}

func provideClock() search.Clock {
	return system.Clock{}
}
