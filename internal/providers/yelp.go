package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

type YelpAdapter struct{}

func (a *YelpAdapter) ProviderID() string { return "yelp" }
func (a *YelpAdapter) Catalog() CatalogEntry {
	return CatalogEntry{
		Provider: "yelp", Label: "Yelp Fusion", Category: "Local / Directory",
		Description: "Local businesses with ratings, category, and contact info",
		CostNote:    "Free — 500 calls/day", RequiresKey: true,
	}
}

func (a *YelpAdapter) TestKey(ctx context.Context, key string) (bool, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET",
		"https://api.yelp.com/v3/businesses/search?term=cafe&location=NYC&limit=1", nil)
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	return resp.StatusCode == 200, nil
}

func (a *YelpAdapter) Search(ctx context.Context, f SearchFilters, apiKey string) ([]NormalizedLead, error) {
	if apiKey == "" {
		return nil, nil
	}
	term := strings.TrimSpace(f.Niche + " " + f.Keyword)
	if term == "" {
		term = "business"
	}
	location := f.Location
	if location == "" {
		location = "New York"
	}
	limit := f.Limit
	if limit <= 0 {
		limit = 200
	}

	var all []NormalizedLead
	offset := 0

	for len(all) < limit {
		perPage := 50
		if remaining := limit - len(all); remaining < perPage {
			perPage = remaining
		}
		u, _ := url.Parse("https://api.yelp.com/v3/businesses/search")
		q := u.Query()
		q.Set("term", term)
		q.Set("location", location)
		q.Set("limit", fmt.Sprintf("%d", perPage))
		q.Set("offset", fmt.Sprintf("%d", offset))
		u.RawQuery = q.Encode()

		req, _ := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
		req.Header.Set("Authorization", "Bearer "+apiKey)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			break
		}
		defer resp.Body.Close()

		var data struct {
			Businesses []struct {
				Name     string `json:"name"`
				URL      string `json:"url"`
				Phone    string `json:"phone"`
				Location struct {
					DisplayAddress []string `json:"display_address"`
				} `json:"location"`
			} `json:"businesses"`
		}
		json.NewDecoder(resp.Body).Decode(&data)
		if len(data.Businesses) == 0 {
			break
		}
		for _, b := range data.Businesses {
			all = append(all, NormalizedLead{
				Source:        "yelp",
				BusinessName:  b.Name,
				Website:       b.URL,
				Phone:         b.Phone,
				Location:      strings.Join(b.Location.DisplayAddress, ", "),
				IntentSignals: []string{"local_business"},
				IntentScore:   8,
			})
		}
		if len(data.Businesses) < perPage {
			break
		}
		offset += perPage
	}
	return all, nil
}
