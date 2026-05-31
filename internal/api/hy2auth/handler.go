package hy2auth

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/sssilverhand/subforge/internal/db"
)

// SubLookup checks if a hysteria2 password belongs to an active subscription.
type SubLookup interface {
	GetByHy2Password(ctx context.Context, password string) (*db.Subscription, error)
}

// Handler implements the hysteria2 HTTP authentication backend.
//
// hysteria2 calls POST /internal/hy2/auth with form fields:
//
//	addr  — client IP:port
//	auth  — the password the client sent
//	recv  — bytes received so far (always 0 on first call)
//	send  — bytes sent so far (always 0 on first call)
//
// We respond with JSON: {"ok": true, "id": "<user_id>"} or {"ok": false, "msg": "..."}
// The "id" we return becomes the key in /traffic and /kick APIs.
type Handler struct {
	lookup SubLookup
}

func NewHandler(lookup SubLookup) *Handler {
	return &Handler{lookup: lookup}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		writeAuthResp(w, false, "", "bad request")
		return
	}

	password := r.FormValue("auth")
	if password == "" {
		writeAuthResp(w, false, "", "missing auth")
		return
	}

	sub, err := h.lookup.GetByHy2Password(r.Context(), password)
	if err != nil || sub == nil {
		writeAuthResp(w, false, "", "not found")
		return
	}

	if !sub.IsEnabled || sub.IsExpired || sub.IsTrafficExceeded {
		writeAuthResp(w, false, "", "disabled")
		return
	}

	// Return "sub_<token>" as the user ID so /traffic and /kick use it as key.
	userID := "sub_" + sub.Token
	writeAuthResp(w, true, userID, "")
}

type authResponse struct {
	OK      bool   `json:"ok"`
	ID      string `json:"id,omitempty"`
	Message string `json:"msg,omitempty"`
}

func writeAuthResp(w http.ResponseWriter, ok bool, id, msg string) {
	w.Header().Set("Content-Type", "application/json")
	if ok {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusOK) // hysteria2 expects 200 even for rejections
	}
	_ = json.NewEncoder(w).Encode(authResponse{OK: ok, ID: id, Message: msg})
}
