package db

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type UserStore struct {
	pool *pgxpool.Pool
}

func NewUserStore(pool *pgxpool.Pool) *UserStore {
	return &UserStore{pool: pool}
}

func (s *UserStore) ListWithTelegramID(ctx context.Context) ([]AdminUser, error) {
	const q = `SELECT id, username, password_hash, role, is_active, telegram_chat_id, created_at, updated_at
		FROM admin_users WHERE telegram_chat_id IS NOT NULL`
	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []AdminUser
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, *u)
	}
	return users, rows.Err()
}

func (s *UserStore) GetByID(ctx context.Context, id uuid.UUID) (*AdminUser, error) {
	const q = `SELECT id, username, password_hash, role, is_active, telegram_chat_id, created_at, updated_at
		FROM admin_users WHERE id = $1`
	row := s.pool.QueryRow(ctx, q, id)
	u, err := scanUser(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return u, err
}

func (s *UserStore) GetByUsername(ctx context.Context, username string) (*AdminUser, error) {
	const q = `SELECT id, username, password_hash, role, is_active, telegram_chat_id, created_at, updated_at
		FROM admin_users WHERE username = $1`
	row := s.pool.QueryRow(ctx, q, username)
	u, err := scanUser(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return u, err
}

func (s *UserStore) List(ctx context.Context) ([]AdminUser, error) {
	const q = `SELECT id, username, password_hash, role, is_active, created_at, updated_at
		FROM admin_users ORDER BY created_at`
	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []AdminUser
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, *u)
	}
	return users, rows.Err()
}

// Count returns the total number of admin users (used to detect first-run).
func (s *UserStore) Count(ctx context.Context) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM admin_users`).Scan(&n)
	return n, err
}

type CreateUserParams struct {
	Username     string
	PasswordHash string
	Role         string
}

func (s *UserStore) Create(ctx context.Context, p CreateUserParams) (*AdminUser, error) {
	const q = `INSERT INTO admin_users (username, password_hash, role)
		VALUES ($1, $2, $3)
		RETURNING id, username, password_hash, role, is_active, created_at, updated_at`
	row := s.pool.QueryRow(ctx, q, p.Username, p.PasswordHash, p.Role)
	return scanUser(row)
}

func (s *UserStore) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM admin_users WHERE id = $1`, id)
	return err
}

// ─── API Tokens ──────────────────────────────────────────────────────────────

type TokenStore struct {
	pool *pgxpool.Pool
}

func NewTokenStore(pool *pgxpool.Pool) *TokenStore {
	return &TokenStore{pool: pool}
}

func (s *TokenStore) GetByHash(ctx context.Context, hash string) (*APIToken, error) {
	const q = `SELECT id, name, token_hash, role, created_by, expires_at, created_at
		FROM api_tokens
		WHERE token_hash = $1 AND (expires_at IS NULL OR expires_at > NOW())`
	var t APIToken
	err := s.pool.QueryRow(ctx, q, hash).Scan(
		&t.ID, &t.Name, &t.TokenHash, &t.Role,
		&t.CreatedBy, &t.ExpiresAt, &t.CreatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan token: %w", err)
	}
	return &t, nil
}

type CreateTokenParams struct {
	Name      string
	TokenHash string
	Role      string
	CreatedBy *uuid.UUID
	ExpiresAt *time.Time
}

func (s *TokenStore) Create(ctx context.Context, p CreateTokenParams) (*APIToken, error) {
	const q = `INSERT INTO api_tokens (name, token_hash, role, created_by, expires_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, name, token_hash, role, created_by, expires_at, created_at`
	var t APIToken
	err := s.pool.QueryRow(ctx, q,
		p.Name, p.TokenHash, p.Role, p.CreatedBy, p.ExpiresAt,
	).Scan(&t.ID, &t.Name, &t.TokenHash, &t.Role, &t.CreatedBy, &t.ExpiresAt, &t.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create token: %w", err)
	}
	return &t, nil
}

func (s *TokenStore) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM api_tokens WHERE id = $1`, id)
	return err
}

func (s *TokenStore) ListByCreator(ctx context.Context, createdBy uuid.UUID) ([]APIToken, error) {
	const q = `SELECT id, name, token_hash, role, created_by, expires_at, created_at
		FROM api_tokens WHERE created_by = $1 ORDER BY created_at DESC`
	rows, err := s.pool.Query(ctx, q, createdBy)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tokens []APIToken
	for rows.Next() {
		var t APIToken
		err := rows.Scan(&t.ID, &t.Name, &t.TokenHash, &t.Role, &t.CreatedBy, &t.ExpiresAt, &t.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("scan token: %w", err)
		}
		tokens = append(tokens, t)
	}
	return tokens, rows.Err()
}

func scanUser(row interface{ Scan(dest ...any) error }) (*AdminUser, error) {
	var u AdminUser
	err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role,
		&u.IsActive, &u.TelegramChatID, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("scan user: %w", err)
	}
	return &u, nil
}
