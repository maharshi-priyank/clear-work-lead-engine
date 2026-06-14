package campaign

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"sync"

	"github.com/amplexo/clearwork-leads-engine/internal/providers"
	"github.com/amplexo/clearwork-leads-engine/internal/vault"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Worker struct {
	db       *pgxpool.Pool
	registry *providers.Registry
	vault    *vault.Service
}

func NewWorker(db *pgxpool.Pool, registry *providers.Registry, vaultSvc *vault.Service) *Worker {
	return &Worker{db: db, registry: registry, vault: vaultSvc}
}

type jobPayload struct {
	CampaignID string   `json:"campaignId"`
	UserID     string   `json:"userId"`
	Providers  []string `json:"providers"`
	Filters    Filters  `json:"filters"`
}

func (w *Worker) ProcessCampaign(ctx context.Context, t *asynq.Task) error {
	var p jobPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("invalid payload: %w", err)
	}
	slog.Info("campaign worker started", "campaignId", p.CampaignID, "providers", p.Providers)

	targetCount := p.Filters.TargetCount
	if targetCount <= 0 {
		targetCount = 50
	}
	// Give each provider the full target — dedup removes overlaps.
	// Dividing by provider count caused under-collection when providers returned few results.
	perProvider := targetCount

	searchFilters := providers.SearchFilters{
		Niche:       p.Filters.Niche,
		Location:    p.Filters.Location,
		JobTitle:    p.Filters.JobTitle,
		CompanySize: p.Filters.CompanySize,
		Keyword:     p.Filters.Keyword,
		Limit:       perProvider,
	}

	type result struct {
		leads []providers.NormalizedLead
		err   error
	}
	ch := make(chan result, len(p.Providers))
	var wg sync.WaitGroup

	for _, providerID := range p.Providers {
		wg.Add(1)
		go func(pid string) {
			defer wg.Done()
			adapter := w.registry.Get(pid)
			if adapter == nil {
				slog.Warn("provider not found in registry", "provider", pid)
				ch <- result{}
				return
			}
			apiKey, err := w.vault.GetDecryptedKey(ctx, p.UserID, pid)
			if err != nil {
				slog.Warn("vault key error", "provider", pid, "err", err)
			}
			if adapter.Catalog().RequiresKey && apiKey == "" {
				slog.Warn("no API key saved for provider, skipping", "provider", pid)
				ch <- result{}
				return
			}
			slog.Info("searching provider", "provider", pid, "hasKey", apiKey != "", "limit", searchFilters.Limit)
			leads, err := adapter.Search(ctx, searchFilters, apiKey)
			slog.Info("provider search done", "provider", pid, "found", len(leads), "err", err)
			ch <- result{leads, err}
		}(providerID)
	}

	go func() { wg.Wait(); close(ch) }()

	var all []providers.NormalizedLead
	for r := range ch {
		if r.err != nil {
			slog.Warn("provider error", "err", r.err)
			continue
		}
		all = append(all, r.leads...)
	}

	unique := dedup(all)
	slog.Info("campaign dedup complete", "campaignId", p.CampaignID, "raw", len(all), "unique", len(unique))

	if len(unique) > 0 {
		if err := w.bulkInsert(ctx, p.CampaignID, p.UserID, unique); err != nil {
			_ = w.updateStatus(ctx, p.CampaignID, string(StatusFailed), 0)
			return fmt.Errorf("bulk insert: %w", err)
		}
	}

	return w.updateStatus(ctx, p.CampaignID, string(StatusDone), len(unique))
}

func dedup(leads []providers.NormalizedLead) []providers.NormalizedLead {
	seen := make(map[string]bool, len(leads))
	out := make([]providers.NormalizedLead, 0, len(leads))
	for _, l := range leads {
		key := dedupKey(l)
		if !seen[key] {
			seen[key] = true
			out = append(out, l)
		}
	}
	return out
}

// Domains that host many companies — don't use as a dedup key or every
// lead from that source collapses to a single entry.
var aggregatorDomains = map[string]bool{
	"remoteok.com": true, "linkedin.com": true, "indeed.com": true,
	"greenhouse.io": true, "lever.co": true, "workable.com": true,
	"jobs.ashbyhq.com": true, "boards.greenhouse.io": true,
	"apply.workable.com": true, "serpapi.com": true,
	"ycombinator.com": true, "wellfound.com": true,
}

func dedupKey(l providers.NormalizedLead) string {
	if l.Email != "" {
		return "email:" + strings.ToLower(l.Email)
	}
	if l.Website != "" {
		if u, err := url.Parse(l.Website); err == nil {
			host := strings.TrimPrefix(u.Hostname(), "www.")
			if host != "" && !aggregatorDomains[host] {
				return "domain:" + host
			}
		}
	}
	if l.BusinessName != "" {
		return "name:" + strings.ToLower(strings.TrimSpace(l.BusinessName))
	}
	return "rand:" + l.Source + l.ContactName
}

func newUUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func (w *Worker) bulkInsert(ctx context.Context, campaignID, userID string, leads []providers.NormalizedLead) error {
	rows := make([][]any, 0, len(leads))
	for _, l := range leads {
		signalsJSON, _ := json.Marshal(l.IntentSignals)
		rows = append(rows, []any{
			newUUID(), campaignID, userID,
			l.Source, l.BusinessName, l.Website, l.Email, l.Phone,
			l.ContactName, l.ContactTitle, l.CompanySize, l.Location,
			l.LinkedinURL, l.TechStack, signalsJSON, l.IntentScore,
			l.Description, l.Industry,
		})
	}

	_, err := w.db.CopyFrom(ctx,
		pgx.Identifier{"discovered_leads"},
		[]string{
			"id", "campaignId", "userId",
			"source", "businessName", "website", "email", "phone",
			"contactName", "contactTitle", "companySize", "location",
			"linkedinUrl", "techStack", "intentSignals", "intentScore",
			"description", "industry",
		},
		pgx.CopyFromRows(rows),
	)
	return err
}

func (w *Worker) updateStatus(ctx context.Context, campaignID, status string, found int) error {
	_, err := w.db.Exec(ctx,
		`UPDATE lead_campaigns SET status=$2::"CampaignStatus", "totalFound"=$3, "updatedAt"=now() WHERE id=$1`,
		campaignID, status, found,
	)
	return err
}
