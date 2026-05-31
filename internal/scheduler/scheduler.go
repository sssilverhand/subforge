package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/sssilverhand/subforge/internal/db"
	"github.com/sssilverhand/subforge/internal/proxy/hysteria2"
	"github.com/sssilverhand/subforge/internal/proxy/xray"
)

// Disabler can disable a subscription (remove from binaries + mark DB).
type Disabler interface {
	Disable(ctx context.Context, subID uuid.UUID) error
}

// Notifier sends Telegram notifications (optional — nil = disabled).
type Notifier interface {
	NotifyTrafficWarning(ctx context.Context, sub *db.Subscription)
	NotifyExpiringSoon(ctx context.Context, sub *db.Subscription)
}

type Scheduler struct {
	subs     *db.SubscriptionStore
	nodes    *db.NodeStore
	disabler Disabler
	notify   Notifier // may be nil
	log      *slog.Logger

	trafficInterval time.Duration
	expiryInterval  time.Duration
}

func New(
	subs *db.SubscriptionStore,
	nodes *db.NodeStore,
	disabler Disabler,
	log *slog.Logger,
	trafficInterval, expiryInterval time.Duration,
) *Scheduler {
	return &Scheduler{
		subs:            subs,
		nodes:           nodes,
		disabler:        disabler,
		log:             log,
		trafficInterval: trafficInterval,
		expiryInterval:  expiryInterval,
	}
}

// SetNotifier attaches a bot notifier. Called after bot is created.
func (s *Scheduler) SetNotifier(n Notifier) { s.notify = n }

// Start launches both background jobs. Blocks until ctx is cancelled.
func (s *Scheduler) Start(ctx context.Context) {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); s.runTrafficPoller(ctx) }()
	go func() { defer wg.Done(); s.runExpiryChecker(ctx) }()
	wg.Wait()
}

// ─── Traffic polling ─────────────────────────────────────────────────────────

func (s *Scheduler) runTrafficPoller(ctx context.Context) {
	ticker := time.NewTicker(s.trafficInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.pollAllTraffic(ctx)
		}
	}
}

func (s *Scheduler) pollAllTraffic(ctx context.Context) {
	nodes, err := s.nodes.ListActive(ctx)
	if err != nil {
		s.log.Error("scheduler: list nodes", "err", err)
		return
	}

	for _, node := range nodes {
		s.pollNodeTraffic(ctx, node)
	}

	// After updating traffic, disable any that exceeded their limit.
	exceeded, err := s.subs.MarkTrafficExceeded(ctx)
	if err != nil {
		s.log.Error("scheduler: mark traffic exceeded", "err", err)
		return
	}
	for _, id := range exceeded {
		s.log.Info("scheduler: traffic limit reached, disabling", "sub_id", id)
		if err := s.disabler.Disable(ctx, id); err != nil {
			s.log.Error("scheduler: disable traffic-exceeded sub", "sub_id", id, "err", err)
		}
	}
}

func (s *Scheduler) pollNodeTraffic(ctx context.Context, node db.Node) {
	inbounds, err := s.nodes.ListInbounds(ctx, node.ID)
	if err != nil {
		s.log.Error("scheduler: list inbounds", "node", node.Name, "err", err)
		return
	}

	// Group by protocol type for efficient API calls.
	var xrayInbounds []db.Inbound
	var hy2Inbound *db.Inbound
	for i := range inbounds {
		switch inbounds[i].Protocol {
		case "vless-xhttp", "vless-reality", "vless-ws":
			xrayInbounds = append(xrayInbounds, inbounds[i])
		case "hysteria2":
			hy2Inbound = &inbounds[i]
		}
	}

	if len(xrayInbounds) > 0 && node.XrayAPIAddr != nil {
		s.pollXrayTraffic(ctx, node, xrayInbounds)
	}
	if hy2Inbound != nil && node.Hy2APIURL != nil {
		s.pollHy2Traffic(ctx, node, *hy2Inbound)
	}
}

