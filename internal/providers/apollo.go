package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

type ApolloAdapter struct{}

func (a *ApolloAdapter) ProviderID() string { return "apollo" }
func (a *ApolloAdapter) Catalog() CatalogEntry {
	return CatalogEntry{
		Provider: "apollo", Label: "Apollo.io", Category: "B2B Database",
		Description: "Massive B2B contact & company database with email + phone",
		CostNote:    "$49/mo — use your own key", RequiresKey: true,
	}
}

func (a *ApolloAdapter) TestKey(ctx context.Context, key string) (bool, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", "https://api.apollo.io/v1/auth/health", nil)
	req.Header.Set("X-Api-Key", key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	return resp.StatusCode == 200, nil
}

func (a *ApolloAdapter) Search(ctx context.Context, f SearchFilters, apiKey string) ([]NormalizedLead, error) {
	if apiKey == "" {
		return nil, nil
	}
	limit := f.Limit
	if limit <= 0 {
		limit = 1000
	}

	var all []NormalizedLead
	page := 1
	for len(all) < limit {
		perPage := 100
		if remaining := limit - len(all); remaining < perPage {
			perPage = remaining
		}
		body := map[string]any{
			"per_page": perPage,
			"page":     page,
		}
		if f.Niche != "" {
			body["q_organization_keyword_tags"] = []string{f.Niche}
		}
		if f.Location != "" {
			body["organization_locations"] = []string{f.Location}
		}
		if f.JobTitle != "" {
			body["person_titles"] = []string{f.JobTitle}
		}
		if f.CompanySize != "" {
			body["organization_num_employees_ranges"] = []string{f.CompanySize}
		}
		if f.Keyword != "" {
			body["q_keywords"] = f.Keyword
		}

		raw, _ := json.Marshal(body)
		req, _ := http.NewRequestWithContext(ctx, "POST",
			"https://api.apollo.io/v1/mixed_people/search",
			bytes.NewReader(raw))
		req.Header.Set("X-Api-Key", apiKey)
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
			People []struct {
				FirstName    string `json:"first_name"`
				LastName     string `json:"last_name"`
				Title        string `json:"title"`
				Email        string `json:"email"`
				LinkedinURL  string `json:"linkedin_url"`
				City         string `json:"city"`
				PhoneNumbers []struct {
					RawNumber string `json:"raw_number"`
				} `json:"phone_numbers"`
				Organization *struct {
					Name            string   `json:"name"`
					WebsiteURL      string   `json:"website_url"`
					City            string   `json:"city"`
					EmployeeCount   int      `json:"employee_count"`
					TechnologyNames []string `json:"technology_names"`
					Industry        string   `json:"industry"`
					ShortDescription string  `json:"short_description"`
				} `json:"organization"`
			} `json:"people"`
			Pagination struct {
				TotalEntries int `json:"total_entries"`
				Page         int `json:"page"`
				PerPage      int `json:"per_page"`
			} `json:"pagination"`
		}
		json.NewDecoder(resp.Body).Decode(&data)

		if len(data.People) == 0 {
			break
		}
		for _, p := range data.People {
			phone := ""
			if len(p.PhoneNumbers) > 0 {
				phone = p.PhoneNumbers[0].RawNumber
			}
			contact := p.FirstName + " " + p.LastName
			orgName, orgSite, orgLoc, orgSize := "", "", "", ""
			var techStack []string
			orgIndustry := ""
			orgDesc := ""
			if p.Organization != nil {
				orgName = p.Organization.Name
				orgSite = p.Organization.WebsiteURL
				orgLoc = p.Organization.City
				if orgLoc == "" {
					orgLoc = p.City
				}
				if p.Organization.EmployeeCount > 0 {
					orgSize = fmt.Sprintf("%d", p.Organization.EmployeeCount)
				}
				techStack = p.Organization.TechnologyNames
				orgIndustry = p.Organization.Industry
				orgDesc = p.Organization.ShortDescription
			}
			score := 15
			if p.Email != "" {
				score += 10
			}
			if phone != "" {
				score += 5
			}
			all = append(all, NormalizedLead{
				Source:        "apollo",
				BusinessName:  orgName,
				Website:       orgSite,
				Email:         p.Email,
				Phone:         phone,
				ContactName:   contact,
				ContactTitle:  p.Title,
				CompanySize:   orgSize,
				Location:      orgLoc,
				LinkedinURL:   p.LinkedinURL,
				TechStack:     techStack,
				Description:   orgDesc,
				Industry:      orgIndustry,
				IntentSignals: []string{"b2b_database", "verified_contact"},
				IntentScore:   score,
			})
		}
		total := data.Pagination.TotalEntries
		if total > 0 && len(all) >= total {
			break
		}
		if len(data.People) < perPage {
			break
		}
		page++
	}
	return all, nil
}
