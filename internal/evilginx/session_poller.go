package evilginx

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"
)

// CapturedCookie is the per-cookie payload Evilginx writes to its
// session store. Mirrors the on-disk JSON schema produced by Evilginx
// 3.3.0 (camel-case JSON keys come from the upstream Go struct tags).
type CapturedCookie struct {
	Name     string `json:"Name"`
	Value    string `json:"Value"`
	Path     string `json:"Path"`
	HttpOnly bool   `json:"HttpOnly"`
}

// CapturedSession represents a session captured by Evilginx. The
// JSON tags match what Evilginx writes to its persistence file.
type CapturedSession struct {
	IDInt      int                                  `json:"id"`
	ID         string                               `json:"-"`
	Phishlet   string                               `json:"phishlet"`
	Username   string                               `json:"username"`
	Password   string                               `json:"password"`
	Tokens     map[string]map[string]CapturedCookie `json:"tokens"`
	SessionID  string                               `json:"session_id"`
	LandingURL string                               `json:"landing_url"`
	UserAgent  string                               `json:"useragent"`
	RemoteAddr string                               `json:"remote_addr"`
	CreateTime int64                                `json:"create_time"`
	UpdateTime int64                                `json:"update_time"`
}

// Status returns "Visited", "Vulnerable", or "Exploitable" based on
// what the proxy captured. Drives the dashboard status badge and the
// per-session recommendation panel.
//
//	Visited     - victim clicked the lure but never authenticated.
//	Vulnerable  - victim entered a username/password (cred POST captured).
//	Exploitable - post-auth session cookies (ESTSAUTH/ESTSAUTHPERSISTENT)
//	              were stolen, replayable for full account takeover without
//	              password OR MFA OR passkey.
func (s CapturedSession) Status() string {
	if s.HasReplayableSessionCookie() {
		return "Exploitable"
	}
	if s.Username != "" || s.Password != "" {
		return "Vulnerable"
	}
	return "Visited"
}

// HasReplayableSessionCookie reports whether any captured cookie is
// a Microsoft session-grade auth token. Replay of either ESTSAUTH or
// ESTSAUTHPERSISTENT is sufficient to impersonate the user against
// login.microsoftonline.com.
func (s CapturedSession) HasReplayableSessionCookie() bool {
	for _, jar := range s.Tokens {
		for name := range jar {
			if name == "ESTSAUTH" || name == "ESTSAUTHPERSISTENT" {
				return true
			}
		}
	}
	return false
}

// CookieCount returns the total cookie count across all domains.
func (s CapturedSession) CookieCount() int {
	n := 0
	for _, jar := range s.Tokens {
		n += len(jar)
	}
	return n
}

// SessionCallback is called when new or updated sessions are observed.
type SessionCallback func(session CapturedSession)

// SessionPoller polls the Evilginx persistence file for new captured
// sessions. Evilginx 3.3.0 stores sessions as JSON blobs inside a
// Redis-RESP-formatted append-only file rather than the bbolt format
// used by earlier versions; this poller scans for top-level JSON
// objects directly so it works with either format.
type SessionPoller struct {
	dbPath   string
	interval time.Duration
	callback SessionCallback
	lastSeen map[string]int64
}

func NewSessionPoller(dbPath string, interval time.Duration, callback SessionCallback) *SessionPoller {
	return &SessionPoller{
		dbPath:   dbPath,
		interval: interval,
		callback: callback,
		lastSeen: make(map[string]int64),
	}
}

// Start begins polling in a goroutine. Cancel the context to stop.
func (p *SessionPoller) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(p.interval)
		defer ticker.Stop()
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
		prev, seen := p.lastSeen[s.ID]
		if !seen || s.UpdateTime > prev {
			p.lastSeen[s.ID] = s.UpdateTime
			p.callback(s)
		}
	}
}

func (p *SessionPoller) readSessions() ([]CapturedSession, error) {
	fi, err := os.Stat(p.dbPath)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err == nil && fi.Size() == 0 {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", p.dbPath, err)
	}

	data, err := os.ReadFile(p.dbPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", p.dbPath, err)
	}
	return scanSessionsFromBytes(data), nil
}

// scanSessionsFromBytes finds top-level JSON objects in the byte
// stream and returns those that look like session records (identified
// by a non-empty "phishlet" field). When the same session ID appears
// multiple times (Evilginx writes each update as a fresh RESP frame),
// only the version with the highest update_time is returned.
func scanSessionsFromBytes(data []byte) []CapturedSession {
	latest := make(map[string]CapturedSession)
	i := 0
	for i < len(data) {
		if data[i] != '{' {
			i++
			continue
		}
		end := matchJSONObject(data, i)
		if end < 0 {
			i++
			continue
		}
		var s CapturedSession
		if err := json.Unmarshal(data[i:end+1], &s); err == nil && s.Phishlet != "" {
			s.ID = strconv.Itoa(s.IDInt)
			if existing, ok := latest[s.ID]; !ok || s.UpdateTime >= existing.UpdateTime {
				latest[s.ID] = s
			}
		}
		i = end + 1
	}
	out := make([]CapturedSession, 0, len(latest))
	for _, s := range latest {
		out = append(out, s)
	}
	return out
}

// matchJSONObject returns the index of the closing '}' that pairs
// with the '{' at start, or -1 if not found / unbalanced. Strings
// and their escapes are skipped so braces in string literals do not
// affect depth.
func matchJSONObject(data []byte, start int) int {
	if start >= len(data) || data[start] != '{' {
		return -1
	}
	depth := 0
	inStr := false
	esc := false
	for j := start; j < len(data); j++ {
		c := data[j]
		if esc {
			esc = false
			continue
		}
		if inStr {
			if c == '\\' {
				esc = true
			} else if c == '"' {
				inStr = false
			}
			continue
		}
		switch c {
		case '"':
			inStr = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return j
			}
		}
	}
	return -1
}
