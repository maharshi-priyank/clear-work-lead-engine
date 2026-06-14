package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

type ProductHuntAdapter struct{}

func (a *ProductHuntAdapter) ProviderID() string { return "product_hunt" }
func (a *ProductHuntAdapter) Catalog() CatalogEntry {
	return CatalogEntry{
		Provider: "product_hunt", Label: "Product Hunt", Category: "Startups & Tech",
		Description: "Recently launched tech startups and products",
		CostNote:    "Free with developer token", RequiresKey: true,
	}
}

func (a *ProductHuntAdapter) TestKey(ctx context.Context, key string) (bool, error) {
	body := `{"query":"{ viewer { user { id } } }"}`
	req, _ := http.NewRequestWithContext(ctx, "POST",
		"https://api.producthunt.com/v2/api/graphql",
		bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	return resp.StatusCode == 200, nil
}

func (a *ProductHuntAdapter) Search(ctx context.Context, f SearchFilters, apiKey string) ([]NormalizedLead, error) {
	if apiKey == "" {
		return nil, nil
	}
	limit := f.Limit
	if limit <= 0 {
		limit = 500
	}
	// Product Hunt topics must be single slugs — pick the first clean word
	topic := firstSlug(f.Niche, f.Keyword, "saas")

	var all []NormalizedLead
	cursor := ""

	for len(all) < limit {
		perPage := 50
		if remaining := limit - len(all); remaining < perPage {
			perPage = remaining
		}
		after := ""
		if cursor != "" {
			after = `, after: "` + cursor + `"`
		}
		gql := fmt.Sprintf(`{ posts(first: %d, topic: "%s"%s) {
			edges { node {
				name tagline description website
				votesCount commentsCount
				makers { name profileUrl twitterUsername }
				topics { edges { node { name } } }
			} cursor }
			pageInfo { hasNextPage endCursor }
		} }`, perPage, topic, after)

		raw, _ := json.Marshal(map[string]string{"query": gql})
		req, _ := http.NewRequestWithContext(ctx, "POST",
			"https://api.producthunt.com/v2/api/graphql",
			bytes.NewReader(raw))
		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			break
		}
		defer resp.Body.Close()

		var data struct {
			Data struct {
				Posts struct {
					Edges []struct {
						Node struct {
							Name          string `json:"name"`
							Tagline       string `json:"tagline"`
							Description   string `json:"description"`
							Website       string `json:"website"`
							VotesCount    int    `json:"votesCount"`
							CommentsCount int    `json:"commentsCount"`
							Makers        []struct {
								Name            string `json:"name"`
								ProfileURL      string `json:"profileUrl"`
								TwitterUsername string `json:"twitterUsername"`
							} `json:"makers"`
							Topics struct {
								Edges []struct {
									Node struct{ Name string `json:"name"` } `json:"node"`
								} `json:"edges"`
							} `json:"topics"`
						} `json:"node"`
						Cursor string `json:"cursor"`
					} `json:"edges"`
					PageInfo struct {
						HasNextPage bool   `json:"hasNextPage"`
						EndCursor   string `json:"endCursor"`
					} `json:"pageInfo"`
				} `json:"posts"`
			} `json:"data"`
		}
		json.NewDecoder(resp.Body).Decode(&data)

		edges := data.Data.Posts.Edges
		if len(edges) == 0 {
			break
		}
		for _, e := range edges {
			n := e.Node
			contact := ""
			contactURL := ""
			if len(n.Makers) > 0 {
				contact = n.Makers[0].Name
				contactURL = n.Makers[0].ProfileURL
			}

			// Build topic list for industry/tech stack
			var topics []string
			for _, t := range n.Topics.Edges {
				topics = append(topics, t.Node.Name)
			}

			// Combine tagline + description for notes
			desc := n.Tagline
			if n.Description != "" {
				if desc != "" {
					desc += " — " + n.Description
				} else {
					desc = n.Description
				}
			}
			if len(desc) > 500 {
				desc = desc[:500] + "..."
			}

			// Score based on community traction
			score := 20
			if n.VotesCount > 100 {
				score += 10
			}
			if n.VotesCount > 500 {
				score += 10
			}

			_ = contactURL
			all = append(all, NormalizedLead{
				Source:        "product_hunt",
				BusinessName:  n.Name,
				Website:       n.Website,
				ContactName:   contact,
				Description:   desc,
				Industry:      "Technology",
				TechStack:     topics,
				IntentSignals: []string{"recently_launched", "startup", "product_hunt_featured"},
				IntentScore:   score,
			})
			cursor = e.Cursor
		}
		if !data.Data.Posts.PageInfo.HasNextPage {
			break
		}
	}
	return all, nil
}
