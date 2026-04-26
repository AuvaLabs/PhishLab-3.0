package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/CyberOneHQ/evilginx-lab/internal/evilginx"
	"github.com/CyberOneHQ/evilginx-lab/internal/gophish"
	"github.com/CyberOneHQ/evilginx-lab/internal/store"
)

func setupTestHandler(t *testing.T) (*APIHandler, *store.DB) {
	t.Helper()
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	phishletsDir := t.TempDir()
	egClient := evilginx.NewClient(dir, phishletsDir)
	handler := NewAPIHandler(db, egClient, nil)
	return handler, db
}

func TestHandleDashboard_NoEngagement(t *testing.T) {
	handler, _ := setupTestHandler(t)

	req := httptest.NewRequest("GET", "/api/dashboard", nil)
	w := httptest.NewRecorder()
	handler.HandleDashboard(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}

	var summary store.DashboardSummary
	if err := json.NewDecoder(w.Body).Decode(&summary); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if summary.Engagement != nil {
		t.Error("expected nil engagement")
	}
	if len(summary.Services) != 3 {
		t.Errorf("expected 3 services, got %d", len(summary.Services))
	}
	if len(summary.Credentials) != 0 {
		t.Errorf("expected 0 credentials, got %d", len(summary.Credentials))
	}
	if len(summary.Campaigns) != 0 {
		t.Errorf("expected 0 campaigns, got %d", len(summary.Campaigns))
	}
	if len(summary.Timeline) != 0 {
		t.Errorf("expected 0 timeline events, got %d", len(summary.Timeline))
	}
}

func TestHandleDashboard_WithEngagement(t *testing.T) {
	handler, db := setupTestHandler(t)

	db.UpsertEngagement(store.Engagement{
		ID:     "ENG-001",
		Name:   "Test",
		Domain: "test.com",
		Status: "active",
	})

	req := httptest.NewRequest("GET", "/api/dashboard", nil)
	w := httptest.NewRecorder()
	handler.HandleDashboard(w, req)

	var summary store.DashboardSummary
	json.NewDecoder(w.Body).Decode(&summary)

	if summary.Engagement == nil {
		t.Fatal("expected non-nil engagement")
	}
	if summary.Engagement.Name != "Test" {
		t.Errorf("name = %q, want 'Test'", summary.Engagement.Name)
	}
}

func TestHandleDashboard_WithCampaignsAndTimeline(t *testing.T) {
	handler, db := setupTestHandler(t)

	db.UpsertEngagement(store.Engagement{ID: "ENG-001", Name: "Test", Status: "active"})
	db.InsertCampaign(store.CampaignRecord{
		EngagementID: "ENG-001", GophishID: 1, Name: "Camp1", Status: "launched", LaunchedAt: time.Now(),
	})
	db.InsertTimelineEvent(store.TimelineEvent{
		EngagementID: "ENG-001", Source: "gophish", EventType: "email_sent", Timestamp: time.Now(),
	})

	req := httptest.NewRequest("GET", "/api/dashboard", nil)
	w := httptest.NewRecorder()
	handler.HandleDashboard(w, req)

	var summary store.DashboardSummary
	json.NewDecoder(w.Body).Decode(&summary)

	if len(summary.Campaigns) != 1 {
		t.Errorf("expected 1 campaign, got %d", len(summary.Campaigns))
	}
	if len(summary.Timeline) != 1 {
		t.Errorf("expected 1 timeline event, got %d", len(summary.Timeline))
	}
}

func TestHandleCredentials_Empty(t *testing.T) {
	handler, _ := setupTestHandler(t)

	req := httptest.NewRequest("GET", "/api/credentials", nil)
	w := httptest.NewRecorder()
	handler.HandleCredentials(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}

	var creds []store.CapturedCredential
	json.NewDecoder(w.Body).Decode(&creds)

	if len(creds) != 0 {
		t.Errorf("expected 0 credentials, got %d", len(creds))
	}
}

func TestHandleServices(t *testing.T) {
	handler, _ := setupTestHandler(t)

	req := httptest.NewRequest("GET", "/api/services", nil)
	w := httptest.NewRecorder()
	handler.HandleServices(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}

	var services []store.ServiceHealth
	json.NewDecoder(w.Body).Decode(&services)

	if len(services) != 3 {
		t.Errorf("expected 3 services, got %d", len(services))
	}

	names := map[string]bool{}
	for _, s := range services {
		names[s.Name] = true
	}
	for _, expected := range []string{"evilginx", "gophish", "mailhog"} {
		if !names[expected] {
			t.Errorf("missing service %q", expected)
		}
	}
}

