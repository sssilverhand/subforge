package api

import (
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	agentcfg "github.com/sssilverhand/subforge/internal/agent/config"
	hy2gen "github.com/sssilverhand/subforge/internal/agent/confgen/hysteria2"
	xraygen "github.com/sssilverhand/subforge/internal/agent/confgen/xray"
	"github.com/sssilverhand/subforge/internal/agent/runtime"
)

type Handler struct {
	cfg *agentcfg.Config
	svc *runtime.ServiceManager
}

func NewHandler(cfg *agentcfg.Config) *Handler {
	return &Handler{cfg: cfg, svc: runtime.NewServiceManager()}
}

func (h *Handler) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(h.authMiddleware)

	r.Get("/status", h.status)

	r.Post("/xray/install", h.installXray)
	r.Post("/xray/config", h.writeXrayConfig)
	r.Post("/xray/restart", h.restartXray)

	r.Post("/hysteria2/install", h.installHysteria2)
	r.Post("/hysteria2/config", h.writeHysteria2Config)
	r.Post("/hysteria2/restart", h.restartHysteria2)

	return r
}

// ─── Status ──────────────────────────────────────────────────────────────────

func (h *Handler) status(w http.ResponseWriter, r *http.Request) {
	xraySt, _ := h.svc.Status(h.cfg.Xray.Service)
	hy2St, _ := h.svc.Status(h.cfg.Hysteria2.Service)

	writeJSON(w, http.StatusOK, map[string]any{
		"xray": map[string]any{
			"running": xraySt != nil && xraySt.Running,
			"version": binaryVersion(h.cfg.Xray.BinaryPath, "version"),
			"service": h.cfg.Xray.Service,
		},
		"hysteria2": map[string]any{
			"running": hy2St != nil && hy2St.Running,
			"version": binaryVersion(h.cfg.Hysteria2.BinaryPath, "version"),
			"service": h.cfg.Hysteria2.Service,
		},
	})
}

// ─── Xray ────────────────────────────────────────────────────────────────────

func (h *Handler) installXray(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Version string `json:"version"` // empty = latest
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	info, err := runtime.InstallXray(body.Version, h.cfg.Xray.BinaryPath)
	if err != nil {
		writeError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = h.svc.Restart(h.cfg.Xray.Service)
	writeJSON(w, http.StatusOK, info)
}

func (h *Handler) writeXrayConfig(w http.ResponseWriter, r *http.Request) {
	var cfg xraygen.Config
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, "invalid config: "+err.Error(), http.StatusBadRequest)
		return
	}

	data, err := xraygen.Generate(cfg)
	if err != nil {
		writeError(w, "generate config: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if err := writeFile(h.cfg.Xray.ConfigPath, data); err != nil {
		writeError(w, "write config: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if err := h.svc.Restart(h.cfg.Xray.Service); err != nil {
		writeError(w, "restart xray: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) restartXray(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Restart(h.cfg.Xray.Service); err != nil {
		writeError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ─── Hysteria2 ───────────────────────────────────────────────────────────────

func (h *Handler) installHysteria2(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Version string `json:"version"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	info, err := runtime.InstallHysteria2(body.Version, h.cfg.Hysteria2.BinaryPath)
	if err != nil {
		writeError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = h.svc.Restart(h.cfg.Hysteria2.Service)
	writeJSON(w, http.StatusOK, info)
}

func (h *Handler) writeHysteria2Config(w http.ResponseWriter, r *http.Request) {
	var cfg hy2gen.Config
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, "invalid config: "+err.Error(), http.StatusBadRequest)
		return
	}

	data, err := hy2gen.Generate(cfg)
	if err != nil {
		writeError(w, "generate config: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if err := writeFile(h.cfg.Hysteria2.ConfigPath, data); err != nil {
		writeError(w, "write config: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if err := h.svc.Restart(h.cfg.Hysteria2.Service); err != nil {
		writeError(w, "restart hysteria2: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) restartHysteria2(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Restart(h.cfg.Hysteria2.Service); err != nil {
		writeError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func (h *Handler) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bearer := r.Header.Get("Authorization")
		token, _ := strings.CutPrefix(bearer, "Bearer ")
		if strings.TrimSpace(token) != h.cfg.Server.Secret {
			writeError(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0640)
}

func binaryVersion(path, arg string) string {
	out, err := exec.Command(path, arg).Output()
	if err != nil {
		return "unknown"
	}
	// Return first line only
	line := strings.SplitN(strings.TrimSpace(string(out)), "\n", 2)[0]
	return line
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, msg string, code int) {
	writeJSON(w, code, map[string]string{"error": msg})
}
