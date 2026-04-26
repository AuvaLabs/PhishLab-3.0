package store

import "time"

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

// CapturedCredential represents a credential captured from Evilginx
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
	Source       string    `json:"source"`  // gophish, evilginx
	EventType    string    `json:"event_type"` // email_sent, email_opened, link_clicked, submitted_data, credential_captured
	Email        string    `json:"email,omitempty"`
	Detail       string    `json:"detail,omitempty"`
	RID          string    `json:"rid,omitempty"`
	RemoteAddr   string    `json:"remote_addr,omitempty"`
	Timestamp    time.Time `json:"timestamp"`
	CreatedAt    time.Time `json:"created_at"`
}
