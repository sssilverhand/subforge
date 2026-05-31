package subscription

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/sssilverhand/subforge/internal/db"
	hysteria2 "github.com/sssilverhand/subforge/internal/proxy/hysteria2"
	"github.com/sssilverhand/subforge/internal/proxy/xray"
	"github.com/sssilverhand/subforge/internal/subgen"
)

// proxyClients holds live connections to a node's binaries.
type proxyClients struct {
	xray *xray.Client
	hy2  *hysteria2.Client
}

// Service handles subscription lifecycle.
type Service struct {
	subs  *db.SubscriptionStore
	nodes *db.NodeStore
	// nodeClients is a cache of gRPC/HTTP clients keyed by node ID.
	// In production this should be an LRU or connection pool.
	nodeClients map[uuid.UUID]*proxyClients
}

func NewService(subs *db.SubscriptionStore, nodes *db.NodeStore) *Service {
	return &Service{
		subs:        subs,
		nodes:       nodes,
		nodeClients: make(map[uuid.UUID]*proxyClients),
	}
}

// CreateParams are the inputs for creating a new subscription.
type CreateParams struct {
	Name              *string
	PlanID            *uuid.UUID
	InboundIDs        []uuid.UUID // which inbounds to enable
	TrafficLimitBytes *int64      // nil = unlimited
	ExpiresAt         *time.Time
	CreatedBy         *uuid.UUID
}

// Create creates a subscription, persists it to DB, and registers the user
// credentials in every selected inbound's binary.
func (s *Service) Create(ctx context.Context, p CreateParams) (*db.Subscription, error) {
	if len(p.InboundIDs) == 0 {
		return nil, fmt.Errorf("at least one inbound must be selected")
	}

	token, err := generateToken()
	if err != nil {
		return nil, fmt.Errorf("generate token: %w", err)
	}
	hy2Pass, err := generateToken()
	if err != nil {
		return nil, fmt.Errorf("generate hy2 password: %w", err)
	}

	sub, err := s.subs.Create(ctx, db.CreateSubscriptionParams{
		Token:             token,
		Name:              p.Name,
		PlanID:            p.PlanID,
		UUID:              uuid.New(),
		Hy2Password:       hy2Pass,
		TrafficLimitBytes: p.TrafficLimitBytes,
		ExpiresAt:         p.ExpiresAt,
		CreatedBy:         p.CreatedBy,
	})
	if err != nil {
		return nil, fmt.Errorf("db create subscription: %w", err)
	}

	// Link inbounds and register user in each binary.
	// On any error we attempt to roll back the binary state.
	var registered []db.InboundWithNode
	for _, inboundID := range p.InboundIDs {
		if err := s.subs.AddInbound(ctx, sub.ID, inboundID); err != nil {
			_ = s.rollbackRegistrations(ctx, sub, registered)
			_ = s.subs.Delete(ctx, sub.ID)
			return nil, fmt.Errorf("link inbound %s: %w", inboundID, err)
		}
	}

	// Fetch all linked inbounds with node info for binary registration.
	inbounds, err := s.subs.GetInboundsForSubscription(ctx, sub.ID)
	if err != nil {
		_ = s.rollbackRegistrations(ctx, sub, registered)
		_ = s.subs.Delete(ctx, sub.ID)
		return nil, fmt.Errorf("fetch inbounds: %w", err)
	}

	for _, ib := range inbounds {
		if err := s.registerInBinary(ctx, sub, ib); err != nil {
			_ = s.rollbackRegistrations(ctx, sub, registered)
			_ = s.subs.Delete(ctx, sub.ID)
			return nil, fmt.Errorf("register in binary (node %s, inbound %s): %w",
				ib.NodeName, ib.Tag, err)
		}
		registered = append(registered, ib)
	}

	return sub, nil
}

