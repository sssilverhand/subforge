package admin

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/sssilverhand/subforge/internal/api/middleware"
	"github.com/sssilverhand/subforge/internal/config"
	"github.com/sssilverhand/subforge/internal/core/subscription"
	"github.com/sssilverhand/subforge/internal/db"
	"golang.org/x/crypto/bcrypt"
)

// ─── Dependencies injected into Handler ──────────────────────────────────────

type SubService interface {
	Create(ctx context.Context, p subscription.CreateParams) (*db.Subscription, error)
	Enable(ctx context.Context, id uuid.UUID) error
	Disable(ctx context.Context, id uuid.UUID) error
	Delete(ctx context.Context, id uuid.UUID) error
	ResetTraffic(ctx context.Context, id uuid.UUID) error
}

// Handler holds all admin API dependencies.
type Handler struct {
	cfg    *config.Config
	subs   *db.SubscriptionStore
	subSvc SubService
	nodes  *db.NodeStore
	users  *db.UserStore
	tokens *db.TokenStore
}

func NewHandler(
	cfg *config.Config,
	subs *db.SubscriptionStore,
	subSvc SubService,
	nodes *db.NodeStore,
	users *db.UserStore,
	tokens *db.TokenStore,
) *Handler {
	return &Handler{cfg: cfg, subs: subs, subSvc: subSvc,
		nodes: nodes, users: users, tokens: tokens}
}

// Routes mounts all admin endpoints.
func (h *Handler) Routes() http.Handler {
	r := chi.NewRouter()

	// Public: first-run setup + login
	r.Post("/setup", h.setup)
	r.Post("/auth/login", h.login)

	// Protected
	r.Group(func(r chi.Router) {
		r.Use(middleware.Auth(h.cfg.Auth.JWTSecret, h.tokens))

		// Subscriptions
		r.Get("/subscriptions", h.listSubscriptions)
		r.Post("/subscriptions", h.createSubscription)
		r.Get("/subscriptions/{id}", h.getSubscription)
		r.Delete("/subscriptions/{id}", h.deleteSubscription)
		r.Post("/subscriptions/{id}/enable", h.enableSubscription)
		r.Post("/subscriptions/{id}/disable", h.disableSubscription)
		r.Post("/subscriptions/{id}/reset-traffic", h.resetTraffic)

		// Nodes
		r.Get("/nodes", h.listNodes)
		r.Post("/nodes", h.createNode)
		r.Get("/nodes/{id}", h.getNode)
		r.Get("/nodes/{id}/inbounds", h.listInbounds)
		r.Post("/nodes/{id}/inbounds", h.createInbound)

		// Admin users (super_admin only)
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireRole("super_admin"))
			r.Get("/users", h.listUsers)
			r.Post("/users", h.createUser)
			r.Delete("/users/{id}", h.deleteUser)
		})

		// API tokens
		r.Get("/tokens", h.listTokens)
		r.Post("/tokens", h.createToken)
		r.Delete("/tokens/{id}", h.deleteToken)
	})

	return r
}

// ─── Setup & Auth ─────────────────────────────────────────────────────────────

