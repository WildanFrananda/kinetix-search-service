package wiring

import (
	"go.uber.org/dig"

	"github.com/WildanFrananda/kinetix-search-service/internal/catalogclient"
	"github.com/WildanFrananda/kinetix-search-service/internal/config"
	"github.com/WildanFrananda/kinetix-search-service/internal/identityclient"
	"github.com/WildanFrananda/kinetix-search-service/internal/mesh"
	"github.com/WildanFrananda/kinetix-search-service/internal/orderclient"
	"github.com/WildanFrananda/kinetix-search-service/internal/search"
)

type sourceParams struct {
	dig.In
	Config config.Config
}

type catalogResult struct {
	dig.Out
	Client *catalogclient.Client
	Source search.Source[search.ProductDoc]
}

func provideCatalog(p sourceParams) (catalogResult, error) {
	client, err := catalogclient.Dial(mesh.Settings{
		Endpoint: p.Config.CatalogEndpoint,
		PKIDir:   p.Config.PKIDir,
		Deadline: p.Config.CatalogDeadline,
	})
	if err != nil {
		return catalogResult{}, err
	}

	return catalogResult{Client: client, Source: client}, nil
}

type identityResult struct {
	dig.Out
	Client *identityclient.Client
	Source search.Source[search.MerchantDoc]
}

func provideIdentity(p sourceParams) (identityResult, error) {
	client, err := identityclient.Dial(mesh.Settings{
		Endpoint: p.Config.IdentityEndpoint,
		PKIDir:   p.Config.PKIDir,
		Deadline: p.Config.IdentityDeadline,
	})
	if err != nil {
		return identityResult{}, err
	}

	return identityResult{Client: client, Source: client}, nil
}

type orderResult struct {
	dig.Out
	Client *orderclient.Client
	Source search.Source[search.OrderDoc]
}

func provideOrders(p sourceParams) (orderResult, error) {
	client, err := orderclient.Dial(mesh.Settings{
		Endpoint: p.Config.OrderEndpoint,
		PKIDir:   p.Config.PKIDir,
		Deadline: p.Config.OrderDeadline,
	})
	if err != nil {
		return orderResult{}, err
	}

	return orderResult{Client: client, Source: client}, nil
}
