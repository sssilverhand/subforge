package db

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PlanStore struct {
	pool *pgxpool.Pool
}

func NewPlanStore(pool *pgxpool.Pool) *PlanStore {
	return &PlanStore{pool: pool}
}

func (s *PlanStore) GetByID(ctx context.Context, id uuid.UUID) (*Plan, error) {
	const q = `SELECT id, name, description, price_usd, traffic_limit_bytes, duration_days, is_active, created_at
		FROM plans WHERE id = $1`
	row := s.pool.QueryRow(ctx, q, id)
	p, err := scanPlan(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return p, err
}

func (s *PlanStore) ListActive(ctx context.Context) ([]Plan, error) {
	const q = `SELECT id, name, description, price_usd, traffic_limit_bytes, duration_days, is_active, created_at
		FROM plans WHERE is_active = TRUE ORDER BY price_usd NULLS FIRST`
	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var plans []Plan
	for rows.Next() {
		p, err := scanPlan(rows)
		if err != nil {
			return nil, err
		}
		plans = append(plans, *p)
	}
	return plans, rows.Err()
}

type CreatePlanParams struct {
	Name              string
	Description       *string
	PriceUSD          *float64
	TrafficLimitBytes *int64
	DurationDays      *int
}

func (s *PlanStore) Create(ctx context.Context, p CreatePlanParams) (*Plan, error) {
	const q = `INSERT INTO plans (name, description, price_usd, traffic_limit_bytes, duration_days)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, name, description, price_usd, traffic_limit_bytes, duration_days, is_active, created_at`
	row := s.pool.QueryRow(ctx, q, p.Name, p.Description, p.PriceUSD, p.TrafficLimitBytes, p.DurationDays)
	return scanPlan(row)
}

func (s *PlanStore) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `UPDATE plans SET is_active = FALSE WHERE id = $1`, id)
	return err
}

func scanPlan(row interface{ Scan(dest ...any) error }) (*Plan, error) {
	var p Plan
	err := row.Scan(&p.ID, &p.Name, &p.Description, &p.PriceUSD,
		&p.TrafficLimitBytes, &p.DurationDays, &p.IsActive, &p.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("scan plan: %w", err)
	}
	return &p, nil
}
