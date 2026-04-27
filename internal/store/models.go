package store

import (
	"encoding/json"
	"strings"
	"time"
)

// Engagement represents a stored engagement record
type Engagement struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Client       string    `json:"client"`
	Operator     string    `json:"operator"`
	StartDate    string    `json:"start_date"`
	EndDate      string    `json:"end_date"`
	Domain       string    `json:"domain"`
	PhishletName string    `json:"phishlet_name"`
	RoEReference string    `json:"roe_reference"`
	Notes        string    `json:"notes"`
	Status       string    `json:"status"` // active, completed, archived
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// CapturedCredential represents a credential captured from Evilginx.
//
// MarshalJSON enriches the wire representation with computed fields
// (status, recommendations, cookie_count, exploitable) so the dashboard
// can render badges + remediation guidance without a second round trip.
type CapturedCredential struct {
	ID           int64     `json:"id"`
	EngagementID string    `json:"engagement_id"`
	SessionID    string    `json:"session_id"`
	Phishlet     string    `json:"phishlet"`
	Username     string    `json:"username"`
	Password     string    `json:"password"`
	TokensJSON   string    `json:"tokens_json"`
	UserAgent    string    `json:"user_agent"`
	RemoteAddr   string    `json:"remote_addr"`
	CapturedAt   time.Time `json:"captured_at"`
	CreatedAt    time.Time `json:"created_at"`
}

// Status returns "Visited" / "Vulnerable" / "Exploitable" based on
// what was captured. Drives the dashboard status badge.
//
//	Visited     - victim clicked the lure but never authenticated
//	Vulnerable  - victim entered a username/password (cred POST captured)
//	Exploitable - post-auth session cookies (ESTSAUTH/ESTSAUTHPERSISTENT)
//	              were stolen, replayable for full account takeover
//	              without password OR MFA OR passkey
func (c CapturedCredential) Status() string {
	if c.HasReplayableSessionCookie() {
		return "Exploitable"
	}
	if c.Username != "" || c.Password != "" {
		return "Vulnerable"
	}
	return "Visited"
}

// HasReplayableSessionCookie reports whether the captured cookie set
// includes a Microsoft session-grade auth token. ESTSAUTH and
// ESTSAUTHPERSISTENT (set on .login.microsoftonline.com) are
// sufficient on their own for session replay against M365.
func (c CapturedCredential) HasReplayableSessionCookie() bool {
	if c.TokensJSON == "" || c.TokensJSON == "{}" {
		return false
	}
	// Cheap substring match avoids a full JSON unmarshal per row.
	// The cookie names are unique enough that a string contains is
	// a safe heuristic; the dashboard surfaces this as a badge so
	// the cost of a false positive is just an UI hint.
	return strings.Contains(c.TokensJSON, `"ESTSAUTHPERSISTENT"`) ||
		strings.Contains(c.TokensJSON, `"ESTSAUTH"`)
}

// CookieCount counts unique cookies across all proxied domains in
// the captured token jar. Returns 0 if tokens haven't been captured
// or the JSON is malformed.
func (c CapturedCredential) CookieCount() int {
	if c.TokensJSON == "" || c.TokensJSON == "{}" {
		return 0
	}
	var jar map[string]map[string]any
	if err := json.Unmarshal([]byte(c.TokensJSON), &jar); err != nil {
		return 0
	}
	n := 0
	for _, domain := range jar {
		n += len(domain)
	}
	return n
}

