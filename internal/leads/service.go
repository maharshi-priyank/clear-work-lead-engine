package leads

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct{ db *pgxpool.Pool }

func NewService(db *pgxpool.Pool) *Service { return &Service{db: db} }

func (s *Service) FindAll(ctx context.Context, userID string, q QueryParams) (*PagedResult, error) {
	if q.Page <= 0 {
		q.Page = 1
	}
	if q.Limit <= 0 {
		q.Limit = 50
	}
	offset := (q.Page - 1) * q.Limit

	where := []string{`"userId"=$1`}
	args := []any{userID}
	n := 2

	if q.CampaignID != "" {
		where = append(where, fmt.Sprintf(`"campaignId"=$%d`, n))
		args = append(args, q.CampaignID)
		n++
	}
	if q.Source != "" {
		where = append(where, fmt.Sprintf("source=$%d", n))
		args = append(args, q.Source)
		n++
	}
	if q.MinScore > 0 {
		where = append(where, fmt.Sprintf(`"intentScore">=$%d`, n))
		args = append(args, q.MinScore)
		n++
	}
	if q.Imported == "true" {
		where = append(where, `"importedAsLeadId" IS NOT NULL`)
	} else if q.Imported == "false" {
		where = append(where, `"importedAsLeadId" IS NULL`)
	}
	if q.Search != "" {
		where = append(where, fmt.Sprintf(
			`("businessName" ILIKE $%d OR "contactName" ILIKE $%d OR email ILIKE $%d OR location ILIKE $%d)`,
			n, n, n, n,
		))
		args = append(args, "%"+q.Search+"%")
		n++
	}

	clause := strings.Join(where, " AND ")
	countSQL := `SELECT COUNT(*) FROM discovered_leads WHERE ` + clause
	listSQL := fmt.Sprintf(
		`SELECT id, "campaignId", "userId", source,
		        COALESCE("businessName",''), COALESCE(website,''), COALESCE(email,''),
		        COALESCE(phone,''), COALESCE("contactName",''), COALESCE("contactTitle",''),
		        COALESCE("companySize",''), COALESCE(location,''), COALESCE("linkedinUrl",''),
		        COALESCE("techStack", '{}'), "intentSignals"::text, "intentScore",
		        COALESCE(description,''), COALESCE(industry,''),
		        COALESCE("importedAsLeadId",''), "createdAt"
		 FROM discovered_leads WHERE %s
		 ORDER BY "intentScore" DESC
		 LIMIT $%d OFFSET $%d`,
		clause, n, n+1,
	)
	listArgs := append(args, q.Limit, offset)

	var total int
	s.db.QueryRow(ctx, countSQL, args...).Scan(&total)

	rows, err := s.db.Query(ctx, listSQL, listArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []DiscoveredLead
	for rows.Next() {
		var l DiscoveredLead
		var signalsJSON string
		err := rows.Scan(
			&l.ID, &l.CampaignID, &l.UserID, &l.Source,
			&l.BusinessName, &l.Website, &l.Email, &l.Phone,
			&l.ContactName, &l.ContactTitle, &l.CompanySize,
			&l.Location, &l.LinkedinURL, &l.TechStack,
			&signalsJSON, &l.IntentScore,
			&l.Description, &l.Industry,
			&l.ImportedAsLeadID, &l.CreatedAt,
		)
		if err != nil {
			continue
		}
		json.Unmarshal([]byte(signalsJSON), &l.IntentSignals)
		items = append(items, l)
	}
	if items == nil {
		items = []DiscoveredLead{}
	}
	return &PagedResult{Items: items, Total: total, Page: q.Page, Limit: q.Limit}, nil
}

func (s *Service) ImportToCRM(ctx context.Context, userID, id string) (map[string]any, error) {
	var dl DiscoveredLead
	var signalsJSON string
	err := s.db.QueryRow(ctx,
		`SELECT id, "campaignId", "userId", source,
		        COALESCE("businessName",''), COALESCE(website,''), COALESCE(email,''),
		        COALESCE(phone,''), COALESCE("contactName",''), COALESCE("contactTitle",''),
		        COALESCE("companySize",''), COALESCE(location,''), COALESCE("linkedinUrl",''),
		        COALESCE("techStack",'{}'), "intentSignals"::text, "intentScore",
		        COALESCE(description,''), COALESCE(industry,''),
		        COALESCE("importedAsLeadId",''), "createdAt"
		 FROM discovered_leads WHERE id=$1 AND "userId"=$2`,
		id, userID,
	).Scan(
		&dl.ID, &dl.CampaignID, &dl.UserID, &dl.Source,
		&dl.BusinessName, &dl.Website, &dl.Email, &dl.Phone,
		&dl.ContactName, &dl.ContactTitle, &dl.CompanySize,
		&dl.Location, &dl.LinkedinURL, &dl.TechStack,
		&signalsJSON, &dl.IntentScore,
		&dl.Description, &dl.Industry,
		&dl.ImportedAsLeadID, &dl.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("discovered lead not found")
	}
	if dl.ImportedAsLeadID != "" {
		return map[string]any{"alreadyImported": true, "leadId": dl.ImportedAsLeadID}, nil
	}

	name := dl.ContactName
	if name == "" {
		name = dl.BusinessName
	}
	if name == "" {
		name = "Unknown"
	}

	var notes []string
	if dl.Description != "" {
		notes = append(notes, "About: "+dl.Description)
	}
	if dl.Industry != "" {
		notes = append(notes, "Industry: "+dl.Industry)
	}
	if dl.ContactTitle != "" {
		notes = append(notes, "Contact title: "+dl.ContactTitle)
	}
	if dl.Location != "" {
		notes = append(notes, "Location: "+dl.Location)
	}
	if len(dl.TechStack) > 0 {
		notes = append(notes, "Tech stack: "+strings.Join(dl.TechStack, ", "))
	}
	if dl.Website != "" {
		notes = append(notes, "Website: "+dl.Website)
	}
	if dl.LinkedinURL != "" {
		notes = append(notes, "LinkedIn: "+dl.LinkedinURL)
	}
	notesStr := strings.Join(notes, "\n")

	var leadID string
	err = s.db.QueryRow(ctx,
		`INSERT INTO leads (id, "userId", name, email, phone, company, source, notes, "lastActivityAt", "createdAt", "updatedAt")
		 VALUES (gen_random_uuid()::text,$1,$2,$3,$4,$5,$6,$7,now(),now(),now()) RETURNING id`,
		userID, name, nullStr(dl.Email), nullStr(dl.Phone),
		nullStr(dl.BusinessName), dl.Source, nullStr(notesStr),
	).Scan(&leadID)
	if err != nil {
		return nil, fmt.Errorf("create lead: %w", err)
	}

	s.db.Exec(ctx,
		`UPDATE discovered_leads SET "importedAsLeadId"=$1 WHERE id=$2`,
		leadID, id,
	)
	s.db.Exec(ctx,
		`UPDATE lead_campaigns SET "totalImported"="totalImported"+1, "updatedAt"=now() WHERE id=$1`,
		dl.CampaignID,
	)

	return map[string]any{"leadId": leadID, "alreadyImported": false}, nil
}

func (s *Service) BulkImport(ctx context.Context, userID string, ids []string) (map[string]any, error) {
	succeeded := 0
	for _, id := range ids {
		if _, err := s.ImportToCRM(ctx, userID, id); err == nil {
			succeeded++
		}
	}
	return map[string]any{"imported": succeeded, "total": len(ids)}, nil
}

func (s *Service) ExportCSV(ctx context.Context, userID string, q QueryParams) (string, error) {
	q.Limit = 5000
	q.Page = 1
	result, err := s.FindAll(ctx, userID, q)
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	sb.WriteString("Business Name,Industry,Contact Name,Title,Email,Phone,Website,Location,Company Size,Description,Tech Stack,Source,Intent Score,LinkedIn\n")
	for _, l := range result.Items {
		row := []string{
			l.BusinessName, l.Industry, l.ContactName, l.ContactTitle, l.Email, l.Phone,
			l.Website, l.Location, l.CompanySize, l.Description,
			strings.Join(l.TechStack, "|"),
			l.Source, fmt.Sprintf("%d", l.IntentScore), l.LinkedinURL,
		}
		for i, v := range row {
			row[i] = `"` + strings.ReplaceAll(v, `"`, `""`) + `"`
		}
		sb.WriteString(strings.Join(row, ",") + "\n")
	}
	return sb.String(), nil
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}
