package evilginx

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Client manages Evilginx process and configuration
type Client struct {
	installDir  string
	phishletsDir string
	binaryPath  string
}

func NewClient(installDir, phishletsDir string) *Client {
	return &Client{
		installDir:  installDir,
		phishletsDir: phishletsDir,
		binaryPath:  filepath.Join(installDir, "dist", "evilginx"),
	}
}

// IsInstalled checks if the Evilginx binary exists
func (c *Client) IsInstalled() bool {
	_, err := os.Stat(c.binaryPath)
	return err == nil
}

// BinaryPath returns the path to the Evilginx binary
func (c *Client) BinaryPath() string {
	return c.binaryPath
}

// BBoltDBPath returns the expected path to the Evilginx bbolt database
func (c *Client) BBoltDBPath() string {
	return filepath.Join(c.installDir, "data.db")
}

// ListPhishlets returns the available phishlet names from the phishlets directory
func (c *Client) ListPhishlets() ([]string, error) {
	entries, err := os.ReadDir(c.phishletsDir)
	if err != nil {
		return nil, fmt.Errorf("reading phishlets dir %s: %w", c.phishletsDir, err)
	}

	var phishlets []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".yaml") {
			name := strings.TrimSuffix(e.Name(), ".yaml")
			phishlets = append(phishlets, name)
		}
	}
	return phishlets, nil
}

// ServiceStatus checks the systemd service status for evilginx
func (c *Client) ServiceStatus() (string, error) {
	return systemdStatusCheck("evilginx")
}

// RestartService restarts the evilginx systemd service
func (c *Client) RestartService() error {
	return systemdAction("restart", "evilginx")
}

// ServiceStatus checks systemd status for any service by name
func ServiceStatus(service string) (string, error) {
	return systemdStatusCheck(service)
}

// RestartService restarts any systemd service by name
func RestartService(service string) error {
	return systemdAction("restart", service)
}

func systemdStatusCheck(service string) (string, error) {
	out, err := exec.Command("systemctl", "is-active", service).Output()
	status := strings.TrimSpace(string(out))
	if err != nil {
		return status, nil // "inactive" or "failed" is not an exec error we care about
	}
	return status, nil
}

func systemdAction(action, service string) error {
	cmd := exec.Command("systemctl", action, service)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
