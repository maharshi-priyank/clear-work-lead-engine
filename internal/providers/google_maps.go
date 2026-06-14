package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type GoogleMapsAdapter struct{}

func (a *GoogleMapsAdapter) ProviderID() string { return "google_maps" }
func (a *GoogleMapsAdapter) Catalog() CatalogEntry {
	return CatalogEntry{
		Provider: "google_maps", Label: "Google Maps", Category: "Local / Directory",
		Description: "Find local businesses via Google Maps API (AIza...) or SerpAPI key",
		CostNote:    "Google Maps API key (AIza...) or SerpAPI key", RequiresKey: true,
	}
}

func isGoogleMapsKey(key string) bool {
	return strings.HasPrefix(key, "AIza")
}

func (a *GoogleMapsAdapter) TestKey(ctx context.Context, key string) (bool, error) {
	if isGoogleMapsKey(key) {
		u := fmt.Sprintf(
			"https://maps.googleapis.com/maps/api/place/textsearch/json?query=test&key=%s", key,
		)
		req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return false, err
		}
		defer resp.Body.Close()
		var data struct{ Status string `json:"status"` }
		json.NewDecoder(resp.Body).Decode(&data)
		return data.Status != "REQUEST_DENIED", nil
	}
	req, _ := http.NewRequestWithContext(ctx, "GET",
		"https://serpapi.com/account?api_key="+key, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	var data struct{ AccountEmail string `json:"account_email"` }
	json.NewDecoder(resp.Body).Decode(&data)
	return data.AccountEmail != "", nil
}

func (a *GoogleMapsAdapter) Search(ctx context.Context, f SearchFilters, apiKey string) ([]NormalizedLead, error) {
	if apiKey == "" {
		return nil, nil
	}
	if isGoogleMapsKey(apiKey) {
		return a.searchViaPlacesAPI(ctx, f, apiKey)
	}
	return a.searchViaSerpAPI(ctx, f, apiKey)
}

// queriesForFilters expands a single niche into multiple search queries so we
// can pull more than the 60-result hard cap of one Places textsearch session.
func queriesForFilters(niche, location string) []string {
	base := niche + " in " + location
	// Generate a few query variants to multiply coverage
	variants := []string{
		base,
		niche + " company " + location,
		niche + " startup " + location,
		niche + " agency " + location,
		niche + " services " + location,
	}
	seen := map[string]bool{}
	out := []string{}
	for _, v := range variants {
		k := strings.ToLower(v)
		if !seen[k] {
			seen[k] = true
			out = append(out, v)
		}
	}
	return out
}

func (a *GoogleMapsAdapter) searchViaPlacesAPI(ctx context.Context, f SearchFilters, apiKey string) ([]NormalizedLead, error) {
	niche := firstSlug(f.Niche, f.Keyword, "software company")
	location := firstSlug(f.Location, "", "India")

	limit := f.Limit
	if limit <= 0 {
		limit = 60
	}

	queries := queriesForFilters(niche, location)
	seenName := map[string]bool{}
	var all []NormalizedLead

	for _, query := range queries {
		if len(all) >= limit {
			break
		}
		pageToken := ""
		firstPage := true

		for len(all) < limit {
			// Google requires ~2s delay before a nextPageToken becomes valid
			if !firstPage && pageToken != "" {
				time.Sleep(2 * time.Second)
			}
			firstPage = false

			u, _ := url.Parse("https://maps.googleapis.com/maps/api/place/textsearch/json")
			qp := u.Query()
			qp.Set("query", query)
			qp.Set("key", apiKey)
			if pageToken != "" {
				qp.Set("pagetoken", pageToken)
			}
			u.RawQuery = qp.Encode()

			req, _ := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				break
			}

			var data struct {
				Results []struct {
					PlaceID          string   `json:"place_id"`
					Name             string   `json:"name"`
					FormattedAddress string   `json:"formatted_address"`
					Types            []string `json:"types"`
				} `json:"results"`
				NextPageToken string `json:"next_page_token"`
				Status        string `json:"status"`
			}
			json.NewDecoder(resp.Body).Decode(&data)
			resp.Body.Close()

			for _, r := range data.Results {
				if seenName[strings.ToLower(r.Name)] {
					continue
				}
				seenName[strings.ToLower(r.Name)] = true

				phone, website, editorial := a.placeDetails(ctx, r.PlaceID, apiKey)
				industry := ""
				if len(r.Types) > 0 {
					industry = strings.ReplaceAll(r.Types[0], "_", " ")
				}
				all = append(all, NormalizedLead{
					Source:        "google_maps",
					BusinessName:  r.Name,
					Website:       website,
					Phone:         phone,
					Location:      r.FormattedAddress,
					Description:   editorial,
					Industry:      industry,
					IntentSignals: []string{"local_business", "google_maps"},
					IntentScore:   15,
				})
			}

			pageToken = data.NextPageToken
			if pageToken == "" || len(data.Results) == 0 {
				break
			}
		}
	}
	return all, nil
}