// Disable removes a subscription's credentials from all binaries and marks it disabled.
// The subscription record is kept in DB for history/billing.
func (s *Service) Disable(ctx context.Context, subID uuid.UUID) error {
	sub, err := s.subs.GetByID(ctx, subID)
	if err != nil {
		return err
	}
	if sub == nil {
		return fmt.Errorf("subscription not found")
	}

	inbounds, err := s.subs.GetInboundsForSubscription(ctx, subID)
	if err != nil {
		return err
	}

	for _, ib := range inbounds {
		if err := s.unregisterFromBinary(ctx, sub, ib); err != nil {
			// Log but continue — partial removal is better than none.
			// In production: emit a metric/alert here.
			fmt.Printf("warn: unregister from binary %s/%s: %v\n", ib.NodeName, ib.Tag, err)
		}
	}

	return s.subs.SetEnabled(ctx, subID, false)
}

// Enable re-registers credentials in all binaries and marks enabled.
func (s *Service) Enable(ctx context.Context, subID uuid.UUID) error {
	sub, err := s.subs.GetByID(ctx, subID)
	if err != nil {
		return err
	}
	if sub == nil {
		return fmt.Errorf("subscription not found")
	}

	inbounds, err := s.subs.GetInboundsForSubscription(ctx, subID)
	if err != nil {
		return err
	}

	for _, ib := range inbounds {
		if err := s.registerInBinary(ctx, sub, ib); err != nil {
			return fmt.Errorf("re-register in binary %s/%s: %w", ib.NodeName, ib.Tag, err)
		}
	}

	return s.subs.SetEnabled(ctx, subID, true)
}

// GetByID exposes the underlying store for callers that need a full Subscription row.
func (s *Service) GetByID(ctx context.Context, id uuid.UUID) (*db.Subscription, error) {
	return s.subs.GetByID(ctx, id)
}

// RenewAfterPayment resets traffic, sets new limit and expiry, and re-enables.
// Called after a successful payment to activate or extend a subscription.
func (s *Service) RenewAfterPayment(ctx context.Context, id uuid.UUID, newLimit *int64, newExpiry *time.Time) error {
	const q = `UPDATE subscriptions
		SET traffic_used_bytes    = 0,
		    is_traffic_exceeded   = FALSE,
		    is_expired            = FALSE,
		    is_enabled            = TRUE,
		    traffic_limit_bytes   = COALESCE($2, traffic_limit_bytes),
		    expires_at            = COALESCE($3, expires_at),
		    updated_at            = NOW()
		WHERE id = $1`
	err := s.subs.Exec(ctx, q, id, newLimit, newExpiry)
	if err != nil {
		return fmt.Errorf("renew subscription: %w", err)
	}
	return s.Enable(ctx, id)
}

// ResetTraffic zeroes traffic counters and re-enables a traffic-exceeded subscription.
func (s *Service) ResetTraffic(ctx context.Context, subID uuid.UUID) error {
	if err := s.subs.ResetTraffic(ctx, subID); err != nil {
		return err
	}
	// Re-register in binaries (subscription may have been disabled due to traffic limit).
	return s.Enable(ctx, subID)
}

// Delete fully removes a subscription: unregisters from binaries, deletes from DB.
func (s *Service) Delete(ctx context.Context, subID uuid.UUID) error {
	if err := s.Disable(ctx, subID); err != nil {
		return err
	}
	return s.subs.Delete(ctx, subID)
}

// BuildSubData assembles SubData for subscription generation.
func (s *Service) BuildSubData(ctx context.Context, sub *db.Subscription) (*subgen.SubData, error) {
	inbounds, err := s.subs.GetInboundsForSubscription(ctx, sub.ID)
	if err != nil {
		return nil, err
	}

	name := ""
	if sub.Name != nil {
		name = *sub.Name
	}

	data := &subgen.SubData{
		SubName:     name,
		UserUUID:    sub.UUID,
		Hy2Password: sub.Hy2Password,
	}

	for _, ib := range inbounds {
		ep, err := inboundToEndpoint(ib)
		if err != nil {
			return nil, fmt.Errorf("inbound %s: %w", ib.Tag, err)
		}
		data.Endpoints = append(data.Endpoints, ep)
	}

	return data, nil
}

