package evilginx

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewClient(t *testing.T) {
	c := NewClient("/opt/evilginx2", "/opt/evilginx2/phishlets")
	if c.BinaryPath() != "/opt/evilginx2/dist/evilginx" {
		t.Errorf("BinaryPath = %q", c.BinaryPath())
	}
	if c.BBoltDBPath() != "/opt/evilginx2/data.db" {
		t.Errorf("BBoltDBPath = %q", c.BBoltDBPath())
	}
}

func TestIsInstalled_False(t *testing.T) {
	c := NewClient("/nonexistent", "/nonexistent")
	if c.IsInstalled() {
		t.Error("expected IsInstalled=false for nonexistent path")
	}
}

func TestIsInstalled_True(t *testing.T) {
	dir := t.TempDir()
	distDir := filepath.Join(dir, "dist")
	os.MkdirAll(distDir, 0755)
	os.WriteFile(filepath.Join(distDir, "evilginx"), []byte("binary"), 0755)

	c := NewClient(dir, dir)
	if !c.IsInstalled() {
		t.Error("expected IsInstalled=true")
	}
}

func TestListPhishlets(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"o365.yaml", "google.yaml", "aws.yaml"} {
		os.WriteFile(filepath.Join(dir, name), []byte("test"), 0644)
	}
	// Non-yaml files should be ignored
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("readme"), 0644)

	c := NewClient("", dir)
	phishlets, err := c.ListPhishlets()
	if err != nil {
		t.Fatal(err)
	}
	if len(phishlets) != 3 {
		t.Errorf("expected 3 phishlets, got %d: %v", len(phishlets), phishlets)
	}

	expected := map[string]bool{"o365": true, "google": true, "aws": true}
	for _, p := range phishlets {
		if !expected[p] {
			t.Errorf("unexpected phishlet: %q", p)
		}
	}
}

func TestListPhishlets_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	c := NewClient("", dir)
	phishlets, err := c.ListPhishlets()
	if err != nil {
		t.Fatal(err)
	}
	if len(phishlets) != 0 {
		t.Errorf("expected 0 phishlets, got %d", len(phishlets))
	}
}

func TestListPhishlets_NonexistentDir(t *testing.T) {
	c := NewClient("", "/nonexistent/phishlets")
	_, err := c.ListPhishlets()
	if err == nil {
		t.Error("expected error for nonexistent dir")
	}
}

func TestServiceStatus_Package(t *testing.T) {
	// Should not error even for unknown services
	status, err := ServiceStatus("nonexistent-service-xyz")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	// Status will be "inactive" or empty on systems without this service
	if status == "active" {
		t.Error("nonexistent service should not be active")
	}
}
