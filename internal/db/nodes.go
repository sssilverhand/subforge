package db

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type NodeStore struct {
	pool *pgxpool.Pool
}

func NewNodeStore(pool *pgxpool.Pool) *NodeStore {
	return &NodeStore{pool: pool}
}

func (s *NodeStore) GetByID(ctx context.Context, id uuid.UUID) (*Node, error) {
	const q = `
		SELECT id, name, xray_api_addr, xray_api_tls, hy2_api_url, hy2_api_secret,
		       public_host, agent_url, agent_secret, agent_version, agent_last_seen,
		       is_active, created_at, updated_at
		FROM nodes WHERE id = $1`
	row := s.pool.QueryRow(ctx, q, id)
	n, err := scanNode(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return n, err
}

func (s *NodeStore) ListActive(ctx context.Context) ([]Node, error) {
	const q = `
		SELECT id, name, xray_api_addr, xray_api_tls, hy2_api_url, hy2_api_secret,
		       public_host, agent_url, agent_secret, agent_version, agent_last_seen,
		       is_active, created_at, updated_at
		FROM nodes WHERE is_active = TRUE ORDER BY name`
	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []Node
	for rows.Next() {
		n, err := scanNode(rows)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, *n)
	}
	return nodes, rows.Err()
}

type CreateNodeParams struct {
	Name         string
	XrayAPIAddr  *string
	XrayAPITLS   bool
	Hy2APIURL    *string
	Hy2APISecret *string
	PublicHost   string
}

func (s *NodeStore) Create(ctx context.Context, p CreateNodeParams) (*Node, error) {
	const q = `
		INSERT INTO nodes (name, xray_api_addr, xray_api_tls, hy2_api_url, hy2_api_secret, public_host)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, name, xray_api_addr, xray_api_tls, hy2_api_url, hy2_api_secret,
		          public_host, is_active, created_at, updated_at`
	row := s.pool.QueryRow(ctx, q,
		p.Name, p.XrayAPIAddr, p.XrayAPITLS,
		p.Hy2APIURL, p.Hy2APISecret, p.PublicHost,
	)
	return scanNode(row)
}

// ListInboundsForNode returns all inbounds for a given node.
func (s *NodeStore) ListInbounds(ctx context.Context, nodeID uuid.UUID) ([]Inbound, error) {
	const q = `
		SELECT id, node_id, tag, protocol, port, settings, is_active, created_at
		FROM inbounds WHERE node_id = $1 ORDER BY protocol, port`
	rows, err := s.pool.Query(ctx, q, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var inbounds []Inbound
	for rows.Next() {
		var ib Inbound
		err := rows.Scan(&ib.ID, &ib.NodeID, &ib.Tag, &ib.Protocol,
			&ib.Port, &ib.Settings, &ib.IsActive, &ib.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("scan inbound: %w", err)
		}
		inbounds = append(inbounds, ib)
	}
	return inbounds, rows.Err()
}

type CreateInboundParams struct {
	NodeID   uuid.UUID
	Tag      string
	Protocol string
	Port     int
	Settings []byte
}

func (s *NodeStore) CreateInbound(ctx context.Context, p CreateInboundParams) (*Inbound, error) {
	const q = `
		INSERT INTO inbounds (node_id, tag, protocol, port, settings)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, node_id, tag, protocol, port, settings, is_active, created_at`
	var ib Inbound
	err := s.pool.QueryRow(ctx, q,
		p.NodeID, p.Tag, p.Protocol, p.Port, p.Settings,
	).Scan(&ib.ID, &ib.NodeID, &ib.Tag, &ib.Protocol,
		&ib.Port, &ib.Settings, &ib.IsActive, &ib.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create inbound: %w", err)
	}
	return &ib, nil
}

func scanNode(row interface{ Scan(dest ...any) error }) (*Node, error) {
	var n Node
	err := row.Scan(
		&n.ID, &n.Name, &n.XrayAPIAddr, &n.XrayAPITLS,
		&n.Hy2APIURL, &n.Hy2APISecret,
		&n.PublicHost, &n.AgentURL, &n.AgentSecret, &n.AgentVersion, &n.AgentLastSeen,
		&n.IsActive, &n.CreatedAt, &n.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scan node: %w", err)
	}
	return &n, nil
}
