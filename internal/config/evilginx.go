package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// EvilginxGeneratedConfig represents the config.json that Evilginx reads
type EvilginxGeneratedConfig struct {
	General   EvilginxGeneral   `json:"general"`
	Blacklist EvilginxBlacklist `json:"blacklist"`
}

type EvilginxGeneral struct {
	Domain      string `json:"domain"`
	ExternalIP  string `json:"ipv4_external"`
	BindIP      string `json:"ipv4_bind"`
	RedirectURL string `json:"redirect_url"`
	HTTPS       int    `json:"https_port"`
	DNS         int    `json:"dns_port"`
	AutoCert    bool   `json:"autocert"`
}

type EvilginxBlacklist struct {
	Mode string `json:"mode"`
}

// GenerateEvilginxConfig creates config.json for Evilginx from engagement config
func GenerateEvilginxConfig(cfg EngagementConfig, publicIP string) EvilginxGeneratedConfig {
	return EvilginxGeneratedConfig{
		General: EvilginxGeneral{
			Domain:      cfg.Domain.Phishing,
			ExternalIP:  publicIP,
			BindIP:      publicIP,
			RedirectURL: cfg.Domain.RedirectURL,
			HTTPS:       443,
			DNS:         53,
			AutoCert:    cfg.Evilginx.AutoCert,
		},
		Blacklist: EvilginxBlacklist{
			Mode: "unauth",
		},
	}
}

// WriteEvilginxConfig writes the generated config to the Evilginx data directory
func WriteEvilginxConfig(cfg EvilginxGeneratedConfig, dataDir string) error {
	if err := os.MkdirAll(dataDir, 0750); err != nil {
		return fmt.Errorf("creating evilginx data dir: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling evilginx config: %w", err)
	}

	path := filepath.Join(dataDir, "config.json")
	if err := os.WriteFile(path, data, 0640); err != nil {
		return fmt.Errorf("writing evilginx config to %s: %w", path, err)
	}

	return nil
}

// GenerateSetupCommands returns the Evilginx interactive commands as a string
func GenerateSetupCommands(cfg EngagementConfig, publicIP string) string {
	return fmt.Sprintf(`config domain %s
config ip %s
config redirect_url %s
config autocert on
phishlets hostname %s %s
phishlets enable %s`,
		cfg.Domain.Phishing,
		publicIP,
		cfg.Domain.RedirectURL,
		cfg.Phishlet.Name,
		cfg.Phishlet.Hostname,
		cfg.Phishlet.Name,
	)
}
