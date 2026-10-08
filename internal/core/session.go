package core

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"sync"
	"time"
)

// SessionType identifies the protocol being exposed.
type SessionType string

const (
	TypeSSH   SessionType = "ssh"
	TypeHTTP  SessionType = "http"
	TypeHTTPS SessionType = "https"
)

// Session holds the runtime state of one ephemeral exposure.
type Session struct {
	Token     string      `json:"token"`
	Type      SessionType `json:"type"`
	Target    string      `json:"target"`
	TargetIP  string      `json:"target_ip"`
	Port      int         `json:"port"`
	GottyPort int         `json:"gotty_port,omitempty"`
	GottyPID  int         `json:"gotty_pid,omitempty"`
	CreatedAt time.Time   `json:"created_at"`
	ExpiresAt time.Time   `json:"expires_at"`
	PublicURL string      `json:"public_url"`
}

// Expired reports whether the TTL has elapsed.
func (s *Session) Expired() bool { return time.Now().After(s.ExpiresAt) }

// Store is a thread-safe in-memory session registry backed by a JSON file.
type Store struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	path     string
}

// NewStore creates a Store and loads any persisted state from path.
func NewStore(path string) *Store {
	st := &Store{sessions: make(map[string]*Session), path: path}
	_ = st.load()
	return st
}

func (st *Store) Add(s *Session) error {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.sessions[s.Token] = s
	return st.save()
}

func (st *Store) Remove(token string) error {
	st.mu.Lock()
	defer st.mu.Unlock()
	delete(st.sessions, token)
	return st.save()
}

// Active returns the first non-expired session, or nil.
func (st *Store) Active() *Session {
	st.mu.RLock()
	defer st.mu.RUnlock()
	for _, s := range st.sessions {
		if !s.Expired() {
			return s
		}
	}
	return nil
}

func (st *Store) Get(token string) *Session {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.sessions[token]
}

// AllExpired returns a snapshot of all expired sessions.
func (st *Store) AllExpired() []*Session {
	st.mu.RLock()
	defer st.mu.RUnlock()
	var out []*Session
	for _, s := range st.sessions {
		if s.Expired() {
			out = append(out, s)
		}
	}
	return out
}

func (st *Store) save() error {
	if st.path == "" {
		return nil
	}
	data, err := json.MarshalIndent(map[string]interface{}{"sessions": st.sessions}, "", "  ")
	if err != nil {
		return err
	}
	tmp := st.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, st.path)
}

func (st *Store) load() error {
	if st.path == "" {
		return nil
	}
	data, err := os.ReadFile(st.path)
	if err != nil {
		return nil
	}
	var env struct {
		Sessions map[string]*Session `json:"sessions"`
	}
	if err := json.Unmarshal(data, &env); err != nil {
		return err
	}
	if env.Sessions != nil {
		st.sessions = env.Sessions
	}
	return nil
}

// RandToken generates a cryptographically random 32-char hex token.
func RandToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