func (h *Handler) setup(w http.ResponseWriter, r *http.Request) {
	if !h.cfg.Auth.SuperAdminSetup {
		writeError(w, "setup disabled", http.StatusForbidden)
		return
	}
	count, err := h.users.Count(r.Context())
	if err != nil {
		writeError(w, "db error", http.StatusInternalServerError)
		return
	}
	if count > 0 {
		writeError(w, "already set up", http.StatusConflict)
		return
	}

	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Username == "" || body.Password == "" {
		writeError(w, "username and password required", http.StatusBadRequest)
		return
	}
	if len(body.Password) < 8 {
		writeError(w, "password must be at least 8 characters", http.StatusBadRequest)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, "internal error", http.StatusInternalServerError)
		return
	}
	user, err := h.users.Create(r.Context(), db.CreateUserParams{
		Username:     body.Username,
		PasswordHash: string(hash),
		Role:         "super_admin",
	})
	if err != nil {
		writeError(w, "create user failed", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":       user.ID,
		"username": user.Username,
		"role":     user.Role,
	})
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, "invalid request", http.StatusBadRequest)
		return
	}

	user, err := h.users.GetByUsername(r.Context(), body.Username)
	if err != nil || user == nil || !user.IsActive {
		writeError(w, "invalid credentials", http.StatusUnauthorized)
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(body.Password)); err != nil {
		writeError(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	token, err := middleware.IssueJWT(
		h.cfg.Auth.JWTSecret,
		user.ID, user.Username, user.Role,
		h.cfg.Auth.TokenExpiry,
	)
	if err != nil {
		writeError(w, "token error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"token":      token,
		"expires_in": int(h.cfg.Auth.TokenExpiry.Seconds()),
		"role":       user.Role,
	})
}

// ─── Subscriptions ───────────────────────────────────────────────────────────

func (h *Handler) listSubscriptions(w http.ResponseWriter, r *http.Request) {
	limit, offset := pagination(r)
	subs, err := h.subs.ListAll(r.Context(), limit, offset)
	if err != nil {
		writeError(w, "db error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":  subsToView(subs, h.cfg.Server.ExternalURL),
		"limit":  limit,
		"offset": offset,
	})
}

func (h *Handler) createSubscription(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name              *string     `json:"name"`
		PlanID            *uuid.UUID  `json:"plan_id"`
		InboundIDs        []uuid.UUID `json:"inbound_ids"`
		TrafficLimitBytes *int64      `json:"traffic_limit_bytes"`
		ExpiresAt         *time.Time  `json:"expires_at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, "invalid request", http.StatusBadRequest)
		return
	}
	if len(body.InboundIDs) == 0 {
		writeError(w, "inbound_ids required", http.StatusBadRequest)
		return
	}

	claims := middleware.GetClaims(r)
	sub, err := h.subSvc.Create(r.Context(), subscription.CreateParams{
		Name:              body.Name,
		PlanID:            body.PlanID,
		InboundIDs:        body.InboundIDs,
		TrafficLimitBytes: body.TrafficLimitBytes,
		ExpiresAt:         body.ExpiresAt,
		CreatedBy:         &claims.UserID,
	})
	if err != nil {
		writeError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, subToView(sub, h.cfg.Server.ExternalURL))
}

func (h *Handler) getSubscription(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r, "id")
	if err != nil {
		writeError(w, "invalid id", http.StatusBadRequest)
		return
	}
	sub, err := h.subs.GetByID(r.Context(), id)
	if err != nil || sub == nil {
		writeError(w, "not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, subToView(sub, h.cfg.Server.ExternalURL))
}

func (h *Handler) deleteSubscription(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r, "id")
	if err != nil {
		writeError(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := h.subSvc.Delete(r.Context(), id); err != nil {
		writeError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) enableSubscription(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r, "id")
	if err != nil {
		writeError(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := h.subSvc.Enable(r.Context(), id); err != nil {
		writeError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) disableSubscription(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r, "id")
	if err != nil {
		writeError(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := h.subSvc.Disable(r.Context(), id); err != nil {
		writeError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) resetTraffic(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r, "id")
	if err != nil {
		writeError(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := h.subSvc.ResetTraffic(r.Context(), id); err != nil {
		writeError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ─── Nodes ───────────────────────────────────────────────────────────────────

func (h *Handler) listNodes(w http.ResponseWriter, r *http.Request) {
	nodes, err := h.nodes.ListActive(r.Context())
	if err != nil {
		writeError(w, "db error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": nodes})
}

func (h *Handler) createNode(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name         string  `json:"name"`
		PublicHost   string  `json:"public_host"`
		XrayAPIAddr  *string `json:"xray_api_addr"`
		XrayAPITLS   bool    `json:"xray_api_tls"`
		Hy2APIURL    *string `json:"hy2_api_url"`
		Hy2APISecret *string `json:"hy2_api_secret"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" || body.PublicHost == "" {
		writeError(w, "name and public_host required", http.StatusBadRequest)
		return
	}

	node, err := h.nodes.Create(r.Context(), db.CreateNodeParams{
		Name:         body.Name,
		PublicHost:   body.PublicHost,
		XrayAPIAddr:  body.XrayAPIAddr,
		XrayAPITLS:   body.XrayAPITLS,
		Hy2APIURL:    body.Hy2APIURL,
		Hy2APISecret: body.Hy2APISecret,
	})
	if err != nil {
		writeError(w, "create failed", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, node)
}

func (h *Handler) getNode(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r, "id")
	if err != nil {
		writeError(w, "invalid id", http.StatusBadRequest)
		return
	}
	node, err := h.nodes.GetByID(r.Context(), id)
	if err != nil || node == nil {
		writeError(w, "not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, node)
}

func (h *Handler) listInbounds(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r, "id")
	if err != nil {
		writeError(w, "invalid id", http.StatusBadRequest)
		return
	}
	inbounds, err := h.nodes.ListInbounds(r.Context(), id)
	if err != nil {
		writeError(w, "db error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": inbounds})
}

func (h *Handler) createInbound(w http.ResponseWriter, r *http.Request) {
	nodeID, err := parseUUID(r, "id")
	if err != nil {
		writeError(w, "invalid node id", http.StatusBadRequest)
		return
	}

	var body struct {
		Tag      string          `json:"tag"`
		Protocol string          `json:"protocol"`
		Port     int             `json:"port"`
		Settings json.RawMessage `json:"settings"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, "invalid request", http.StatusBadRequest)
		return
	}
	if body.Tag == "" || body.Protocol == "" || body.Port == 0 {
		writeError(w, "tag, protocol, and port required", http.StatusBadRequest)
		return
	}

	settings := []byte(body.Settings)
	if len(settings) == 0 {
		settings = []byte("{}")
	}

	ib, err := h.nodes.CreateInbound(r.Context(), db.CreateInboundParams{
		NodeID:   nodeID,
		Tag:      body.Tag,
		Protocol: body.Protocol,
		Port:     body.Port,
		Settings: settings,
	})
	if err != nil {
		writeError(w, "create failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, ib)
}

// ─── Admin Users ─────────────────────────────────────────────────────────────

func (h *Handler) listUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.users.List(r.Context())
	if err != nil {
		writeError(w, "db error", http.StatusInternalServerError)
		return
	}
	// Strip password hashes from response
	type userView struct {
		ID        uuid.UUID `json:"id"`
		Username  string    `json:"username"`
		Role      string    `json:"role"`
		IsActive  bool      `json:"is_active"`
		CreatedAt time.Time `json:"created_at"`
	}
	views := make([]userView, len(users))
	for i, u := range users {
		views[i] = userView{u.ID, u.Username, u.Role, u.IsActive, u.CreatedAt}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": views})
}

func (h *Handler) createUser(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Username == "" || body.Password == "" {
		writeError(w, "username and password required", http.StatusBadRequest)
		return
	}
	if body.Role == "" {
		body.Role = "operator"
	}
	if body.Role == "super_admin" {
		writeError(w, "cannot create super_admin via API", http.StatusForbidden)
		return
	}
	if len(body.Password) < 8 {
		writeError(w, "password must be at least 8 characters", http.StatusBadRequest)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, "internal error", http.StatusInternalServerError)
		return
	}
	user, err := h.users.Create(r.Context(), db.CreateUserParams{
		Username:     body.Username,
		PasswordHash: string(hash),
		Role:         body.Role,
	})
	if err != nil {
		writeError(w, "create failed", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":       user.ID,
		"username": user.Username,
		"role":     user.Role,
	})
}

func (h *Handler) deleteUser(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r, "id")
	if err != nil {
		writeError(w, "invalid id", http.StatusBadRequest)
		return
	}
	// Prevent self-deletion
	if claims := middleware.GetClaims(r); claims.UserID == id {
		writeError(w, "cannot delete yourself", http.StatusBadRequest)
		return
	}
	if err := h.users.Delete(r.Context(), id); err != nil {
		writeError(w, "delete failed", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ─── API Tokens ──────────────────────────────────────────────────────────────

func (h *Handler) listTokens(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r)
	tokens, err := h.tokens.ListByCreator(r.Context(), claims.UserID)
	if err != nil {
		writeError(w, "db error", http.StatusInternalServerError)
		return
	}
	type tokenView struct {
		ID        uuid.UUID  `json:"id"`
		Name      string     `json:"name"`
		Role      string     `json:"role"`
		ExpiresAt *time.Time `json:"expires_at"`
		CreatedAt time.Time  `json:"created_at"`
	}
	views := make([]tokenView, len(tokens))
	for i, t := range tokens {
		views[i] = tokenView{t.ID, t.Name, t.Role, t.ExpiresAt, t.CreatedAt}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": views})
}

func (h *Handler) createToken(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name      string     `json:"name"`
		Role      string     `json:"role"`
		ExpiresAt *time.Time `json:"expires_at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
		writeError(w, "name required", http.StatusBadRequest)
		return
	}
	if body.Role == "" {
		body.Role = "operator"
	}

	raw, err := generateRawToken()
	if err != nil {
		writeError(w, "internal error", http.StatusInternalServerError)
		return
	}

	claims := middleware.GetClaims(r)
	tok, err := h.tokens.Create(r.Context(), db.CreateTokenParams{
		Name:      body.Name,
		TokenHash: middleware.HashAPIToken(raw),
		Role:      body.Role,
		CreatedBy: &claims.UserID,
		ExpiresAt: body.ExpiresAt,
	})
	if err != nil {
		writeError(w, "create failed", http.StatusInternalServerError)
		return
	}
	// Raw token is returned ONCE — never stored
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":    tok.ID,
		"name":  tok.Name,
		"token": raw, // shown once
		"role":  tok.Role,
	})
}

