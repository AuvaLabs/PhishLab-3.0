package campaign

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/CyberOneHQ/evilginx-lab/internal/config"
	"github.com/CyberOneHQ/evilginx-lab/internal/gophish"
	"github.com/CyberOneHQ/evilginx-lab/internal/store"
)

func setupTestDB(t *testing.T) *store.DB {
	t.Helper()
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func setupMockGophish(t *testing.T) (*httptest.Server, *gophish.Client) {
	t.Helper()
	groupID := int64(1)
	templateID := int64(1)
	pageID := int64(1)
	campaignID := int64(1)

	mux := http.NewServeMux()

	mux.HandleFunc("/api/groups/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			var g gophish.Group
			json.NewDecoder(r.Body).Decode(&g)
			g.ID = groupID
			groupID++
			json.NewEncoder(w).Encode(g)
			return
		}
		json.NewEncoder(w).Encode([]gophish.Group{})
	})

	mux.HandleFunc("/api/templates/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			var t gophish.Template
			json.NewDecoder(r.Body).Decode(&t)
			t.ID = templateID
			templateID++
			json.NewEncoder(w).Encode(t)
			return
		}
		json.NewEncoder(w).Encode([]gophish.Template{})
	})

	mux.HandleFunc("/api/pages/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			var p gophish.Page
			json.NewDecoder(r.Body).Decode(&p)
			p.ID = pageID
			pageID++
			json.NewEncoder(w).Encode(p)
			return
		}
		json.NewEncoder(w).Encode([]gophish.Page{})
	})

	mux.HandleFunc("/api/smtp/", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]gophish.SendingProfile{
			{ID: 1, Name: "Test Profile", Host: "localhost:1025"},
		})
	})

	mux.HandleFunc("/api/campaigns/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			var c gophish.Campaign
			json.NewDecoder(r.Body).Decode(&c)
			c.ID = campaignID
			c.Status = "Queued"
			campaignID++
			json.NewEncoder(w).Encode(c)
			return
		}
		json.NewEncoder(w).Encode([]gophish.Campaign{})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	client := gophish.NewClient(srv.URL, "test-api-key")
	return srv, client
}

func testConfig() config.EngagementConfig {
	return config.EngagementConfig{
		Engagement: config.EngagementInfo{
			Name: "Test", ID: "ENG-001",
		},
		Domain: config.DomainConfig{
			Phishing:    "phish.test.com",
			RedirectURL: "https://real.test.com",
		},
		Phishlet: config.PhishletConfig{Name: "o365"},
		Targets: []config.Target{
			{Email: "user1@target.com", FirstName: "Alice", LastName: "Smith", Position: "Analyst"},
			{Email: "user2@target.com", FirstName: "Bob", LastName: "Jones", Position: "Manager"},
		},
	}
}

func TestLaunch_Success(t *testing.T) {
	db := setupTestDB(t)
	_, gpClient := setupMockGophish(t)

	db.UpsertEngagement(store.Engagement{ID: "ENG-001", Name: "Test", Status: "active"})

	var receivedEvents []store.TimelineEvent
	o := NewOrchestrator(db, gpClient, testConfig(), func(evt store.TimelineEvent) {
		receivedEvents = append(receivedEvents, evt)
	})

	req := LaunchRequest{
		Name: "Test Campaign",
	}
	req.Template.Name = "Phish Template"
	req.Template.Subject = "Urgent: Verify Account"
	req.Template.HTML = "<p>Click {{.URL}}</p>"
	req.LandingPage.Name = "Login Page"
	req.LandingPage.HTML = "<form>login</form>"

	result, err := o.Launch(req)
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}

	if result.GophishID == 0 {
		t.Error("expected non-zero Gophish ID")
	}
	if result.Status != "Queued" {
		t.Errorf("status = %q, want 'Queued'", result.Status)
	}
	if result.PhishURL != "https://phish.test.com" {
		t.Errorf("phish URL = %q, want 'https://phish.test.com'", result.PhishURL)
	}

	// Verify local campaign record
	campaigns, err := db.GetCampaigns("ENG-001")
	if err != nil {
		t.Fatal(err)
	}
	if len(campaigns) != 1 {
		t.Fatalf("expected 1 campaign, got %d", len(campaigns))
	}
	if campaigns[0].Name != "Test Campaign" {
		t.Errorf("campaign name = %q", campaigns[0].Name)
	}
	if campaigns[0].TargetCount != 2 {
		t.Errorf("target count = %d, want 2", campaigns[0].TargetCount)
	}

	// Verify timeline event
	if len(receivedEvents) != 1 {
		t.Fatalf("expected 1 event callback, got %d", len(receivedEvents))
	}
	if receivedEvents[0].EventType != "campaign_launched" {
		t.Errorf("event type = %q", receivedEvents[0].EventType)
	}
}

func TestLaunch_NoEngagement(t *testing.T) {
	db := setupTestDB(t)
	_, gpClient := setupMockGophish(t)
	o := NewOrchestrator(db, gpClient, testConfig(), nil)

	req := LaunchRequest{Name: "Test"}
	req.Template.Name = "tmpl"
	req.LandingPage.Name = "page"

	_, err := o.Launch(req)
	if err == nil {
		t.Fatal("expected error for no active engagement")
	}
}

func TestLaunch_NoTargets(t *testing.T) {
	db := setupTestDB(t)
	_, gpClient := setupMockGophish(t)
	db.UpsertEngagement(store.Engagement{ID: "ENG-001", Name: "Test", Status: "active"})

	cfg := testConfig()
	cfg.Targets = nil
	o := NewOrchestrator(db, gpClient, cfg, nil)

	req := LaunchRequest{Name: "Test"}
	req.Template.Name = "tmpl"
	req.LandingPage.Name = "page"

	_, err := o.Launch(req)
	if err == nil {
		t.Fatal("expected error for no targets")
	}
}

