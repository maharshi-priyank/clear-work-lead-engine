package vault

import (
	"context"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	db  *pgxpool.Pool
	key string
}

func NewService(db *pgxpool.Pool) *Service {
	return &Service{db: db, key: os.Getenv("VAULT_ENCRYPTION_KEY")}
}

func (s *Service) GetDecryptedKey(ctx context.Context, userID, provider string) (string, error) {
	var enc string
	err := s.db.QueryRow(ctx,
		`SELECT "encryptedKey" FROM lead_provider_keys WHERE "userId"=$1 AND provider=$2`,
		userID, provider,
	).Scan(&enc)
	if err != nil {
		return "", nil
	}
	return Decrypt(enc, s.key)
}

type ProviderKeyRow struct {
	Provider     string  `json:"provider"`
	Status       string  `json:"status"`
	LastTestedAt *string `json:"lastTestedAt"`
}

func (s *Service) List(ctx context.Context, userID string) ([]ProviderKeyRow, error) {
	rows, err := s.db.Query(ctx,
		`SELECT provider, status, "lastTestedAt"::text FROM lead_provider_keys WHERE "userId"=$1 ORDER BY "createdAt"`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []ProviderKeyRow
	for rows.Next() {
		var r ProviderKeyRow
		if err := rows.Scan(&r.Provider, &r.Status, &r.LastTestedAt); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, nil
}

func (s *Service) Save(ctx context.Context, userID, provider, rawKey string) error {
	return s.SaveWithStatus(ctx, userID, provider, rawKey, "active")
}

func (s *Service) SaveWithStatus(ctx context.Context, userID, provider, rawKey, status string) error {
	enc, err := Encrypt(rawKey, s.key)
	if err != nil {
		return fmt.Errorf("encrypt: %w", err)
	}
	_, err = s.db.Exec(ctx,
		`INSERT INTO lead_provider_keys (id, "userId", provider, "encryptedKey", status, "lastTestedAt", "createdAt", "updatedAt")
		 VALUES (gen_random_uuid()::text,$1,$2,$3,$4,now(),now(),now())
		 ON CONFLICT ("userId", provider) DO UPDATE
		 SET "encryptedKey"=$3, status=$4, "lastTestedAt"=now(), "updatedAt"=now()`,
		userID, provider, enc, status,
	)
	return err
}

func (s *Service) UpdateStatus(ctx context.Context, userID, provider, status string) error {
	_, err := s.db.Exec(ctx,
		`UPDATE lead_provider_keys SET status=$3, "lastTestedAt"=now(), "updatedAt"=now()
		 WHERE "userId"=$1 AND provider=$2`,
		userID, provider, status,
	)
	return err
}

func (s *Service) Remove(ctx context.Context, userID, provider string) error {
	_, err := s.db.Exec(ctx,
		`DELETE FROM lead_provider_keys WHERE "userId"=$1 AND provider=$2`,
		userID, provider,
	)
	return err
}
