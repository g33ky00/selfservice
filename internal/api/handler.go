package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/g33ky00/selfservice/internal/core"
)

// Handler routes HTTP requests to the Manager.
type Handler struct {
	mgr *core.Manager
}

// NewHandler creates a Handler backed by mgr.
func NewHandler(mgr *core.Manager) *Handler {
	return &Handler{mgr: mgr}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/v1")
	switch {
	case r.Method == http.MethodPost && path == "/sessions":
		h.provision(w, r)
	case r.Method == http.MethodDelete && strings.HasPrefix(path, "/sessions/"):
		token := strings.TrimPrefix(path, "/sessions/")
		h.destroy(w, r, token)
	case r.Method == http.MethodGet && path == "/sessions":
		h.status(w, r)
	case r.Method == http.MethodGet && path == "/healthz":
		w.WriteHeader(http.StatusOK)
	default:
		http.NotFound(w, r)
	}
}

func (h *Handler) provision(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Type     string `json:"type"`
		Target   string `json:"target"`
		TargetIP string `json:"target_ip"`
		Port     int    `json:"port"`
		SSHUser  string `json:"ssh_user"`
		TTL      int    `json:"ttl"` // seconds
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if body.TargetIP == "" {
		writeErr(w, http.StatusBadRequest, "target_ip required")
		return
	}
	if body.Type == "" {
		body.Type = "ssh"
	}
	req := core.ProvisionRequest{
		Type:     core.SessionType(body.Type),
		Target:   body.Target,
		TargetIP: body.TargetIP,
		Port:     body.Port,
		SSHUser:  body.SSHUser,
		TTL:      time.Duration(body.TTL) * time.Second,
	}
	result, err := h.mgr.Provision(r.Context(), req)
	if err != nil {
		writeErr(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (h *Handler) destroy(w http.ResponseWriter, r *http.Request, token string) {
	if token == "" {
		writeErr(w, http.StatusBadRequest, "token required")
		return
	}
	if err := h.mgr.Destroy(r.Context(), token); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "destroyed", "token": token})
}

func (h *Handler) status(w http.ResponseWriter, r *http.Request) {
	sess := h.mgr.Status()
	if sess == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"active": false})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"active": true, "session": sess})
}

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, detail string) {
	writeJSON(w, code, map[string]string{"status": "error", "details": detail})
}
