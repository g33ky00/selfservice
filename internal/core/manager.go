package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

// Config holds the daemon configuration.
type Config struct {
	CFToken     string
	CFAccountID string
	TunnelID    string
	PublicHost  string
	StateDir    string
	GottyBin    string
	DefaultTTL  time.Duration // 0 → 15m
	MaxTTL      time.Duration // 0 → 2h
}

func (c *Config) applyDefaults() {
	if c.DefaultTTL <= 0 {
		c.DefaultTTL = 15 * time.Minute
	}
	if c.MaxTTL <= 0 {
		c.MaxTTL = 2 * time.Hour
	}
	if c.StateDir == "" {
		home := os.Getenv("HOME")
		if home == "" {
			home = "/tmp"
		}
		c.StateDir = filepath.Join(home, ".selfservice")
	}
}

// ProvisionRequest is the payload for POST /v1/sessions.
type ProvisionRequest struct {
	Type     SessionType
	Target   string
	TargetIP string
	Port     int
	SSHUser  string
	TTL      time.Duration // 0 → DefaultTTL
}

// ProvisionResult is returned on success.
type ProvisionResult struct {
	Token     string    `json:"token"`
	PublicURL string    `json:"public_url"`
	ExpiresAt time.Time `json:"expires_at"`
	TTL       int       `json:"ttl_seconds"`
}

// Manager orchestrates session lifecycle.
type Manager struct {
	cfg   Config
	cf    *cfClient
	store *Store
	gotty gottyConfig
	mu    sync.Mutex
	gc    map[string]*time.Timer
}

// NewManager builds a Manager and purges expired sessions from a previous run.
func NewManager(cfg Config) (*Manager, error) {
	cfg.applyDefaults()
	if err := os.MkdirAll(cfg.StateDir, 0700); err != nil {
		return nil, fmt.Errorf("selfservice: mkdir %s: %w", cfg.StateDir, err)
	}
	m := &Manager{
		cfg:   cfg,
		cf:    newCFClient(cfg.CFAccountID, cfg.TunnelID, cfg.CFToken, cfg.PublicHost),
		store: NewStore(filepath.Join(cfg.StateDir, "sessions.json")),
		gotty: gottyConfig{
			Bin:    cfg.GottyBin,
			Host:   cfg.PublicHost,
			LogDir: filepath.Join(cfg.StateDir, "logs"),
		},
		gc: make(map[string]*time.Timer),
	}
	m.mu.Lock()
	m.purgeExpiredLocked()
	m.mu.Unlock()
	return m, nil
}

// Provision creates a new ephemeral session.
func (m *Manager) Provision(ctx context.Context, req ProvisionRequest) (*ProvisionResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if active := m.store.Active(); active != nil {
		return nil, fmt.Errorf("active session already running: %s (expires %s)",
			active.Token, active.ExpiresAt.Format(time.RFC3339))
	}

	ttl := req.TTL
	if ttl <= 0 {
		ttl = m.cfg.DefaultTTL
	}
	if ttl > m.cfg.MaxTTL {
		ttl = m.cfg.MaxTTL
	}

	token, err := RandToken()
	if err != nil {
		return nil, fmt.Errorf("generate token: %w", err)
	}

	now := time.Now()
	sess := &Session{
		Token:     token,
		Type:      req.Type,
		Target:    req.Target,
		TargetIP:  req.TargetIP,
		Port:      req.Port,
		CreatedAt: now,
		ExpiresAt: now.Add(ttl),
		PublicURL: fmt.Sprintf("https://%s/%s", m.cfg.PublicHost, token),
	}

	if req.Type == TypeSSH {
		if m.cfg.GottyBin == "" {
			return nil, fmt.Errorf("gotty_bin not configured for SSH sessions")
		}
		proc, gPort, err := m.launchGotty(req, token)
		if err != nil {
			return nil, err
		}
		sess.GottyPID = proc.Pid
		sess.GottyPort = gPort
	}

	ingressPort := req.Port
	if req.Type == TypeSSH {
		ingressPort = sess.GottyPort
	}

	if err := m.cf.SetIngress(token, ingressPort); err != nil {
		m.killGotty(sess)
		return nil, fmt.Errorf("CF ingress: %w", err)
	}

	if err := m.store.Add(sess); err != nil {
		_ = m.cf.ResetIngress()
		m.killGotty(sess)
		return nil, fmt.Errorf("persist session: %w", err)
	}

	t := time.AfterFunc(ttl, func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		m.destroyLocked(token)
	})
	m.gc[token] = t

	return &ProvisionResult{
		Token:     token,
		PublicURL: sess.PublicURL,
		ExpiresAt: sess.ExpiresAt,
		TTL:       int(ttl.Seconds()),
	}, nil
}

// Destroy terminates the session identified by token.
func (m *Manager) Destroy(_ context.Context, token string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.destroyLocked(token)
	return nil
}

// Status returns the currently active session or nil.
func (m *Manager) Status() *Session {
	return m.store.Active()
}

func (m *Manager) destroyLocked(token string) {
	if t, ok := m.gc[token]; ok {
		t.Stop()
		delete(m.gc, token)
	}
	if sess := m.store.Get(token); sess != nil {
		m.killGotty(sess)
	}
	_ = m.cf.ResetIngress()
	_ = m.store.Remove(token)
}

func (m *Manager) purgeExpiredLocked() {
	for _, s := range m.store.AllExpired() {
		m.destroyLocked(s.Token)
	}
}

func (m *Manager) launchGotty(req ProvisionRequest, token string) (*os.Process, int, error) {
	user := req.SSHUser
	if user == "" {
		user = "root"
	}
	var args []string
	if req.Port != 0 && req.Port != 22 {
		args = append(args, "-p", strconv.Itoa(req.Port))
	}
	args = append(args, fmt.Sprintf("%s@%s", user, req.TargetIP))
	return startGotty(m.gotty, args, token)
}

func (m *Manager) killGotty(s *Session) {
	if s.GottyPID <= 0 {
		return
	}
	if p, err := os.FindProcess(s.GottyPID); err == nil {
		_ = p.Kill()
	}
}