func (h *Handler) deleteToken(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r, "id")
	if err != nil {
		writeError(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := h.tokens.Delete(r.Context(), id); err != nil {
		writeError(w, "delete failed", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, msg string, code int) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func parseUUID(r *http.Request, param string) (uuid.UUID, error) {
	return uuid.Parse(chi.URLParam(r, param))
}

func pagination(r *http.Request) (limit, offset int) {
	limit, _ = strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ = strconv.Atoi(r.URL.Query().Get("offset"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return
}

func generateRawToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// ─── View models ─────────────────────────────────────────────────────────────

type subView struct {
	ID                uuid.UUID  `json:"id"`
	Token             string     `json:"token"`
	Name              *string    `json:"name"`
	SubURL            string     `json:"sub_url"`
	UUID              uuid.UUID  `json:"uuid"`
	TrafficUsedBytes  int64      `json:"traffic_used_bytes"`
	TrafficLimitBytes *int64     `json:"traffic_limit_bytes"`
	ExpiresAt         *time.Time `json:"expires_at"`
	IsEnabled         bool       `json:"is_enabled"`
	IsTrafficExceeded bool       `json:"is_traffic_exceeded"`
	IsExpired         bool       `json:"is_expired"`
	CreatedAt         time.Time  `json:"created_at"`
	LastUsedAt        *time.Time `json:"last_used_at"`
}

func subToView(s *db.Subscription, baseURL string) subView {
	return subView{
		ID:                s.ID,
		Token:             s.Token,
		Name:              s.Name,
		SubURL:            baseURL + "/sub/" + s.Token,
		UUID:              s.UUID,
		TrafficUsedBytes:  s.TrafficUsedBytes,
		TrafficLimitBytes: s.TrafficLimitBytes,
		ExpiresAt:         s.ExpiresAt,
		IsEnabled:         s.IsEnabled,
		IsTrafficExceeded: s.IsTrafficExceeded,
		IsExpired:         s.IsExpired,
		CreatedAt:         s.CreatedAt,
		LastUsedAt:        s.LastUsedAt,
	}
}

func subsToView(subs []db.Subscription, baseURL string) []subView {
	views := make([]subView, len(subs))
	for i, s := range subs {
		sc := s
		views[i] = subToView(&sc, baseURL)
	}
	return views
}
