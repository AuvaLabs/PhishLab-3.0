package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type EngagementConfig struct {
	Engagement EngagementInfo `yaml:"engagement"`
	Domain     DomainConfig   `yaml:"domain"`
	Phishlet   PhishletConfig `yaml:"phishlet"`
	Evilginx   EvilginxConfig `yaml:"evilginx"`
	Gophish    GophishConfig  `yaml:"gophish"`
	SMTP       SMTPConfig     `yaml:"smtp"`
	Targets    []Target       `yaml:"targets"`
	Dashboard  DashboardConf  `yaml:"dashboard"`
	Store      StoreConfig    `yaml:"store"`
	Polling    PollingConfig  `yaml:"polling"`
}

type EngagementInfo struct {
	Name         string `yaml:"name"`
	Client       string `yaml:"client"`
	ID           string `yaml:"id"`
	StartDate    string `yaml:"start_date"`
	EndDate      string `yaml:"end_date"`
	Operator     string `yaml:"operator"`
	RoEReference string `yaml:"roe_reference"`
	Notes        string `yaml:"notes"`
}

type DomainConfig struct {
	Phishing    string `yaml:"phishing"`
	RedirectURL string `yaml:"redirect_url"`
}

type PhishletConfig struct {
	Name       string `yaml:"name"`
	Hostname   string `yaml:"hostname"`
	AutoEnable bool   `yaml:"auto_enable"`
}

type EvilginxConfig struct {
	InstallDir  string `yaml:"install_dir"`
	PhishletsDir string `yaml:"phishlets_dir"`
	AutoCert    bool   `yaml:"autocert"`
}

type GophishConfig struct {
	AdminURL string `yaml:"admin_url"`
	APIKey   string `yaml:"api_key"`
}

type SMTPConfig struct {
	Mode             string `yaml:"mode"`
	Host             string `yaml:"host"`
	Port             int    `yaml:"port"`
	Username         string `yaml:"username"`
	Password         string `yaml:"password"`
	FromAddress      string `yaml:"from_address"`
	FromName         string `yaml:"from_name"`
	TLS              bool   `yaml:"tls"`
	IgnoreCertErrors bool   `yaml:"ignore_cert_errors"`
}

type Target struct {
	Email     string `yaml:"email"`
	FirstName string `yaml:"first_name"`
	LastName  string `yaml:"last_name"`
	Position  string `yaml:"position"`
}

type DashboardConf struct {
	Listen string `yaml:"listen"`
}

type StoreConfig struct {
	Path string `yaml:"path"`
}

type PollingConfig struct {
	Interval int `yaml:"interval"`
}

func LoadConfig(path string) (EngagementConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return EngagementConfig{}, fmt.Errorf("reading config %s: %w", path, err)
	}

	var cfg EngagementConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return EngagementConfig{}, fmt.Errorf("parsing config %s: %w", path, err)
	}

	if err := cfg.validate(); err != nil {
		return EngagementConfig{}, fmt.Errorf("validating config: %w", err)
	}

	return cfg, nil
}

func (c EngagementConfig) validate() error {
	if c.Engagement.Name == "" {
		return fmt.Errorf("engagement.name is required")
	}
	if c.Domain.Phishing == "" {
		return fmt.Errorf("domain.phishing is required")
	}
	if c.Phishlet.Name == "" {
		return fmt.Errorf("phishlet.name is required")
	}

	if c.Engagement.StartDate != "" {
		if _, err := time.Parse("2006-01-02", c.Engagement.StartDate); err != nil {
			return fmt.Errorf("engagement.start_date invalid format (use YYYY-MM-DD): %w", err)
		}
	}
	if c.Engagement.EndDate != "" {
		if _, err := time.Parse("2006-01-02", c.Engagement.EndDate); err != nil {
			return fmt.Errorf("engagement.end_date invalid format (use YYYY-MM-DD): %w", err)
		}
	}

	return nil
}

func (c EngagementConfig) WithDefaults() EngagementConfig {
	result := c
	if result.Evilginx.InstallDir == "" {
		result.Evilginx.InstallDir = "/opt/evilginx2"
	}
	if result.Evilginx.PhishletsDir == "" {
		result.Evilginx.PhishletsDir = result.Evilginx.InstallDir + "/phishlets"
	}
	if result.Gophish.AdminURL == "" {
		result.Gophish.AdminURL = "http://127.0.0.1:8800"
	}
	if result.SMTP.Mode == "" {
		result.SMTP.Mode = "mailhog"
	}
	if result.SMTP.Host == "" {
		result.SMTP.Host = "localhost"
	}
	if result.SMTP.Port == 0 {
		result.SMTP.Port = 1025
	}
	if result.Dashboard.Listen == "" {
		result.Dashboard.Listen = "127.0.0.1:8443"
	}
	if result.Store.Path == "" {
		result.Store.Path = "/var/lib/evilginx-lab/evilginx-lab.db"
	}
	if result.Polling.Interval == 0 {
		result.Polling.Interval = 5
	}
	return result
}
