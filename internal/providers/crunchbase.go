package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

type CrunchbaseAdapter struct{}

func (a *CrunchbaseAdapter) ProviderID() string { return "crunchbase" }
func (a *CrunchbaseAdapter) Catalog() CatalogEntry {
	return CatalogEntry{
		Provider: "crunchbase", Label: "Crunchbase", Category: "Startups & Tech",
		Description: "Funded startups, founding team, and funding rounds",
		CostNote:    "$29/mo — use your own key", RequiresKey: true,
	}
}

func (a *CrunchbaseAdapter) TestKey(ctx context.Context, key string) (bool, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET",
		"https://api.crunchbase.com/api/v4/entities/organizations/apple?user_key="+key+"&field_ids=short_description",
		nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	return resp.StatusCode == 200, nil
}

func (a *CrunchbaseAdapter) Search(ctx context.Context, f SearchFilters, apiKey string) ([]NormalizedLead, error) {
	if apiKey == "" {
		return nil, nil
	}
	limit := f.Limit
	if limit <= 0 {
		limit = 200
	}

	var all []NormalizedLead
	after := ""

	for len(all) < limit {
		perPage := 25
		body := map[string]any{
			"field_ids": []string{
				"identifier", "short_description", "website_url",
				"location_identifiers", "num_employees_enum",
				"funding_total", "founded_on",
			},
			"query": []map[string]any{
				{"type": "predicate", "field_id": "facet_ids", "operator_id": "includes", "values": []string{"company"}},
			},
			"limit": perPage,
		}
		if f.Niche != "" {
			body["query"] = append(body["query"].([]map[string]any), map[string]any{
				"type": "predicate", "field_id": "category_groups",
				"operator_id": "includes", "values": []string{f.Niche},
			})
		}
		if after != "" {
			body["after_id"] = after
		}

		raw, _ := json.Marshal(body)
		req, _ := http.NewRequestWithContext(ctx, "POST",
			fmt.Sprintf("https://api.crunchbase.com/api/v4/searches/organizations?user_key=%s", apiKey),
			bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			break
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			break
		}

		var data struct {
			Entities []struct {
				UUID       string `json:"uuid"`
				Properties struct {
					Identifier          struct{ Value string `json:"value"` }                `json:"identifier"`
					WebsiteURL          string                                               `json:"website_url"`
					LocationIdentifiers []struct{ Value string `json:"value"` }              `json:"location_identifiers"`
					NumEmployeesEnum    string                                               `json:"num_employees_enum"`
					FundingTotal        *struct{ ValueUSD float64 `json:"value_usd"` }       `json:"funding_total"`
				} `json:"properties"`
			} `json:"entities"`
			Count int `json:"count"`
		}
		json.NewDecoder(resp.Body).Decode(&data)
		if len(data.Entities) == 0 {
			break
		}

		for _, e := range data.Entities {
			p := e.Properties
			loc := ""
			if len(p.LocationIdentifiers) > 0 {
				loc = p.LocationIdentifiers[0].Value
			}
			score := 15
			signals := []string{"startup"}
			if p.FundingTotal != nil && p.FundingTotal.ValueUSD > 0 {
				score = 35
				signals = []string{"funded_startup"}
			}
			all = append(all, NormalizedLead{
				Source:        "crunchbase",
				BusinessName:  p.Identifier.Value,
				Website:       p.WebsiteURL,
				Location:      loc,
				CompanySize:   p.NumEmployeesEnum,
				IntentSignals: signals,
				IntentScore:   score,
			})
			after = e.UUID
		}
		if len(data.Entities) < perPage {
			break
		}
	}
	return all, nil
}
