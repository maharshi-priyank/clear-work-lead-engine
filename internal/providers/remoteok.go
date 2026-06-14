package providers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
)

type RemoteOKAdapter struct{}

func (a *RemoteOKAdapter) ProviderID() string { return "remoteok" }
func (a *RemoteOKAdapter) Catalog() CatalogEntry {
	return CatalogEntry{
		Provider: "remoteok", Label: "RemoteOK", Category: "Job Boards",
		Description: "Companies actively hiring remotely — strong hiring intent signal",
		CostNote:    "Free — no key needed", RequiresKey: false,
	}
}
func (a *RemoteOKAdapter) TestKey(_ context.Context, _ string) (bool, error) { return true, nil }

func (a *RemoteOKAdapter) Search(ctx context.Context, f SearchFilters, _ string) ([]NormalizedLead, error) {
	// Pick the best single tag from niche/keyword for the API URL
	tag := firstSlug(f.Niche, f.Keyword, "saas")
	tag = strings.ToLower(strings.ReplaceAll(tag, " ", "-"))

	apiURL := "https://remoteok.com/api?tag=" + url.QueryEscape(tag)
	req, _ := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	req.Header.Set("User-Agent", "ClearWork-LeadFinder")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var jobs []map[string]any
	json.NewDecoder(resp.Body).Decode(&jobs)

	// Build a list of all keywords to match against
	rawKeywords := strings.Split(f.Niche+","+f.Keyword, ",")
	var keywords []string
	for _, k := range rawKeywords {
		k = strings.TrimSpace(strings.ToLower(k))
		if k != "" {
			keywords = append(keywords, k)
		}
	}

	seen := make(map[string]bool)
	var leads []NormalizedLead
	limit := f.Limit
	if limit <= 0 {
		limit = 500
	}

	for _, job := range jobs {
		if len(leads) >= limit {
			break
		}
		company, _ := job["company"].(string)
		if company == "" || seen[company] {
			continue
		}

		// Filter by keywords if specified
		if len(keywords) > 0 {
			pos, _ := job["position"].(string)
			desc, _ := job["description"].(string)
			tagsRaw, _ := job["tags"].([]any)
			haystack := strings.ToLower(pos + " " + company + " " + desc)
			for _, t := range tagsRaw {
				if ts, ok := t.(string); ok {
					haystack += " " + strings.ToLower(ts)
				}
			}
			match := false
			for _, kw := range keywords {
				if strings.Contains(haystack, kw) {
					match = true
					break
				}
			}
			if !match {
				continue
			}
		}

		seen[company] = true

		// Extract rich fields
		position, _ := job["position"].(string)
		description, _ := job["description"].(string)
		// Use company_url if available; fall back to the job URL only as a last resort
		// but keep it separate so dedup can use business name instead of remoteok.com domain
		companyURL, _ := job["company_url"].(string)
		jobURL, _ := job["url"].(string)
		_ = jobURL

		// Build tech stack from tags
		tagsRaw, _ := job["tags"].([]any)
		var techStack []string
		for _, t := range tagsRaw {
			if ts, ok := t.(string); ok {
				techStack = append(techStack, ts)
			}
		}

		// Truncate description to 300 chars for notes
		if len(description) > 300 {
			description = description[:300] + "..."
		}

		// Truncate description for the Description field
		desc := description
		if len(desc) > 400 {
			desc = desc[:400] + "..."
		}

		leads = append(leads, NormalizedLead{
			Source:        "remoteok",
			BusinessName:  company,
			Website:       companyURL,
			ContactTitle:  position,
			Location:      "Remote",
			TechStack:     techStack,
			Description:   desc,
			Industry:      "Technology",
			IntentSignals: []string{"actively_hiring", "remote_first"},
			IntentScore:   25,
		})
	}
	return leads, nil
}
