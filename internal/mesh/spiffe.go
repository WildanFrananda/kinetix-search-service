package mesh

import (
	"crypto/x509"
	"os"
	"strings"
)

const defaultTrustDomain = "kinetix.local"

func TrustDomains() []string {
	configured := make([]string, 0, 2)

	for _, value := range strings.Split(os.Getenv("KINETIX_TRUST_DOMAIN"), ",") {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			configured = append(configured, trimmed)
		}
	}

	if len(configured) == 0 {
		return []string{defaultTrustDomain}
	}

	return configured
}

func ServiceOf(id string, domain string) string {
	prefix := "spiffe://" + domain + "/service/"

	if !strings.HasPrefix(id, prefix) {
		return ""
	}

	name := id[len(prefix):]
	if name == "" || strings.Contains(name, "/") {
		return ""
	}

	return name
}

func ServiceIn(id string, domains []string) string {
	for _, domain := range domains {
		if name := ServiceOf(id, domain); name != "" {
			return name
		}
	}

	return ""
}

func PeerService(certificates []*x509.Certificate, domains []string) string {
	if len(certificates) == 0 {
		return ""
	}

	for _, uri := range certificates[0].URIs {
		if name := ServiceIn(uri.String(), domains); name != "" {
			return name
		}
	}

	return ""
}
