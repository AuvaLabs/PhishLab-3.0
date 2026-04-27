package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// DB wraps the SQLite database connection
type DB struct {
	conn *sql.DB
}

func Open(dbPath string) (*DB, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, fmt.Errorf("creating db directory %s: %w", dir, err)
	}

	conn, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("opening sqlite db: %w", err)
	}

	db := &DB{conn: conn}
	if err := db.migrate(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("running migrations: %w", err)
	}

	return db, nil
}

func (db *DB) Close() error {
	return db.conn.Close()
}

func (db *DB) migrate() error {
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS engagements (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			client TEXT NOT NULL DEFAULT '',
			operator TEXT NOT NULL DEFAULT '',
			start_date TEXT NOT NULL DEFAULT '',
			end_date TEXT NOT NULL DEFAULT '',
			domain TEXT NOT NULL DEFAULT '',
			phishlet_name TEXT NOT NULL DEFAULT '',
			roe_reference TEXT NOT NULL DEFAULT '',
			notes TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'active',
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS captured_credentials (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			engagement_id TEXT NOT NULL DEFAULT '',
			session_id TEXT NOT NULL UNIQUE,
			phishlet TEXT NOT NULL DEFAULT '',
			username TEXT NOT NULL DEFAULT '',
			password TEXT NOT NULL DEFAULT '',
			tokens_json TEXT NOT NULL DEFAULT '{}',
			user_agent TEXT NOT NULL DEFAULT '',
			remote_addr TEXT NOT NULL DEFAULT '',
			captured_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (engagement_id) REFERENCES engagements(id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_creds_engagement ON captured_credentials(engagement_id)`,
		`CREATE INDEX IF NOT EXISTS idx_creds_session ON captured_credentials(session_id)`,
		`CREATE TABLE IF NOT EXISTS campaigns (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			engagement_id TEXT NOT NULL DEFAULT '',
			gophish_id INTEGER NOT NULL DEFAULT 0,
			name TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'created',
			target_count INTEGER NOT NULL DEFAULT 0,
			phish_url TEXT NOT NULL DEFAULT '',
			template_name TEXT NOT NULL DEFAULT '',
			launched_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (engagement_id) REFERENCES engagements(id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_campaigns_engagement ON campaigns(engagement_id)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_campaigns_gophish ON campaigns(gophish_id)`,
		`CREATE TABLE IF NOT EXISTS timeline_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			engagement_id TEXT NOT NULL DEFAULT '',
			campaign_id INTEGER NOT NULL DEFAULT 0,
			source TEXT NOT NULL DEFAULT '',
			event_type TEXT NOT NULL DEFAULT '',
			email TEXT NOT NULL DEFAULT '',
			detail TEXT NOT NULL DEFAULT '',
			rid TEXT NOT NULL DEFAULT '',
			remote_addr TEXT NOT NULL DEFAULT '',
			timestamp DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (engagement_id) REFERENCES engagements(id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_timeline_engagement ON timeline_events(engagement_id)`,
		`CREATE INDEX IF NOT EXISTS idx_timeline_campaign ON timeline_events(campaign_id)`,
		`CREATE INDEX IF NOT EXISTS idx_timeline_rid ON timeline_events(rid)`,
	}

	for _, m := range migrations {
		if _, err := db.conn.Exec(m); err != nil {
			return fmt.Errorf("executing migration: %w", err)
		}
	}
	return nil
}

// UpsertEngagement creates or updates an engagement record
func (db *DB) UpsertEngagement(e Engagement) error {
	now := time.Now()
	_, err := db.conn.Exec(`
		INSERT INTO engagements (id, name, client, operator, start_date, end_date, domain, phishlet_name, roe_reference, notes, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name=excluded.name, client=excluded.client, operator=excluded.operator,
			start_date=excluded.start_date, end_date=excluded.end_date,
			domain=excluded.domain, phishlet_name=excluded.phishlet_name,
			roe_reference=excluded.roe_reference, notes=excluded.notes,
			status=excluded.status, updated_at=?`,
		e.ID, e.Name, e.Client, e.Operator, e.StartDate, e.EndDate,
		e.Domain, e.PhishletName, e.RoEReference, e.Notes, e.Status,
		now, now, now,
	)
	return err
}

// GetEngagement returns an engagement by ID
func (db *DB) GetEngagement(id string) (*Engagement, error) {
	row := db.conn.QueryRow(`SELECT id, name, client, operator, start_date, end_date, domain, phishlet_name, roe_reference, notes, status, created_at, updated_at FROM engagements WHERE id = ?`, id)

	var e Engagement
	err := row.Scan(&e.ID, &e.Name, &e.Client, &e.Operator, &e.StartDate, &e.EndDate, &e.Domain, &e.PhishletName, &e.RoEReference, &e.Notes, &e.Status, &e.CreatedAt, &e.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// GetActiveEngagement returns the first active engagement
func (db *DB) GetActiveEngagement() (*Engagement, error) {
	row := db.conn.QueryRow(`SELECT id, name, client, operator, start_date, end_date, domain, phishlet_name, roe_reference, notes, status, created_at, updated_at FROM engagements WHERE status = 'active' ORDER BY created_at DESC LIMIT 1`)

	var e Engagement
	err := row.Scan(&e.ID, &e.Name, &e.Client, &e.Operator, &e.StartDate, &e.EndDate, &e.Domain, &e.PhishletName, &e.RoEReference, &e.Notes, &e.Status, &e.CreatedAt, &e.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// InsertCredential stores a captured credential, upserting on
// session_id so subsequent updates from the poller (e.g. cookies
// captured after a Vulnerable->Exploitable transition) replace the
// earlier row. created_at is preserved on update; captured_at and
// the cred fields move to the latest values.
func (db *DB) InsertCredential(c CapturedCredential) error {
	_, err := db.conn.Exec(`
		INSERT INTO captured_credentials (engagement_id, session_id, phishlet, username, password, tokens_json, user_agent, remote_addr, captured_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(session_id) DO UPDATE SET
			engagement_id = excluded.engagement_id,
			phishlet      = excluded.phishlet,
			username      = excluded.username,
			password      = excluded.password,
			tokens_json   = excluded.tokens_json,
			user_agent    = excluded.user_agent,
			remote_addr   = excluded.remote_addr,
			captured_at   = excluded.captured_at`,
		c.EngagementID, c.SessionID, c.Phishlet, c.Username, c.Password,
		c.TokensJSON, c.UserAgent, c.RemoteAddr, c.CapturedAt, time.Now(),
	)
	return err
}

// ClearEngagementData deletes all captured credentials and timeline
// events tied to the given engagement id. The engagement record
// itself is preserved. Returns (credsDeleted, eventsDeleted, error).
func (db *DB) ClearEngagementData(engagementID string) (int64, int64, error) {
	if engagementID == "" {
		return 0, 0, fmt.Errorf("engagement id required")
	}
	tx, err := db.conn.Begin()
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback()
	credsRes, err := tx.Exec(`DELETE FROM captured_credentials WHERE engagement_id = ?`, engagementID)
	if err != nil {
		return 0, 0, fmt.Errorf("delete credentials: %w", err)
	}
	credsN, _ := credsRes.RowsAffected()
	eventsRes, err := tx.Exec(`DELETE FROM timeline_events WHERE engagement_id = ?`, engagementID)
	if err != nil {
		return 0, 0, fmt.Errorf("delete timeline: %w", err)
	}
	eventsN, _ := eventsRes.RowsAffected()
	if err := tx.Commit(); err != nil {
		return 0, 0, err
	}
	return credsN, eventsN, nil
}

// GetCredentials returns all credentials for an engagement
func (db *DB) GetCredentials(engagementID string) ([]CapturedCredential, error) {
	rows, err := db.conn.Query(`
		SELECT id, engagement_id, session_id, phishlet, username, password, tokens_json, user_agent, remote_addr, captured_at, created_at
		FROM captured_credentials WHERE engagement_id = ? ORDER BY captured_at DESC`, engagementID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var creds []CapturedCredential
	for rows.Next() {
		var c CapturedCredential
		if err := rows.Scan(&c.ID, &c.EngagementID, &c.SessionID, &c.Phishlet, &c.Username, &c.Password, &c.TokensJSON, &c.UserAgent, &c.RemoteAddr, &c.CapturedAt, &c.CreatedAt); err != nil {
			return nil, err
		}
		creds = append(creds, c)
	}
	return creds, rows.Err()
}

// GetAllCredentials returns all credentials across all engagements
func (db *DB) GetAllCredentials() ([]CapturedCredential, error) {
	rows, err := db.conn.Query(`
		SELECT id, engagement_id, session_id, phishlet, username, password, tokens_json, user_agent, remote_addr, captured_at, created_at
		FROM captured_credentials ORDER BY captured_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var creds []CapturedCredential
	for rows.Next() {
		var c CapturedCredential
		if err := rows.Scan(&c.ID, &c.EngagementID, &c.SessionID, &c.Phishlet, &c.Username, &c.Password, &c.TokensJSON, &c.UserAgent, &c.RemoteAddr, &c.CapturedAt, &c.CreatedAt); err != nil {
			return nil, err
		}
		creds = append(creds, c)
	}
	return creds, rows.Err()
}

// CredentialCount returns the count of credentials for an engagement
func (db *DB) CredentialCount(engagementID string) (int, error) {
	var count int
	err := db.conn.QueryRow(`SELECT COUNT(*) FROM captured_credentials WHERE engagement_id = ?`, engagementID).Scan(&count)
	return count, err
}

// InsertCampaign stores a campaign record, returns the new ID
func (db *DB) InsertCampaign(c CampaignRecord) (int64, error) {
	now := time.Now()
	result, err := db.conn.Exec(`
		INSERT INTO campaigns (engagement_id, gophish_id, name, status, target_count, phish_url, template_name, launched_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.EngagementID, c.GophishID, c.Name, c.Status, c.TargetCount,
		c.PhishURL, c.TemplateName, c.LaunchedAt, now, now,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// UpdateCampaignStatus updates the status of a campaign
func (db *DB) UpdateCampaignStatus(id int64, status string) error {
	_, err := db.conn.Exec(`UPDATE campaigns SET status = ?, updated_at = ? WHERE id = ?`, status, time.Now(), id)
	return err
}

// GetCampaigns returns all campaigns for an engagement
func (db *DB) GetCampaigns(engagementID string) ([]CampaignRecord, error) {
	rows, err := db.conn.Query(`
		SELECT id, engagement_id, gophish_id, name, status, target_count, phish_url, template_name, launched_at, created_at, updated_at
		FROM campaigns WHERE engagement_id = ? ORDER BY created_at DESC`, engagementID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var campaigns []CampaignRecord
	for rows.Next() {
		var c CampaignRecord
		if err := rows.Scan(&c.ID, &c.EngagementID, &c.GophishID, &c.Name, &c.Status, &c.TargetCount, &c.PhishURL, &c.TemplateName, &c.LaunchedAt, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		campaigns = append(campaigns, c)
	}
	return campaigns, rows.Err()
}

// GetCampaignByGophishID finds a campaign by its Gophish ID
func (db *DB) GetCampaignByGophishID(gophishID int64) (*CampaignRecord, error) {
	row := db.conn.QueryRow(`
		SELECT id, engagement_id, gophish_id, name, status, target_count, phish_url, template_name, launched_at, created_at, updated_at
		FROM campaigns WHERE gophish_id = ?`, gophishID)

	var c CampaignRecord
	err := row.Scan(&c.ID, &c.EngagementID, &c.GophishID, &c.Name, &c.Status, &c.TargetCount, &c.PhishURL, &c.TemplateName, &c.LaunchedAt, &c.CreatedAt, &c.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// InsertTimelineEvent stores a timeline event
func (db *DB) InsertTimelineEvent(e TimelineEvent) error {
	_, err := db.conn.Exec(`
		INSERT INTO timeline_events (engagement_id, campaign_id, source, event_type, email, detail, rid, remote_addr, timestamp, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.EngagementID, e.CampaignID, e.Source, e.EventType, e.Email,
		e.Detail, e.RID, e.RemoteAddr, e.Timestamp, time.Now(),
	)
	return err
}

// GetTimeline returns timeline events for an engagement, most recent first
func (db *DB) GetTimeline(engagementID string, limit int) ([]TimelineEvent, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := db.conn.Query(`
		SELECT id, engagement_id, campaign_id, source, event_type, email, detail, rid, remote_addr, timestamp, created_at
		FROM timeline_events WHERE engagement_id = ? ORDER BY timestamp DESC LIMIT ?`, engagementID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []TimelineEvent
	for rows.Next() {
		var e TimelineEvent
		if err := rows.Scan(&e.ID, &e.EngagementID, &e.CampaignID, &e.Source, &e.EventType, &e.Email, &e.Detail, &e.RID, &e.RemoteAddr, &e.Timestamp, &e.CreatedAt); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

// GetTimelineByCampaign returns timeline events for a specific campaign
func (db *DB) GetTimelineByCampaign(campaignID int64, limit int) ([]TimelineEvent, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := db.conn.Query(`
		SELECT id, engagement_id, campaign_id, source, event_type, email, detail, rid, remote_addr, timestamp, created_at
		FROM timeline_events WHERE campaign_id = ? ORDER BY timestamp DESC LIMIT ?`, campaignID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []TimelineEvent
	for rows.Next() {
		var e TimelineEvent
		if err := rows.Scan(&e.ID, &e.EngagementID, &e.CampaignID, &e.Source, &e.EventType, &e.Email, &e.Detail, &e.RID, &e.RemoteAddr, &e.Timestamp, &e.CreatedAt); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}
