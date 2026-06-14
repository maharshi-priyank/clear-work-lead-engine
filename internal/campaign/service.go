package campaign

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"
)

const TaskCampaignRun = "campaign:run"

type Service struct {
	db    *pgxpool.Pool
	queue *asynq.Client
}

func NewService(db *pgxpool.Pool, queue *asynq.Client) *Service {
	return &Service{db: db, queue: queue}
}

func (s *Service) Create(ctx context.Context, userID string, dto CreateDTO) (*Campaign, error) {
	filters := Filters{TargetCount: 50}
	if dto.Filters != nil {
		filters = *dto.Filters
	}
	if filters.TargetCount <= 0 {
		filters.TargetCount = 50
	}

	filtersJSON, _ := json.Marshal(filters)
	row := s.db.QueryRow(ctx,
		`INSERT INTO lead_campaigns (id, "userId", name, filters, providers, status, "totalFound", "totalImported", "createdAt", "updatedAt")
		 VALUES (gen_random_uuid()::text,$1,$2,$3,$4,'RUNNING',0,0,now(),now())
		 RETURNING id, "userId", name, filters::text, providers, status, "totalFound", "totalImported", "createdAt", "updatedAt"`,
		userID, dto.Name, filtersJSON, dto.Providers,
	)
	c, err := scanCampaign(row)
	if err != nil {
		return nil, fmt.Errorf("create campaign: %w", err)
	}

	payload, _ := json.Marshal(map[string]any{
		"campaignId": c.ID,
		"userId":     userID,
		"providers":  dto.Providers,
		"filters":    filters,
	})
	task := asynq.NewTask(TaskCampaignRun, payload)
	_, err = s.queue.EnqueueContext(ctx, task,
		asynq.MaxRetry(2),
		asynq.Timeout(15*time.Minute),
		asynq.Queue("campaigns"),
	)
	if err != nil {
		fmt.Printf("warn: failed to enqueue campaign %s: %v\n", c.ID, err)
	}
	return c, nil
}

func (s *Service) FindAll(ctx context.Context, userID string) ([]Campaign, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, "userId", name, filters::text, providers, status, "totalFound", "totalImported", "createdAt", "updatedAt"
		 FROM lead_campaigns WHERE "userId"=$1 ORDER BY "createdAt" DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Campaign{}
	for rows.Next() {
		c, err := scanCampaign(rows)
		if err != nil {
			continue
		}
		out = append(out, *c)
	}
	return out, nil
}

func (s *Service) FindOne(ctx context.Context, userID, id string) (*Campaign, error) {
	row := s.db.QueryRow(ctx,
		`SELECT id, "userId", name, filters::text, providers, status, "totalFound", "totalImported", "createdAt", "updatedAt"
		 FROM lead_campaigns WHERE id=$1 AND "userId"=$2`,
		id, userID,
	)
	c, err := scanCampaign(row)
	if err != nil {
		return nil, fmt.Errorf("campaign not found")
	}
	return c, nil
}

func (s *Service) Refetch(ctx context.Context, userID, id string) (*Campaign, error) {
	c, err := s.FindOne(ctx, userID, id)
	if err != nil {
		return nil, fmt.Errorf("campaign not found")
	}
	if c.Status == StatusRunning || c.Status == StatusPending {
		return nil, fmt.Errorf("campaign is already running")
	}

	// Reset status to RUNNING
	_, err = s.db.Exec(ctx,
		`UPDATE lead_campaigns SET status='RUNNING', "updatedAt"=now() WHERE id=$1 AND "userId"=$2`,
		id, userID,
	)
	if err != nil {
		return nil, err
	}
	c.Status = StatusRunning

	// Re-enqueue with same providers and filters
	payload, _ := json.Marshal(map[string]any{
		"campaignId": c.ID,
		"userId":     userID,
		"providers":  c.Providers,
		"filters":    c.Filters,
	})
	task := asynq.NewTask(TaskCampaignRun, payload)
	_, err = s.queue.EnqueueContext(ctx, task,
		asynq.MaxRetry(2),
		asynq.Timeout(15*time.Minute),
		asynq.Queue("campaigns"),
	)
	if err != nil {
		fmt.Printf("warn: failed to enqueue refetch for campaign %s: %v\n", id, err)
	}
	return c, nil
}

func (s *Service) Delete(ctx context.Context, userID, id string) error {
	tag, err := s.db.Exec(ctx,
		`DELETE FROM lead_campaigns WHERE id=$1 AND "userId"=$2`,
		id, userID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("campaign not found")
	}
	return nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanCampaign(s scanner) (*Campaign, error) {
	var c Campaign
	var filtersJSON string
	var statusStr string
	err := s.Scan(
		&c.ID, &c.UserID, &c.Name, &filtersJSON, &c.Providers,
		&statusStr, &c.TotalFound, &c.TotalImported, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	c.Status = Status(statusStr)
	json.Unmarshal([]byte(filtersJSON), &c.Filters)
	return &c, nil
}