// ─── Internal ────────────────────────────────────────────────────────────────

func (s *Service) registerInBinary(ctx context.Context, sub *db.Subscription, ib db.InboundWithNode) error {
	clients, err := s.getClients(ib)
	if err != nil {
		return err
	}

	email := xray.UserEmail(sub.Token)

	switch ib.Protocol {
	case "vless-xhttp", "vless-reality", "vless-ws":
		if clients.xray == nil {
			return fmt.Errorf("no xray client for node %s", ib.NodeName)
		}
		return clients.xray.AddUser(ctx, ib.Tag, sub.UUID, email)
	case "hysteria2":
		// hysteria2 uses password-based auth; user is added implicitly
		// when they connect using the correct password.
		// No explicit registration needed — password IS the credential.
		// However we still verify the client is reachable.
		if clients.hy2 == nil {
			return fmt.Errorf("no hysteria2 client for node %s", ib.NodeName)
		}
		return nil
	default:
		return fmt.Errorf("unknown protocol: %s", ib.Protocol)
	}
}

func (s *Service) unregisterFromBinary(ctx context.Context, sub *db.Subscription, ib db.InboundWithNode) error {
	clients, err := s.getClients(ib)
	if err != nil {
		return err
	}

	email := xray.UserEmail(sub.Token)

	switch ib.Protocol {
	case "vless-xhttp", "vless-reality", "vless-ws":
		if clients.xray == nil {
			return nil
		}
		return clients.xray.RemoveUser(ctx, ib.Tag, email)
	case "hysteria2":
		// Kick any active session. Future connections are blocked because
		// our hy2auth endpoint checks is_enabled before approving them.
		if clients.hy2 == nil {
			return nil
		}
		return clients.hy2.KickUsers(ctx, []string{hysteria2.UserID(sub.Token)})
	default:
		return fmt.Errorf("unknown protocol: %s", ib.Protocol)
	}
}

func (s *Service) rollbackRegistrations(ctx context.Context, sub *db.Subscription, registered []db.InboundWithNode) error {
	for _, ib := range registered {
		_ = s.unregisterFromBinary(ctx, sub, ib)
	}
	return nil
}

// getClients returns (cached) proxy clients for the node of a given inbound.
func (s *Service) getClients(ib db.InboundWithNode) (*proxyClients, error) {
	if c, ok := s.nodeClients[ib.NodeID]; ok {
		return c, nil
	}

	c := &proxyClients{}

	if ib.XrayAPIAddr != nil {
		xc, err := xray.NewClient(*ib.XrayAPIAddr, ib.XrayAPITLS)
		if err != nil {
			return nil, fmt.Errorf("xray client for %s: %w", ib.NodeName, err)
		}
		c.xray = xc
	}

	if ib.Hy2APIURL != nil {
		secret := ""
		if ib.Hy2APISecret != nil {
			secret = *ib.Hy2APISecret
		}
		c.hy2 = hysteria2.NewClient(*ib.Hy2APIURL, secret) //nolint
	}

	s.nodeClients[ib.NodeID] = c
	return c, nil
}

// inboundToEndpoint converts a DB inbound row to a subgen.Endpoint.
func inboundToEndpoint(ib db.InboundWithNode) (subgen.Endpoint, error) {
	var settings subgen.EndpointSettings
	if len(ib.Settings) > 0 {
		if err := json.Unmarshal(ib.Settings, &settings); err != nil {
			return subgen.Endpoint{}, fmt.Errorf("unmarshal settings: %w", err)
		}
	}
	return subgen.Endpoint{
		NodeName:   ib.NodeName,
		PublicHost: ib.PublicHost,
		Protocol:   ib.Protocol,
		Port:       ib.Port,
		Settings:   settings,
	}, nil
}

func generateToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
