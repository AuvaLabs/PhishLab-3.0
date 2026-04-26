package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	yaml := `
engagement:
  name: "Test Engagement"
  client: "TestCorp"
  id: "TEST-001"
  start_date: "2026-03-01"
  end_date: "2026-03-31"
domain:
  phishing: "login.test.com"
  redirect_url: "https://example.com/"
phishlet:
  name: "o365"
  hostname: "login.test.com"
`
	dir := t.TempDir()
	path := filepath.Join(dir, "evilginx-lab.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if cfg.Engagement.Name != "Test Engagement" {
		t.Errorf("expected name 'Test Engagement', got %q", cfg.Engagement.Name)
	}
	if cfg.Engagement.Client != "TestCorp" {
		t.Errorf("expected client 'TestCorp', got %q", cfg.Engagement.Client)
	}
	if cfg.Domain.Phishing != "login.test.com" {
		t.Errorf("expected domain 'login.test.com', got %q", cfg.Domain.Phishing)
	}
	if cfg.Phishlet.Name != "o365" {
		t.Errorf("expected phishlet 'o365', got %q", cfg.Phishlet.Name)
	}
}

func TestLoadConfig_ValidationErrors(t *testing.T) {
	tests := []struct {
		name string
		yaml string
	}{
		{
			name: "missing engagement name",
			yaml: `
domain:
  phishing: "login.test.com"
phishlet:
  name: "o365"
`,
		},
		{
			name: "missing domain",
			yaml: `
engagement:
  name: "Test"
phishlet:
  name: "o365"
`,
		},
		{
			name: "missing phishlet",
			yaml: `
engagement:
  name: "Test"
domain:
  phishing: "login.test.com"
`,
		},
		{
			name: "invalid start_date",
			yaml: `
engagement:
  name: "Test"
  start_date: "not-a-date"
domain:
  phishing: "login.test.com"
phishlet:
  name: "o365"
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "evilginx-lab.yaml")
			if err := os.WriteFile(path, []byte(tt.yaml), 0644); err != nil {
				t.Fatal(err)
			}

			_, err := LoadConfig(path)
			if err == nil {
				t.Error("expected validation error, got nil")
			}
		})
	}
}

func TestLoadConfig_FileNotFound(t *testing.T) {
	_, err := LoadConfig("/nonexistent/evilginx-lab.yaml")
	if err == nil {
		t.Error("expected error for missing file, got nil")
	}
}

func TestWithDefaults(t *testing.T) {
	cfg := EngagementConfig{
		Engagement: EngagementInfo{Name: "Test"},
		Domain:     DomainConfig{Phishing: "test.com"},
		Phishlet:   PhishletConfig{Name: "o365"},
	}

	result := cfg.WithDefaults()

	if result.Evilginx.InstallDir != "/opt/evilginx2" {
		t.Errorf("expected default install dir, got %q", result.Evilginx.InstallDir)
	}
	if result.Gophish.AdminURL != "http://127.0.0.1:8800" {
		t.Errorf("expected default gophish URL, got %q", result.Gophish.AdminURL)
	}
	if result.Dashboard.Listen != "127.0.0.1:8443" {
		t.Errorf("expected default dashboard listen, got %q", result.Dashboard.Listen)
	}
	if result.Polling.Interval != 5 {
		t.Errorf("expected default polling interval 5, got %d", result.Polling.Interval)
	}
	if result.SMTP.Port != 1025 {
		t.Errorf("expected default SMTP port 1025, got %d", result.SMTP.Port)
	}
}
