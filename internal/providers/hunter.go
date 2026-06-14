package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

type HunterAdapter struct{}

func (a *HunterAdapter) ProviderID() string { return "hunter" }
func (a *HunterAdapter) Catalog() CatalogEntry {
	return CatalogEntry{
		Provider: "hunter", Label: "Hunter.io", Category: "B2B Database",
		Description: "Find and verify email addresses for any company",
		CostNote:    "$49/mo — use your own key", RequiresKey: true,
	}
}

func (a *HunterAdapter) TestKey(ctx context.Context, key string) (bool, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET",
		"https://api.hunter.io/v2/account?api_key="+key, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	var data struct {
		Data struct{ Email string `json:"email"` } `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&data)
	return data.Data.Email != "", nil
}

func (a *HunterAdapter) Search(ctx context.Context, f SearchFilters, apiKey string) ([]NormalizedLead, error) {
	if apiKey == "" {
		return nil, nil
	}
	limit := f.Limit
	if limit <= 0 {
		limit = 200
	}
	keyword := f.Niche
	if keyword == "" {
		keyword = f.Keyword
	}
	if keyword == "" {
		keyword = "saas"
	}

	var all []NormalizedLead
	offset := 0
	for len(all) < limit {
		perPage := 100
		if remaining := limit - len(all); remaining < perPage {
			perPage = remaining
		}
		u := fmt.Sprintf(
			"https://api.hunter.io/v2/companies/suggest?query=%s&limit=%d&offset=%d&api_key=%s",
			url.QueryEscape(keyword), perPage, offset, apiKey,
		)
		req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			break
		}
		defer resp.Body.Close()

		var data struct {
			Data struct {
				Companies []struct {
					Name    string `json:"name"`
					Domain  string `json:"domain"`
					Country string `json:"country"`
					Size    string `json:"size"`
				} `json:"companies"`
			} `json:"data"`
		}
		json.NewDecoder(resp.Body).Decode(&data)
		if len(data.Data.Companies) == 0 {
			break
		}
		for _, c := range data.Data.Companies {
			website := ""
			if c.Domain != "" {
				website = "https://" + c.Domain
			}
			all = append(all, NormalizedLead{
				Source:        "hunter",
				BusinessName:  c.Name,
				Website:       website,
				Location:      c.Country,
				CompanySize:   c.Size,
				IntentSignals: []string{"email_database"},
				IntentScore:   12,
			})
		}
		if len(data.Data.Companies) < perPage {
			break
		}
		offset += perPage
	}
	return all, nil
}