// Recommendations returns engagement-report-grade remediation guidance
// for the captured session. Tailored by status; references vendor
// remediation primitives (Entra ID admin actions, Conditional Access,
// token protection) so the operator can paste straight into the
// findings doc.
func (c CapturedCredential) Recommendations() []string {
	switch c.Status() {
	case "Exploitable":
		return []string{
			"Revoke the user's active sign-in sessions immediately (Entra ID admin > Users > [user] > Authentication methods > Revoke sessions, OR PowerShell: Revoke-MgUserSignInSession -UserId <upn>).",
			"Force the user to re-authenticate on next sign-in (the captured ESTSAUTH/ESTSAUTHPERSISTENT cookies are valid until expiry — up to 90 days for the persistent variant).",
			"Investigate whether the captured cookies have already been replayed: review Entra ID sign-in logs for unfamiliar IPs / user agents on the affected account around the capture timestamp.",
			"Enforce token protection (sign-in session token binding) via Conditional Access — this binds session cookies to the originating device/TPM and breaks reverse-proxy phishing replay (E5 license required).",
			"Roll out FIDO2 / passkey authentication tenant-wide. While FIDO2 does not stop session theft after the user authenticates through a proxy, it raises the per-engagement attacker effort and disrupts most adversary-in-the-middle phishing kits.",
			"Add a Conditional Access policy that requires compliant device + non-anonymous IP for session continuation, not just initial sign-in.",
			"Document this incident in the engagement report under \"Exploitable: Session Cookie Theft\" with the captured cookie inventory and confirm scope of access (mailbox read, Teams, OneDrive, SPO).",
		}
	case "Vulnerable":
		return []string{
			"Rotate the user's password immediately if the password was captured (check Password column).",
			"Confirm whether the user proceeded to MFA / passkey after entering credentials — if so, post-auth session cookies may exist on subsequent captures of the same session id.",
			"Enable MFA / passkey on the user's account if not already enforced.",
			"Add a Conditional Access policy that requires MFA for all sign-ins to this tenant.",
			"Run a credential-stuffing check against the captured username + password against other internal systems.",
		}
	default: // Visited
		return []string{
			"User clicked the lure but did not enter credentials. No remediation required for this user.",
			"This still indicates the user is susceptible to lure click-through — recommend phishing awareness training.",
		}
	}
}

// MarshalJSON adds the computed status / recommendations / cookie
// count / exploitable boolean to the wire representation so the
// dashboard does not have to compute them client-side.
func (c CapturedCredential) MarshalJSON() ([]byte, error) {
	type alias CapturedCredential
	return json.Marshal(struct {
		alias
		Status          string   `json:"status"`
		Exploitable     bool     `json:"exploitable"`
		CookieCount     int      `json:"cookie_count"`
		Recommendations []string `json:"recommendations"`
	}{
		alias:           alias(c),
		Status:          c.Status(),
		Exploitable:     c.HasReplayableSessionCookie(),
		CookieCount:     c.CookieCount(),
		Recommendations: c.Recommendations(),
	})
}

// ServiceHealth represents the health status of a managed service
type ServiceHealth struct {
	Name   string `json:"name"`
	Status string `json:"status"` // active, inactive, failed
	Uptime string `json:"uptime,omitempty"`
}

// DashboardSummary is the top-level view returned to the dashboard
type DashboardSummary struct {
	Engagement      *Engagement          `json:"engagement"`
	Services        []ServiceHealth      `json:"services"`
	Phishlets       []PhishletInfo       `json:"phishlets"`
	Credentials     []CapturedCredential `json:"credentials"`
	Campaigns       []CampaignRecord     `json:"campaigns"`
	Timeline        []TimelineEvent      `json:"timeline"`
	CredentialCount int                  `json:"credential_count"`
	CampaignCount   int                  `json:"campaign_count"`
}

// PhishletInfo describes a phishlet's status
type PhishletInfo struct {
	Name     string `json:"name"`
	Hostname string `json:"hostname,omitempty"`
	Enabled  bool   `json:"enabled"`
}

// CampaignRecord tracks a Gophish campaign linked to an engagement
type CampaignRecord struct {
	ID           int64     `json:"id"`
	EngagementID string    `json:"engagement_id"`
	GophishID    int64     `json:"gophish_id"`
	Name         string    `json:"name"`
	Status       string    `json:"status"` // created, launched, completed, error
	TargetCount  int       `json:"target_count"`
	PhishURL     string    `json:"phish_url"`
	TemplateName string    `json:"template_name"`
	LaunchedAt   time.Time `json:"launched_at"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// TimelineEvent is a unified event from Gophish or Evilginx
type TimelineEvent struct {
	ID           int64     `json:"id"`
	EngagementID string    `json:"engagement_id"`
	CampaignID   int64     `json:"campaign_id,omitempty"`
	Source       string    `json:"source"`     // gophish, evilginx
	EventType    string    `json:"event_type"` // email_sent, email_opened, link_clicked, submitted_data, credential_captured
	Email        string    `json:"email,omitempty"`
	Detail       string    `json:"detail,omitempty"`
	RID          string    `json:"rid,omitempty"`
	RemoteAddr   string    `json:"remote_addr,omitempty"`
	Timestamp    time.Time `json:"timestamp"`
	CreatedAt    time.Time `json:"created_at"`
}
