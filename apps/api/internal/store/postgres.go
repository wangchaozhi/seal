package store

import (
	"context"
	"database/sql"
	"errors"
)

var ErrNotFound = errors.New("record not found")

// PostgresSealConfigRepository contains PostgreSQL-specific SQL while accepting
// a standard *sql.DB. Production composition can inject pgx/stdlib, lib/pq, or
// a managed-provider driver without coupling the domain package to one SDK.
type PostgresSealConfigRepository struct{ database *sql.DB }

func NewPostgresSealConfigRepository(ctx context.Context, database *sql.DB) (*PostgresSealConfigRepository, error) {
	if database == nil {
		return nil, errors.New("database is required")
	}
	if err := database.PingContext(ctx); err != nil {
		return nil, err
	}
	return &PostgresSealConfigRepository{database: database}, nil
}

func (repository *PostgresSealConfigRepository) Create(ctx context.Context, record SealConfigRecord) (SealConfigRecord, error) {
	err := repository.database.QueryRowContext(ctx, `
		INSERT INTO seal_configs (id, user_id, name, schema_version, config, created_at, updated_at)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5::jsonb, $6, $7)
		RETURNING created_at, updated_at`, record.ID, record.UserID, record.Name, record.SchemaVersion, record.Config, record.CreatedAt, record.UpdatedAt).
		Scan(&record.CreatedAt, &record.UpdatedAt)
	return record, err
}

func (repository *PostgresSealConfigRepository) Get(ctx context.Context, userID, id string) (SealConfigRecord, error) {
	var record SealConfigRecord
	err := repository.database.QueryRowContext(ctx, `SELECT id::text, user_id::text, name, schema_version, config, created_at, updated_at FROM seal_configs WHERE id=$1::uuid AND user_id=$2::uuid`, id, userID).
		Scan(&record.ID, &record.UserID, &record.Name, &record.SchemaVersion, &record.Config, &record.CreatedAt, &record.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return SealConfigRecord{}, ErrNotFound
	}
	return record, err
}

func (repository *PostgresSealConfigRepository) List(ctx context.Context, userID string, limit, offset int) ([]SealConfigRecord, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := repository.database.QueryContext(ctx, `SELECT id::text, user_id::text, name, schema_version, config, created_at, updated_at FROM seal_configs WHERE user_id=$1::uuid ORDER BY updated_at DESC LIMIT $2 OFFSET $3`, userID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]SealConfigRecord, 0)
	for rows.Next() {
		var record SealConfigRecord
		if err := rows.Scan(&record.ID, &record.UserID, &record.Name, &record.SchemaVersion, &record.Config, &record.CreatedAt, &record.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, record)
	}
	return result, rows.Err()
}

func (repository *PostgresSealConfigRepository) Update(ctx context.Context, record SealConfigRecord) (SealConfigRecord, error) {
	err := repository.database.QueryRowContext(ctx, `UPDATE seal_configs SET name=$3, schema_version=$4, config=$5::jsonb, updated_at=NOW() WHERE id=$1::uuid AND user_id=$2::uuid RETURNING created_at, updated_at`, record.ID, record.UserID, record.Name, record.SchemaVersion, record.Config).
		Scan(&record.CreatedAt, &record.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return SealConfigRecord{}, ErrNotFound
	}
	return record, err
}

func (repository *PostgresSealConfigRepository) Delete(ctx context.Context, userID, id string) error {
	result, err := repository.database.ExecContext(ctx, `DELETE FROM seal_configs WHERE id=$1::uuid AND user_id=$2::uuid`, id, userID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

var _ SealConfigRepository = (*PostgresSealConfigRepository)(nil)
