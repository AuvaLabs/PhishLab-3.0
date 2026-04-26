package gophish

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPing_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/campaigns/" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer testkey" {
			t.Error("missing or wrong auth header")
		}
		json.NewEncoder(w).Encode([]Campaign{})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "testkey")
	if err := client.Ping(); err != nil {
		t.Errorf("Ping: %v", err)
	}
}

func TestPing_ServerDown(t *testing.T) {
	client := NewClient("http://127.0.0.1:1", "key")
	if err := client.Ping(); err == nil {
		t.Error("expected error for unreachable server")
	}
}

func TestGetCampaigns(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		campaigns := []Campaign{
			{ID: 1, Name: "Test Campaign", Status: "In progress"},
		}
		json.NewEncoder(w).Encode(campaigns)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "key")
	campaigns, err := client.GetCampaigns()
	if err != nil {
		t.Fatal(err)
	}
	if len(campaigns) != 1 {
		t.Fatalf("expected 1 campaign, got %d", len(campaigns))
	}
	if campaigns[0].Name != "Test Campaign" {
		t.Errorf("name = %q, want 'Test Campaign'", campaigns[0].Name)
	}
}

func TestCreateSendingProfile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		var sp SendingProfile
		json.NewDecoder(r.Body).Decode(&sp)

		sp.ID = 42
		json.NewEncoder(w).Encode(sp)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "key")
	sp, err := client.CreateSendingProfile(SendingProfile{
		Name:        "Test SMTP",
		Host:        "localhost:1025",
		FromAddress: "test@test.com",
	})
	if err != nil {
		t.Fatal(err)
	}
	if sp.ID != 42 {
		t.Errorf("ID = %d, want 42", sp.ID)
	}
}

func TestCreateGroup(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var g Group
		json.NewDecoder(r.Body).Decode(&g)
		g.ID = 1
		json.NewEncoder(w).Encode(g)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "key")
	g, err := client.CreateGroup(Group{
		Name: "Targets",
		Targets: []Target{
			{Email: "user@test.com", FirstName: "Test", LastName: "User"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if g.ID != 1 {
		t.Errorf("group ID = %d, want 1", g.ID)
	}
}

func TestAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"message":"Invalid API key"}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "badkey")
	_, err := client.GetCampaigns()
	if err == nil {
		t.Error("expected error for 401 response")
	}
}
