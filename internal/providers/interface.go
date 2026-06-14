package providers

import (
	"context"
	"strings"
)

type NormalizedLead struct {
	Source        string   `json:"source"`
	BusinessName  string   `json:"businessName,omitempty"`
	Website       string   `json:"website,omitempty"`
	Email         string   `json:"email,omitempty"`
	Phone         string   `json:"phone,omitempty"`
	ContactName   string   `json:"contactName,omitempty"`
	ContactTitle  string   `json:"contactTitle,omitempty"`
	CompanySize   string   `json:"companySize,omitempty"`
	Location      string   `json:"location,omitempty"`
	LinkedinURL   string   `json:"linkedinUrl,omitempty"`
	TechStack     []string `json:"techStack,omitempty"`
	IntentSignals []string `json:"intentSignals,omitempty"`
	IntentScore   int      `json:"intentScore,omitempty"`
	Description   string   `json:"description,omitempty"`  // what the company does / why they're a good lead
	Industry      string   `json:"industry,omitempty"`     // sector / vertical
}

type SearchFilters struct {
	Niche       string
	Location    string
	JobTitle    string
	CompanySize string
	Keyword     string
	Limit       int
}

type CatalogEntry struct {
	Provider    string `json:"provider"`
	Label       string `json:"label"`
	Category    string `json:"category"`
	Description string `json:"description"`
	CostNote    string `json:"costNote"`
	RequiresKey bool   `json:"requiresKey"`
}

// firstSlug picks the first non-empty trimmed token from comma-separated fields.
// Falls back to defaultVal if nothing found.
func firstSlug(fields ...string) string {
	last := fields[len(fields)-1]
	for _, f := range fields[:len(fields)-1] {
		for _, part := range strings.Split(f, ",") {
			s := strings.TrimSpace(part)
			if s != "" {
				return s
			}
		}
	}
	return last
}

type Adapter interface {
	ProviderID() string
	Catalog() CatalogEntry
	TestKey(ctx context.Context, key string) (bool, error)
	Search(ctx context.Context, filters SearchFilters, apiKey string) ([]NormalizedLead, error)
}
