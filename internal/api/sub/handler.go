package sub

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/sssilverhand/subforge/internal/subgen"
	"github.com/sssilverhand/subforge/internal/subgen/clash"
	"github.com/sssilverhand/subforge/internal/subgen/raw"
	"github.com/sssilverhand/subforge/internal/subgen/singbox"
)

// Store is the minimal interface the handler needs.
type Store interface {
	GetSubscriptionByToken(token string) (*Subscription, error)
	UpdateLastUsed(id string, t time.Time) error
}

type Subscription struct {
	ID                 string
	Name               string
	UserUUID           uuid.UUID
	Hy2Password        string
	IsEnabled          bool
	TrafficUsedBytes   int64
	TrafficLimitBytes  int64 // 0 = unlimited
	ExpiresAt          *time.Time
	Endpoints          []subgen.Endpoint
}

type Handler struct {
	store Store
}

func NewHandler(store Store) *Handler {
	return &Handler{store: store}
}

func (h *Handler) Routes() http.Handler {
	r := chi.NewRouter()
	r.Get("/{token}", h.serve)
	return r
}

// serve handles GET /sub/{token}?client=singbox
//
// Supported client values:
//
//	singbox  → sing-box JSON
//	clash    → Clash Meta YAML
//	raw      → base64 URI list (default)
func (h *Handler) serve(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	if token == "" {
		http.NotFound(w, r)
		return
	}

	sub, err := h.store.GetSubscriptionByToken(token)
	if err != nil || sub == nil {
		http.NotFound(w, r)
		return
	}

	if !sub.IsEnabled {
		http.Error(w, "subscription disabled", http.StatusForbidden)
		return
	}

	data := subgen.SubData{
		SubName:     sub.Name,
		UserUUID:    sub.UserUUID,
		Hy2Password: sub.Hy2Password,
		Endpoints:   sub.Endpoints,
	}

	var gen subgen.Generator
	switch strings.ToLower(r.URL.Query().Get("client")) {
	case "singbox", "sing-box", "hiddify", "nekobox":
		gen = singbox.New()
	case "clash", "clashmetaforandroid", "clashverge", "mihomo":
		gen = clash.New()
	default:
		gen = raw.New()
	}

	body, err := gen.Generate(data)
	if err != nil {
		http.Error(w, "generation error", http.StatusInternalServerError)
		return
	}

	_ = h.store.UpdateLastUsed(sub.ID, time.Now())

	w.Header().Set("Content-Type", gen.ContentType())
	w.Header().Set("Content-Disposition", "attachment; filename=\"subforge.txt\"")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Subscription-Userinfo", buildUserinfo(sub))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

// buildUserinfo returns the Subscription-Userinfo header understood by v2rayN/NekoRay/Hiddify.
// Format: upload=<bytes>; download=<bytes>; total=<bytes>; expire=<unix>
func buildUserinfo(sub *Subscription) string {
	total := sub.TrafficLimitBytes
	expire := int64(0)
	if sub.ExpiresAt != nil {
		expire = sub.ExpiresAt.Unix()
	}
	// clients treat upload+download as total used; we report all used as download
	return fmt.Sprintf("upload=0; download=%d; total=%d; expire=%d",
		sub.TrafficUsedBytes, total, expire)
}
