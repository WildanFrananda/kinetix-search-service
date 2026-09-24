package mesh_test

import (
	"crypto/x509"
	"crypto/x509/pkix"
	"net/url"
	"os"
	"testing"

	"github.com/WildanFrananda/kinetix-search-service/internal/grpcapi"
	"github.com/WildanFrananda/kinetix-search-service/internal/mesh"
)

func TestAnUnsetVariableIsTheDomainTheEstateRunsToday(t *testing.T) {
	t.Setenv("KINETIX_TRUST_DOMAIN", "")

	got := mesh.TrustDomains()
	if len(got) != 1 || got[0] != "kinetix.local" {
		t.Fatalf("want [kinetix.local], got %v", got)
	}
}

func TestACutoverCanAcceptBothDomainsAtOnce(t *testing.T) {
	t.Setenv("KINETIX_TRUST_DOMAIN", "kinetix.local, prod.kinetix ")

	domains := mesh.TrustDomains()
	if len(domains) != 2 {
		t.Fatalf("want two domains, got %v", domains)
	}

	for _, domain := range domains {
		id := "spiffe://" + domain + "/service/order"
		if name := mesh.ServiceIn(id, domains); name != "order" {
			t.Fatalf("%s was not placed: %q", id, name)
		}
	}
}

func TestADomainOutsideTheListIsRefused(t *testing.T) {
	domains := []string{"kinetix.local", "prod.kinetix"}

	if name := mesh.ServiceIn("spiffe://staging.kinetix/service/order", domains); name != "" {
		t.Fatalf("a foreign domain was placed as %q", name)
	}
}

func TestADomainThisOneIsMerelyAPrefixOfIsRefused(t *testing.T) {
	if name := mesh.ServiceOf("spiffe://kinetix.local.example.com/service/order", "kinetix.local"); name != "" {
		t.Fatalf("a look-alike domain was placed as %q", name)
	}
}

func TestAPathIsNotAServiceName(t *testing.T) {
	for _, id := range []string{
		"spiffe://kinetix.local/service/a/b",
		"spiffe://kinetix.local/service/",
		"spiffe://kinetix.local/agent/order",
	} {
		if name := mesh.ServiceOf(id, "kinetix.local"); name != "" {
			t.Fatalf("%s was placed as %q", id, name)
		}
	}
}

func TestTheUriSanIsReadAndTheCommonNameIsNot(t *testing.T) {
	uri, err := url.Parse("spiffe://kinetix.local/service/order")
	if err != nil {
		t.Fatal(err)
	}

	leaf := &x509.Certificate{Subject: mustSubject("warehouse"), URIs: []*url.URL{uri}}

	if name := mesh.PeerService([]*x509.Certificate{leaf}, []string{"kinetix.local"}); name != "order" {
		t.Fatalf("want order, got %q", name)
	}
}

func TestACertificateWithNoUriSanNamesNobody(t *testing.T) {
	leaf := &x509.Certificate{Subject: mustSubject("order")}

	if name := mesh.PeerService([]*x509.Certificate{leaf}, []string{"kinetix.local"}); name != "" {
		t.Fatalf("a bare common name was placed as %q", name)
	}
}

func TestNoCertificateNamesNobody(t *testing.T) {
	if name := mesh.PeerService(nil, []string{"kinetix.local"}); name != "" {
		t.Fatalf("nothing was placed as %q", name)
	}
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

func mustSubject(commonName string) pkix.Name {
	return pkix.Name{CommonName: commonName}
}

func TestAnEmptyAllowListPermitsNobody(t *testing.T) {
	allowed := grpcapi.AllowedPeers("")

	if len(allowed) != 0 {
		t.Fatalf("an empty list named %v", allowed)
	}
}

func TestAnAllowListIsReadAsACommaSeparatedSet(t *testing.T) {
	allowed := grpcapi.AllowedPeers(" order , backoffice ,")

	if _, ok := allowed["order"]; !ok {
		t.Fatal("order is not on the list")
	}
	if _, ok := allowed["backoffice"]; !ok {
		t.Fatal("backoffice is not on the list")
	}
	if len(allowed) != 2 {
		t.Fatalf("want two names, got %v", allowed)
	}
}
