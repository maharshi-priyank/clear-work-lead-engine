package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

type ProxycurlAdapter struct{}

func (a *ProxycurlAdapter) ProviderID() string { return "proxycurl" }
func (a *ProxycurlAdapter) Catalog() CatalogEntry {
	return CatalogEntry{
		Provider: "proxycurl", Label: "Proxycurl", Category: "LinkedIn",
		Description: "LinkedIn profiles and company data via API",
		CostNote:    "$0.01/lookup — use your own key", RequiresKey: true,
	}
}

func (a *ProxycurlAdapter) TestKey(ctx context.Context, key string) (bool, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET",
		"https://nubela.co/proxycurl/api/credit-balance", nil)
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	return resp.StatusCode == 200, nil
}

func (a *ProxycurlAdapter) Search(ctx context.Context, f SearchFilters, apiKey string) ([]NormalizedLead, error) {
	if apiKey == "" {
		return nil, nil
	}
	limit := f.Limit
	if limit <= 0 {
		limit = 100
	}
	industry := f.Niche
	if industry == "" {
		industry = "Technology"
	}

	var all []NormalizedLead
	nextPageURL := ""
	base := "https://nubela.co/proxycurl/api/linkedin/company/search"

	for len(all) < limit {
		var u string
		if nextPageURL != "" {
			u = nextPageURL
		} else {
			params := url.Values{}
			params.Set("country", "IN")
			params.Set("industry", industry)
			params.Set("page_size", fmt.Sprintf("%d", proxycurlMin(100, limit-len(all))))
			if f.CompanySize != "" {
				params.Set("employee_count_max", f.CompanySize)
			}
			u = base + "?" + params.Encode()
		}

		req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
		req.Header.Set("Authorization", "Bearer "+apiKey)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			break
		}
		defer resp.Body.Close()

		var data struct {
			Results []struct {
				Name    string `json:"name"`
				Website string `json:"website"`
				URL     string `json:"url"`
				HQ      *struct {
					City string `json:"city"`
				} `json:"hq"`
				CompanySize string `json:"company_size_on_linkedin"`
			} `json:"results"`
			NextPage string `json:"next_page"`
		}
		json.NewDecoder(resp.Body).Decode(&data)
		if len(data.Results) == 0 {
			break
		}
		for _, c := range data.Results {
			loc := ""
			if c.HQ != nil {
				loc = c.HQ.City
			}
			all = append(all, NormalizedLead{
				Source:        "proxycurl",
				BusinessName:  c.Name,
				Website:       c.Website,
				LinkedinURL:   c.URL,
				Location:      loc,
				CompanySize:   c.CompanySize,
				IntentSignals: []string{"linkedin_company"},
				IntentScore:   18,
			})
		}
		nextPageURL = data.NextPage
		if nextPageURL == "" {
			break
		}
	}
	return all, nil
}

func proxycurlMin(a, b int) int {
	if a < b {
		return a
	}
	return b
}