func (a *GoogleMapsAdapter) placeDetails(ctx context.Context, placeID, apiKey string) (phone, website, editorial string) {
	u := fmt.Sprintf(
		"https://maps.googleapis.com/maps/api/place/details/json?place_id=%s&fields=formatted_phone_number,website,editorial_summary&key=%s",
		placeID, apiKey,
	)
	req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", ""
	}
	defer resp.Body.Close()
	var data struct {
		Result struct {
			Phone           string `json:"formatted_phone_number"`
			Website         string `json:"website"`
			EditorialSummary struct {
				Overview string `json:"overview"`
			} `json:"editorial_summary"`
		} `json:"result"`
	}
	json.NewDecoder(resp.Body).Decode(&data)
	return data.Result.Phone, data.Result.Website, data.Result.EditorialSummary.Overview
}

func (a *GoogleMapsAdapter) searchViaSerpAPI(ctx context.Context, f SearchFilters, apiKey string) ([]NormalizedLead, error) {
	niche := firstSlug(f.Niche, f.Keyword, "software company")
	location := firstSlug(f.Location, "", "India")
	limit := f.Limit
	if limit <= 0 {
		limit = 60
	}

	var all []NormalizedLead
	start := 0

	for len(all) < limit {
		u, _ := url.Parse("https://serpapi.com/search")
		qp := u.Query()
		qp.Set("engine", "google_maps")
		qp.Set("q", niche+" company")
		qp.Set("location", location)
		qp.Set("start", fmt.Sprintf("%d", start))
		qp.Set("api_key", apiKey)
		u.RawQuery = qp.Encode()

		req, _ := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			break
		}
		defer resp.Body.Close()

		var data struct {
			LocalResults []struct {
				Title       string `json:"title"`
				Website     string `json:"website"`
				Phone       string `json:"phone"`
				Address     string `json:"address"`
				Type        string `json:"type"`
				Description string `json:"description"`
				Rating      float64 `json:"rating"`
			} `json:"local_results"`
		}
		json.NewDecoder(resp.Body).Decode(&data)
		if len(data.LocalResults) == 0 {
			break
		}
		for _, r := range data.LocalResults {
			desc := r.Description
			if r.Rating > 0 && desc == "" {
				desc = fmt.Sprintf("Rated %.1f/5 on Google Maps", r.Rating)
			}
			all = append(all, NormalizedLead{
				Source:        "google_maps",
				BusinessName:  r.Title,
				Website:       r.Website,
				Phone:         r.Phone,
				Location:      r.Address,
				Industry:      r.Type,
				Description:   desc,
				IntentSignals: []string{"local_business", "google_maps"},
				IntentScore:   10,
			})
		}
		if len(data.LocalResults) < 20 {
			break
		}
		start += 20
	}
	return all, nil
}
