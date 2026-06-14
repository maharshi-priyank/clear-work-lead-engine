package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

type GithubAdapter struct{}

func (a *GithubAdapter) ProviderID() string { return "github" }
func (a *GithubAdapter) Catalog() CatalogEntry {
	return CatalogEntry{
		Provider: "github", Label: "GitHub", Category: "Startups & Tech",
		Description: "Find open-source orgs, tech startups, and active developers",
		CostNote:    "Free with PAT (5000 req/hr authenticated)",
		RequiresKey: true,
	}
}

func (a *GithubAdapter) TestKey(ctx context.Context, key string) (bool, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", "https://api.github.com/user", nil)
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("User-Agent", "ClearWork-LeadFinder")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	return resp.StatusCode == 200, nil
}

func (a *GithubAdapter) Search(ctx context.Context, f SearchFilters, apiKey string) ([]NormalizedLead, error) {
	query := buildQuery(f.Niche, f.Keyword, f.Location)
	if query == "" {
		query = "saas startup"
	}
	limit := f.Limit
	if limit <= 0 {
		limit = 100
	}

	headers := map[string]string{"User-Agent": "ClearWork-LeadFinder"}
	if apiKey != "" {
		headers["Authorization"] = "Bearer " + apiKey
	}

	var all []NormalizedLead
	page := 1
	for len(all) < limit {
		perPage := 100
		if remaining := limit - len(all); remaining < perPage {
			perPage = remaining
		}
		u := fmt.Sprintf(
			"https://api.github.com/search/users?q=%s+type:org&per_page=%d&page=%d",
			url.QueryEscape(query), perPage, page,
		)
		items, err := doGithubSearch(ctx, u, headers)
		if err != nil || len(items) == 0 {
			break
		}

		type result struct {
			lead NormalizedLead
			err  error
		}
		ch := make(chan result, len(items))
		for _, item := range items {
			go func(login string) {
				lead, err := enrichGithubOrg(ctx, login, headers)
				ch <- result{lead, err}
			}(item)
		}
		for range items {
			r := <-ch
			if r.err == nil {
				all = append(all, r.lead)
			}
		}
		if len(items) < perPage {
			break
		}
		page++
	}
	return all, nil
}

func doGithubSearch(ctx context.Context, u string, headers map[string]string) ([]string, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, nil
	}
	var data struct {
		Items []struct{ Login string `json:"login"` } `json:"items"`
	}
	json.NewDecoder(resp.Body).Decode(&data)
	logins := make([]string, 0, len(data.Items))
	for _, i := range data.Items {
		logins = append(logins, i.Login)
	}
	return logins, nil
}

func enrichGithubOrg(ctx context.Context, login string, headers map[string]string) (NormalizedLead, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", "https://api.github.com/orgs/"+login, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return NormalizedLead{Source: "github", BusinessName: login,
			Website: "https://github.com/" + login, IntentScore: 10, IntentSignals: []string{"github_org"}}, err
	}
	defer resp.Body.Close()
	var org struct {
		Name        string `json:"name"`
		Blog        string `json:"blog"`
		Email       string `json:"email"`
		Location    string `json:"location"`
		Description string `json:"description"`
		PublicRepos int    `json:"public_repos"`
		Followers   int    `json:"followers"`
	}
	json.NewDecoder(resp.Body).Decode(&org)
	name := org.Name
	if name == "" {
		name = login
	}
	website := org.Blog
	if website == "" {
		website = "https://github.com/" + login
	}
	score := 10
	if org.PublicRepos > 10 {
		score += 5
	}
	if org.Followers > 100 {
		score += 5
	}
	return NormalizedLead{
		Source:        "github",
		BusinessName:  name,
		Website:       website,
		Email:         org.Email,
		Location:      org.Location,
		Description:   org.Description,
		Industry:      "Technology",
		IntentSignals: []string{"github_org", "open_source"},
		IntentScore:   score,
	}, nil
}

func buildQuery(parts ...string) string {
	q := ""
	for _, p := range parts {
		if p != "" {
			if q != "" {
				q += " "
			}
			q += p
		}
	}
	return q
}
