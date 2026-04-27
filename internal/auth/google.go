// Package auth provides Google OAuth login and a session-cookie
// middleware that gates the dashboard. Configuration is read from
// environment variables on startup; if the OAuth client id/secret
// are unset the middleware is a no-op so legacy nginx basic-auth
// gating still works during incremental rollout.
package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// Config holds the runtime OAuth configuration.
type Config struct {
	ClientID       string
	ClientSecret   string
	RedirectURL    string
	AllowedEmails  []string
	AllowedDomains []string
	SessionSecret  []byte
	SessionTTL     time.Duration
	CookieSecure   bool
	// AllowlistFile is the JSON file backing the runtime-editable
	// allowlist. Bootstrap entries from env vars are merged with
	// it; runtime add/remove ops persist here. Default:
	// /etc/evilginx-lab/allowlist.json
	AllowlistFile string
}

// Handler exposes the OAuth login/callback/logout HTTP handlers and a
// gating middleware. Construct with NewHandler.
type Handler struct {
	cfg            Config
	oauth          *oauth2.Config
	mu             sync.RWMutex
	runtimeEmails  []string
	runtimeDomains []string
}

type allowlistFile struct {
	Emails  []string `json:"emails"`
	Domains []string `json:"domains"`
}

// NewHandler builds a Handler from a Config and seeds the runtime
// allowlist from env config + on-disk allowlist file (if any).
func NewHandler(cfg Config) *Handler {
	h := &Handler{
		cfg: cfg,
		oauth: &oauth2.Config{
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
			RedirectURL:  cfg.RedirectURL,
			Scopes:       []string{"openid", "email", "profile"},
			Endpoint:     google.Endpoint,
		},
	}
	h.runtimeEmails = mergeUnique(nil, cfg.AllowedEmails)
	h.runtimeDomains = mergeUnique(nil, cfg.AllowedDomains)
	if cfg.AllowlistFile != "" {
		if data, err := os.ReadFile(cfg.AllowlistFile); err == nil {
			var f allowlistFile
			if err := json.Unmarshal(data, &f); err == nil {
				h.runtimeEmails = mergeUnique(h.runtimeEmails, f.Emails)
				h.runtimeDomains = mergeUnique(h.runtimeDomains, f.Domains)
			}
		}
	}
	return h
}

func mergeUnique(a, b []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(a)+len(b))
	for _, list := range [][]string{a, b} {
		for _, v := range list {
			v = strings.ToLower(strings.TrimSpace(v))
			if v == "" {
				continue
			}
			if _, ok := seen[v]; ok {
				continue
			}
			seen[v] = struct{}{}
			out = append(out, v)
		}
	}
	return out
}

// ListAllowed returns the current allowlist (env + runtime additions).
func (h *Handler) ListAllowed() ([]string, []string) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	emails := append([]string(nil), h.runtimeEmails...)
	domains := append([]string(nil), h.runtimeDomains...)
	return emails, domains
}

// AddAllowed adds an email or domain to the runtime allowlist and
// persists it. An entry containing "@" is treated as an email;
// otherwise as a domain.
func (h *Handler) AddAllowed(entry string) error {
	entry = strings.ToLower(strings.TrimSpace(entry))
	if entry == "" {
		return errors.New("empty entry")
	}
	h.mu.Lock()
	if strings.Contains(entry, "@") {
		h.runtimeEmails = mergeUnique(h.runtimeEmails, []string{entry})
	} else {
		h.runtimeDomains = mergeUnique(h.runtimeDomains, []string{entry})
	}
	h.mu.Unlock()
	return h.persist()
}

// RemoveAllowed removes an email or domain from the runtime allowlist.
// Bootstrap entries from env config are also removable at runtime;
// they will return on next process restart unless the env config is
// also updated.
func (h *Handler) RemoveAllowed(entry string) error {
	entry = strings.ToLower(strings.TrimSpace(entry))
	if entry == "" {
		return errors.New("empty entry")
	}
	h.mu.Lock()
	if strings.Contains(entry, "@") {
		h.runtimeEmails = removeFrom(h.runtimeEmails, entry)
	} else {
		h.runtimeDomains = removeFrom(h.runtimeDomains, entry)
	}
	h.mu.Unlock()
	return h.persist()
}

