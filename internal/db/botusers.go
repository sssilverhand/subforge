package db

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type BotUser struct {
	ChatID    int64
	Username  string
	FirstName string
	LastName  string
	JoinedAt  time.Time
}

type BotUserStore struct {
	pool *pgxpool.Pool
}

func NewBotUserStore(pool *pgxpool.Pool) *BotUserStore {
	return &BotUserStore{pool: pool}
}

func (s *BotUserStore) Upsert(ctx context.Context, u BotUser) error {
	const q = `
		INSERT INTO bot_users (chat_id, username, first_name, last_name)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (chat_id) DO UPDATE
		SET username   = EXCLUDED.username,
		    first_name = EXCLUDED.first_name,
		    last_name  = EXCLUDED.last_name`
	_, err := s.pool.Exec(ctx, q, u.ChatID, u.Username, u.FirstName, u.LastName)
	return err
}

func (s *BotUserStore) ListRecent(ctx context.Context, limit int) ([]BotUser, error) {
	const q = `SELECT chat_id, username, first_name, last_name, joined_at
		FROM bot_users ORDER BY joined_at DESC LIMIT $1`
	rows, err := s.pool.Query(ctx, q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []BotUser
	for rows.Next() {
		var u BotUser
		if err := rows.Scan(&u.ChatID, &u.Username, &u.FirstName, &u.LastName, &u.JoinedAt); err != nil {
			return nil, fmt.Errorf("scan bot user: %w", err)
		}
		users = append(users, u)
	}
	return users, rows.Err()
}