func (s *Scheduler) pollXrayTraffic(ctx context.Context, node db.Node, _ []db.Inbound) {
	client, err := xray.NewClient(*node.XrayAPIAddr, node.XrayAPITLS)
	if err != nil {
		s.log.Error("scheduler: xray client", "node", node.Name, "err", err)
		return
	}
	defer client.Close()

	// Get all subscriptions on this node.
	// We fetch all enabled subs and query their traffic from xray stats.
	subs, err := s.subs.ListAll(ctx, 10000, 0)
	if err != nil {
		s.log.Error("scheduler: list subs for traffic poll", "err", err)
		return
	}

	for _, sub := range subs {
		if !sub.IsEnabled {
			continue
		}
		email := xray.UserEmail(sub.Token)
		traffic, err := client.GetUserTraffic(ctx, email, true) // true = reset counter
		if err != nil {
			s.log.Warn("scheduler: get xray traffic", "email", email, "err", err)
			continue
		}
		total := traffic.BytesUp + traffic.BytesDown
		if total == 0 {
			continue
		}
		if err := s.subs.AddTraffic(ctx, sub.ID, total); err != nil {
			s.log.Error("scheduler: add traffic", "sub_id", sub.ID, "err", err)
		}
	}
}

func (s *Scheduler) pollHy2Traffic(ctx context.Context, node db.Node, _ db.Inbound) {
	secret := ""
	if node.Hy2APISecret != nil {
		secret = *node.Hy2APISecret
	}
	client := hysteria2.NewClient(*node.Hy2APIURL, secret)

	// clear=true resets counters after reading, preventing double-counting.
	stats, err := client.GetTraffic(ctx, true)
	if err != nil {
		s.log.Error("scheduler: hy2 get traffic", "node", node.Name, "err", err)
		return
	}

	// hysteria2 stats are keyed by the user ID returned from our auth backend:
	// "sub_<token>" (see internal/api/hy2auth and hysteria2.UserID).
	subs, err := s.subs.ListAll(ctx, 10000, 0)
	if err != nil {
		s.log.Error("scheduler: list subs for hy2 traffic", "err", err)
		return
	}

	// Build a map userID→sub for O(1) lookup.
	byUserID := make(map[string]db.Subscription, len(subs))
	for _, sub := range subs {
		byUserID[hysteria2.UserID(sub.Token)] = sub
	}

	for userID, t := range stats {
		sub, ok := byUserID[userID]
		if !ok {
			continue
		}
		total := t.TX + t.RX
		if total == 0 {
			continue
		}
		if err := s.subs.AddTraffic(ctx, sub.ID, total); err != nil {
			s.log.Error("scheduler: add hy2 traffic", "sub_id", sub.ID, "err", err)
		}
	}
}

// ─── Expiry checking ─────────────────────────────────────────────────────────

func (s *Scheduler) runExpiryChecker(ctx context.Context) {
	ticker := time.NewTicker(s.expiryInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.checkExpiry(ctx)
		}
	}
}

func (s *Scheduler) checkExpiry(ctx context.Context) {
	// Disable expired subscriptions
	expired, err := s.subs.MarkExpired(ctx)
	if err != nil {
		s.log.Error("scheduler: mark expired", "err", err)
		return
	}
	for _, id := range expired {
		s.log.Info("scheduler: subscription expired, disabling", "sub_id", id)
		if err := s.disabler.Disable(ctx, id); err != nil {
			s.log.Error("scheduler: disable expired sub", "sub_id", id, "err", err)
		}
	}

	if s.notify == nil {
		return
	}

	// Warn about subscriptions expiring in the next 3 days
	expiringSoon, err := s.subs.ListExpiringSoon(ctx, 3*24*time.Hour)
	if err != nil {
		s.log.Error("scheduler: list expiring soon", "err", err)
		return
	}
	for i := range expiringSoon {
		if expiringSoon[i].TelegramChatID != nil {
			s.notify.NotifyExpiringSoon(ctx, &expiringSoon[i])
		}
	}

	// Warn about subscriptions at ≥ 80% traffic
	allSubs, err := s.subs.ListAll(ctx, 10000, 0)
	if err != nil {
		return
	}
	for i := range allSubs {
		sub := &allSubs[i]
		if sub.TrafficLimitBytes == nil || *sub.TrafficLimitBytes == 0 {
			continue
		}
		if sub.TelegramChatID == nil || !sub.IsEnabled {
			continue
		}
		pct := float64(sub.TrafficUsedBytes) / float64(*sub.TrafficLimitBytes)
		if pct >= 0.80 && pct < 1.0 {
			s.notify.NotifyTrafficWarning(ctx, sub)
		}
	}
}