func removeFrom(list []string, target string) []string {
	out := make([]string, 0, len(list))
	for _, v := range list {
		if !strings.EqualFold(v, target) {
			out = append(out, v)
		}
	}
	return out
}

func (h *Handler) persist() error {
	if h.cfg.AllowlistFile == "" {
		return nil
	}
	h.mu.RLock()
	body, _ := json.MarshalIndent(allowlistFile{Emails: h.runtimeEmails, Domains: h.runtimeDomains}, "", "  ")
	h.mu.RUnlock()
	tmp := h.cfg.AllowlistFile + ".tmp"
	if err := os.WriteFile(tmp, body, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, h.cfg.AllowlistFile)
}

// Enabled reports whether OAuth is configured. If false the
// Middleware allows all requests through (legacy mode).
func (h *Handler) Enabled() bool {
	return h != nil && h.cfg.ClientID != "" && h.cfg.ClientSecret != ""
}

// Login starts the OAuth flow by redirecting the user to Google.
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	state, err := randString(32)
	if err != nil {
		http.Error(w, "auth setup failed", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    state,
		Path:     "/",
		MaxAge:   600,
		HttpOnly: true,
		Secure:   h.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, h.oauth.AuthCodeURL(state), http.StatusFound)
}

// Callback handles Google's redirect, exchanges the auth code for
// tokens, fetches userinfo, applies the email/domain allowlist, and
// issues a signed session cookie on success.
func (h *Handler) Callback(w http.ResponseWriter, r *http.Request) {
	stateCookie, err := r.Cookie("oauth_state")
	if err != nil || r.URL.Query().Get("state") != stateCookie.Value {
		http.Error(w, "invalid oauth state", http.StatusBadRequest)
		return
	}
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "missing code", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	tok, err := h.oauth.Exchange(ctx, code)
	if err != nil {
		http.Error(w, "token exchange failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	client := h.oauth.Client(ctx, tok)
	resp, err := client.Get("https://www.googleapis.com/oauth2/v2/userinfo")
	if err != nil {
		http.Error(w, "userinfo failed", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		http.Error(w, "userinfo non-200", http.StatusInternalServerError)
		return
	}
	var ui struct {
		Email         string `json:"email"`
		VerifiedEmail bool   `json:"verified_email"`
		Name          string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&ui); err != nil {
		http.Error(w, "bad userinfo", http.StatusInternalServerError)
		return
	}
	if !ui.VerifiedEmail {
		http.Error(w, "email not verified by Google", http.StatusForbidden)
		return
	}
	if !h.allow(ui.Email) {
		http.Error(w, "access denied for "+ui.Email, http.StatusForbidden)
		return
	}

	sess, err := h.signSession(ui.Email, time.Now().Add(h.cfg.SessionTTL))
	if err != nil {
		http.Error(w, "session signing failed", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "ecc_sess",
		Value:    sess,
		Path:     "/",
		MaxAge:   int(h.cfg.SessionTTL.Seconds()),
		HttpOnly: true,
		Secure:   h.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
	http.SetCookie(w, &http.Cookie{Name: "oauth_state", Path: "/", MaxAge: -1})
	http.Redirect(w, r, "/", http.StatusFound)
}

// Logout clears the session cookie and redirects to login.
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: "ecc_sess", Path: "/", MaxAge: -1})
	http.Redirect(w, r, "/auth/google/login", http.StatusFound)
}

// WhoAmI returns the authenticated user's email as JSON.
func (h *Handler) WhoAmI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if !h.Enabled() {
		_, _ = w.Write([]byte(`{"enabled":false}`))
		return
	}
	c, err := r.Cookie("ecc_sess")
	if err != nil {
		_, _ = w.Write([]byte(`{"enabled":true,"email":null}`))
		return
	}
	email, err := h.readSession(c.Value)
	if err != nil {
		_, _ = w.Write([]byte(`{"enabled":true,"email":null}`))
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"enabled": true, "email": email})
}