func TestLaunch_NoGophish(t *testing.T) {
	db := setupTestDB(t)
	db.UpsertEngagement(store.Engagement{ID: "ENG-001", Name: "Test", Status: "active"})

	o := NewOrchestrator(db, nil, testConfig(), nil)
	req := LaunchRequest{Name: "Test"}
	req.Template.Name = "tmpl"

	_, err := o.Launch(req)
	if err == nil {
		t.Fatal("expected error for nil gophish client")
	}
}

func TestSyncCampaignEvents(t *testing.T) {
	db := setupTestDB(t)
	db.UpsertEngagement(store.Engagement{ID: "ENG-001", Name: "Test", Status: "active"})

	// Mock Gophish with a campaign that has timeline events
	mux := http.NewServeMux()
	mux.HandleFunc("/api/campaigns/1", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(gophish.Campaign{
			ID:     1,
			Status: "In progress",
			Timeline: []gophish.Event{
				{Email: "user1@test.com", Time: time.Now().Format(time.RFC3339), Message: "Email Sent"},
				{Email: "user1@test.com", Time: time.Now().Format(time.RFC3339), Message: "Email Opened"},
			},
			Results: []gophish.Result{
				{Email: "user1@test.com", Status: "Email Opened", IP: "1.2.3.4"},
			},
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	gpClient := gophish.NewClient(srv.URL, "test-key")

	db.InsertCampaign(store.CampaignRecord{
		EngagementID: "ENG-001",
		GophishID:    1,
		Name:         "Test",
		Status:       "launched",
		LaunchedAt:   time.Now(),
	})

	o := NewOrchestrator(db, gpClient, testConfig(), nil)

	if err := o.SyncCampaignEvents("ENG-001"); err != nil {
		t.Fatalf("SyncCampaignEvents: %v", err)
	}

	// Check that timeline events were created
	events, err := db.GetTimeline("ENG-001", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) < 2 {
		t.Errorf("expected at least 2 timeline events, got %d", len(events))
	}

	// Check campaign status was updated
	camp, _ := db.GetCampaignByGophishID(1)
	if camp != nil && camp.Status != "In progress" {
		t.Errorf("campaign status = %q, want 'In progress'", camp.Status)
	}
}

func TestExtractRIDFromURL(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"https://phish.com/?rid=abc123", "abc123"},
		{"https://phish.com/", ""},
		{"https://phish.com/?rid=test&other=1", "test"},
		{"not-a-url", ""},
	}
	for _, tt := range tests {
		got := ExtractRIDFromURL(tt.url)
		if got != tt.want {
			t.Errorf("ExtractRIDFromURL(%q) = %q, want %q", tt.url, got, tt.want)
		}
	}
}

func TestNormalizeGophishEvent(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Email Sent", "email_sent"},
		{"Email Opened", "email_opened"},
		{"Clicked Link", "link_clicked"},
		{"Submitted Data", "submitted_data"},
		{"Reported", "email_reported"},
		{"Unknown Event", "unknown"},
	}
	for _, tt := range tests {
		got := normalizeGophishEvent(tt.input)
		if got != tt.want {
			t.Errorf("normalizeGophishEvent(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestBuildPhishURL(t *testing.T) {
	tests := []struct {
		domain string
		want   string
	}{
		{"phish.test.com", "https://phish.test.com"},
		{"https://phish.test.com", "https://phish.test.com"},
		{"http://phish.test.com", "http://phish.test.com"},
	}
	for _, tt := range tests {
		got := buildPhishURL(tt.domain)
		if got != tt.want {
			t.Errorf("buildPhishURL(%q) = %q, want %q", tt.domain, got, tt.want)
		}
	}
}

func TestParseGophishTime(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	rfc := now.Format(time.RFC3339)

	got := parseGophishTime(rfc)
	if !got.Equal(now) {
		t.Errorf("parseGophishTime(%q) = %v, want %v", rfc, got, now)
	}

	// Invalid should return ~now
	got = parseGophishTime("invalid")
	if time.Since(got) > 5*time.Second {
		t.Error("expected fallback to ~now for invalid time")
	}
}

func TestSyncCampaignEvents_NoCampaigns(t *testing.T) {
	db := setupTestDB(t)
	db.UpsertEngagement(store.Engagement{ID: "ENG-001", Name: "Test", Status: "active"})

	o := NewOrchestrator(db, nil, testConfig(), nil)
	if err := o.SyncCampaignEvents("ENG-001"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSyncCampaignEvents_SkipsZeroGophishID(t *testing.T) {
	db := setupTestDB(t)
	db.UpsertEngagement(store.Engagement{ID: "ENG-001", Name: "Test", Status: "active"})

	// Campaign with gophish_id=0 should be skipped
	db.InsertCampaign(store.CampaignRecord{
		EngagementID: "ENG-001",
		GophishID:    0,
		Name:         "Manual",
		Status:       "created",
		LaunchedAt:   time.Now(),
	})

	_, gpClient := setupMockGophish(t)
	o := NewOrchestrator(db, gpClient, testConfig(), nil)

	if err := o.SyncCampaignEvents("ENG-001"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	events, _ := db.GetTimeline("ENG-001", 100)
	if len(events) != 0 {
		t.Errorf("expected 0 events, got %d", len(events))
	}
}

// Suppress unused import warning
var _ = fmt.Sprintf