func TestHandlePhishlets_EmptyDir(t *testing.T) {
	handler, _ := setupTestHandler(t)

	req := httptest.NewRequest("GET", "/api/phishlets", nil)
	w := httptest.NewRecorder()
	handler.HandlePhishlets(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestHandleGetCampaigns_Empty(t *testing.T) {
	handler, _ := setupTestHandler(t)

	req := httptest.NewRequest("GET", "/api/campaigns", nil)
	w := httptest.NewRecorder()
	handler.HandleGetCampaigns(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}

	var campaigns []store.CampaignRecord
	json.NewDecoder(w.Body).Decode(&campaigns)
	if len(campaigns) != 0 {
		t.Errorf("expected 0 campaigns, got %d", len(campaigns))
	}
}

func TestHandleGetCampaigns_WithData(t *testing.T) {
	handler, db := setupTestHandler(t)
	db.UpsertEngagement(store.Engagement{ID: "ENG-001", Name: "Test", Status: "active"})
	db.InsertCampaign(store.CampaignRecord{
		EngagementID: "ENG-001", GophishID: 1, Name: "C1", Status: "launched", TargetCount: 3, LaunchedAt: time.Now(),
	})

	req := httptest.NewRequest("GET", "/api/campaigns", nil)
	w := httptest.NewRecorder()
	handler.HandleGetCampaigns(w, req)

	var campaigns []store.CampaignRecord
	json.NewDecoder(w.Body).Decode(&campaigns)
	if len(campaigns) != 1 {
		t.Fatalf("expected 1 campaign, got %d", len(campaigns))
	}
	if campaigns[0].Name != "C1" {
		t.Errorf("name = %q", campaigns[0].Name)
	}
}

func TestHandleLaunchCampaign_NoOrchestrator(t *testing.T) {
	handler, _ := setupTestHandler(t)

	req := httptest.NewRequest("POST", "/api/campaigns", nil)
	w := httptest.NewRecorder()
	handler.HandleLaunchCampaign(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", w.Code)
	}
}

func TestHandleGetTimeline_Empty(t *testing.T) {
	handler, _ := setupTestHandler(t)

	req := httptest.NewRequest("GET", "/api/timeline", nil)
	w := httptest.NewRecorder()
	handler.HandleGetTimeline(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}

	var events []store.TimelineEvent
	json.NewDecoder(w.Body).Decode(&events)
	if len(events) != 0 {
		t.Errorf("expected 0 events, got %d", len(events))
	}
}

func TestHandleGetTimeline_WithData(t *testing.T) {
	handler, db := setupTestHandler(t)
	db.UpsertEngagement(store.Engagement{ID: "ENG-001", Name: "Test", Status: "active"})
	db.InsertTimelineEvent(store.TimelineEvent{
		EngagementID: "ENG-001", Source: "evilginx", EventType: "credential_captured",
		Email: "user@test.com", Timestamp: time.Now(),
	})

	req := httptest.NewRequest("GET", "/api/timeline?limit=10", nil)
	w := httptest.NewRecorder()
	handler.HandleGetTimeline(w, req)

	var events []store.TimelineEvent
	json.NewDecoder(w.Body).Decode(&events)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].EventType != "credential_captured" {
		t.Errorf("event type = %q", events[0].EventType)
	}
}

func TestHandleGetTemplates_NoGophish(t *testing.T) {
	handler, _ := setupTestHandler(t)

	req := httptest.NewRequest("GET", "/api/templates", nil)
	w := httptest.NewRecorder()
	handler.HandleGetTemplates(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}

	var templates []gophish.Template
	json.NewDecoder(w.Body).Decode(&templates)
	if len(templates) != 0 {
		t.Errorf("expected 0 templates, got %d", len(templates))
	}
}

func TestHandleSyncCampaigns_NoOrchestrator(t *testing.T) {
	handler, _ := setupTestHandler(t)

	req := httptest.NewRequest("POST", "/api/campaigns/sync", nil)
	w := httptest.NewRecorder()
	handler.HandleSyncCampaigns(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", w.Code)
	}
}