// Middleware gates requests. If OAuth is not enabled it is a no-op.
// Public paths (/healthz, /auth/*) are always passed through.
func (h *Handler) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !h.Enabled() {
			next.ServeHTTP(w, r)
			return
		}
		p := r.URL.Path
		if p == "/healthz" || strings.HasPrefix(p, "/auth/") {
			next.ServeHTTP(w, r)
			return
		}
		c, err := r.Cookie("ecc_sess")
		if err == nil {
			if _, e := h.readSession(c.Value); e == nil {
				next.ServeHTTP(w, r)
				return
			}
		}
		if r.Method == http.MethodGet && !strings.HasPrefix(p, "/api/") && !strings.HasPrefix(p, "/ws") {
			http.Redirect(w, r, "/auth/google/login", http.StatusFound)
			return
		}
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	})
}

func (h *Handler) allow(email string) bool {
	email = strings.ToLower(strings.TrimSpace(email))
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, e := range h.runtimeEmails {
		if strings.EqualFold(e, email) {
			return true
		}
	}
	if at := strings.LastIndex(email, "@"); at >= 0 {
		domain := email[at+1:]
		for _, d := range h.runtimeDomains {
			if strings.EqualFold(d, domain) {
				return true
			}
		}
	}
	return false
}

func (h *Handler) signSession(email string, exp time.Time) (string, error) {
	body, err := json.Marshal(struct {
		E string `json:"e"`
		X int64  `json:"x"`
	}{email, exp.Unix()})
	if err != nil {
		return "", err
	}
	bodyB64 := base64.RawURLEncoding.EncodeToString(body)
	mac := hmac.New(sha256.New, h.cfg.SessionSecret)
	mac.Write([]byte(bodyB64))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return bodyB64 + "." + sig, nil
}

func (h *Handler) readSession(token string) (string, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return "", errors.New("malformed token")
	}
	body, sig := parts[0], parts[1]
	mac := hmac.New(sha256.New, h.cfg.SessionSecret)
	mac.Write([]byte(body))
	expected := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(sig), []byte(expected)) {
		return "", errors.New("bad signature")
	}
	bodyBytes, err := base64.RawURLEncoding.DecodeString(body)
	if err != nil {
		return "", err
	}
	var payload struct {
		E string `json:"e"`
		X int64  `json:"x"`
	}
	if err := json.Unmarshal(bodyBytes, &payload); err != nil {
		return "", err
	}
	if time.Now().Unix() >= payload.X {
		return "", errors.New("session expired")
	}
	return payload.E, nil
}

// LoadConfigFromEnv reads OAuth configuration from environment.
// Returns a zero-value Config when env vars are unset.
func LoadConfigFromEnv() Config {
	secret := []byte(os.Getenv("SESSION_COOKIE_SECRET"))
	if len(secret) < 16 {
		secret, _ = randBytes(32)
	}
	allowlistFile := os.Getenv("OAUTH_ALLOWLIST_FILE")
	if allowlistFile == "" {
		allowlistFile = "/etc/evilginx-lab/allowlist.json"
	}
	return Config{
		ClientID:       os.Getenv("GOOGLE_OAUTH_CLIENT_ID"),
		ClientSecret:   os.Getenv("GOOGLE_OAUTH_CLIENT_SECRET"),
		RedirectURL:    os.Getenv("GOOGLE_OAUTH_REDIRECT_URL"),
		AllowedEmails:  splitCSV(os.Getenv("GOOGLE_OAUTH_ALLOWED_EMAILS")),
		AllowedDomains: splitCSV(os.Getenv("GOOGLE_OAUTH_ALLOWED_DOMAINS")),
		SessionSecret:  secret,
		SessionTTL:     8 * time.Hour,
		CookieSecure:   true,
		AllowlistFile:  allowlistFile,
	}
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, strings.ToLower(t))
		}
	}
	return out
}

func randString(n int) (string, error) {
	b, err := randBytes(n)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func randBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return nil, fmt.Errorf("rand.Read: %w", err)
	}
	return b, nil
}
