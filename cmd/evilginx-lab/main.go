package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/CyberOneHQ/evilginx-lab/internal/campaign"
	"github.com/CyberOneHQ/evilginx-lab/internal/config"
	"github.com/CyberOneHQ/evilginx-lab/internal/evilginx"
	"github.com/CyberOneHQ/evilginx-lab/internal/gophish"
	"github.com/CyberOneHQ/evilginx-lab/internal/server"
	"github.com/CyberOneHQ/evilginx-lab/internal/server/handlers"
	"github.com/CyberOneHQ/evilginx-lab/internal/store"
	"github.com/spf13/cobra"
)

var cfgFile string

func main() {
	rootCmd := &cobra.Command{
		Use:   "evilginx-lab",
		Short: "Evilginx-Lab - Red team phishing engagement platform",
		Long:  "Evilginx-Lab orchestrates Evilginx3, Gophish, and Mailhog into a unified phishing engagement platform for authorized red team operations.",
	}

	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "evilginx-lab.yaml", "engagement config file")

	rootCmd.AddCommand(initCmd())
	rootCmd.AddCommand(deployCmd())
	rootCmd.AddCommand(statusCmd())
	rootCmd.AddCommand(serveCmd())

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func initCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize a new engagement from evilginx-lab.yaml",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadAndValidateConfig()
			if err != nil {
				return err
			}

			// Resolve public IP
			publicIP, err := resolvePublicIP()
			if err != nil {
				return fmt.Errorf("resolving public IP: %w", err)
			}
			fmt.Printf("Public IP: %s\n", publicIP)

			// Generate Evilginx config
			egCfg := config.GenerateEvilginxConfig(cfg, publicIP)
			dataDir := cfg.Evilginx.InstallDir
			if err := config.WriteEvilginxConfig(egCfg, dataDir); err != nil {
				return fmt.Errorf("writing evilginx config: %w", err)
			}
			fmt.Printf("Evilginx config written to %s/config.json\n", dataDir)

			// Write setup commands next to the evilginx config
			cmds := config.GenerateSetupCommands(cfg, publicIP)
			cmdsPath := filepath.Join(dataDir, "setup_commands.txt")
			if err := os.WriteFile(cmdsPath, []byte(cmds), 0640); err != nil {
				return fmt.Errorf("writing setup commands: %w", err)
			}
			fmt.Printf("Setup commands written to %s\n", cmdsPath)

			// Initialize SQLite store and create engagement
			db, err := store.Open(cfg.Store.Path)
			if err != nil {
				return fmt.Errorf("opening store: %w", err)
			}
			defer db.Close()

			eng := store.Engagement{
				ID:           cfg.Engagement.ID,
				Name:         cfg.Engagement.Name,
				Client:       cfg.Engagement.Client,
				Operator:     cfg.Engagement.Operator,
				StartDate:    cfg.Engagement.StartDate,
				EndDate:      cfg.Engagement.EndDate,
				Domain:       cfg.Domain.Phishing,
				PhishletName: cfg.Phishlet.Name,
				RoEReference: cfg.Engagement.RoEReference,
				Notes:        cfg.Engagement.Notes,
				Status:       "active",
			}
			if err := db.UpsertEngagement(eng); err != nil {
				return fmt.Errorf("creating engagement: %w", err)
			}
			fmt.Printf("Engagement '%s' initialized (ID: %s)\n", eng.Name, eng.ID)

			// Auto-configure Gophish sending profile if API key is set
			if cfg.Gophish.APIKey != "" {
				gp := gophish.NewClient(cfg.Gophish.AdminURL, cfg.Gophish.APIKey)
				sp := gophish.SendingProfile{
					Name:             fmt.Sprintf("Evilginx-Lab - %s", cfg.SMTP.Mode),
					Host:             fmt.Sprintf("%s:%d", cfg.SMTP.Host, cfg.SMTP.Port),
					FromAddress:      fmt.Sprintf("%s <%s>", cfg.SMTP.FromName, cfg.SMTP.FromAddress),
					Username:         cfg.SMTP.Username,
					Password:         cfg.SMTP.Password,
					IgnoreCertErrors: cfg.SMTP.IgnoreCertErrors,
				}
				created, err := gp.CreateSendingProfile(sp)
				if err != nil {
					fmt.Printf("Warning: could not create Gophish sending profile: %v\n", err)
				} else {
					fmt.Printf("Gophish sending profile created (ID: %d)\n", created.ID)
				}
			} else {
				fmt.Println("Gophish API key not set - skipping sending profile creation")
				fmt.Println("Set gophish.api_key in evilginx-lab.yaml after first Gophish login")
			}

			fmt.Println("\nEngagement initialized successfully.")
			fmt.Println("Run 'evilginx-lab deploy' to start all services.")
			return nil
		},
	}
}

func deployCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "deploy",
		Short: "Start all services and the dashboard",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadAndValidateConfig()
			if err != nil {
				return err
			}

			// Verify Evilginx binary exists
			egClient := evilginx.NewClient(cfg.Evilginx.InstallDir, cfg.Evilginx.PhishletsDir)
			if !egClient.IsInstalled() {
				return fmt.Errorf("evilginx binary not found at %s - run install.sh first", egClient.BinaryPath())
			}

			// Restart services
			fmt.Println("Starting services...")
			for _, svc := range []string{"evilginx", "gophish", "mailhog"} {
				if err := restartService(svc); err != nil {
					fmt.Printf("Warning: could not restart %s: %v\n", svc, err)
				} else {
					fmt.Printf("  %s: started\n", svc)
				}
			}

			// Open store
			db, err := store.Open(cfg.Store.Path)
			if err != nil {
				return fmt.Errorf("opening store: %w", err)
			}
			defer db.Close()

			// Start session poller
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			gpClient := gophish.NewClient(cfg.Gophish.AdminURL, cfg.Gophish.APIKey)
			apiHandler := handlers.NewAPIHandler(db, egClient, gpClient)

			eng, _ := db.GetActiveEngagement()
			engID := ""
			if eng != nil {
				engID = eng.ID
			}

			// Set up campaign orchestrator with event broadcasting
			orchestrator := campaign.NewOrchestrator(db, gpClient, cfg, func(evt store.TimelineEvent) {
				apiHandler.BroadcastEvent(map[string]any{
					"type":       "timeline_event",
					"event_type": evt.EventType,
					"email":      evt.Email,
					"source":     evt.Source,
					"time":       evt.Timestamp.Format(time.RFC3339),
				})
			})
			apiHandler.SetOrchestrator(orchestrator)

			poller := evilginx.NewSessionPoller(
				egClient.BBoltDBPath(),
				time.Duration(cfg.Polling.Interval)*time.Second,
				func(s evilginx.CapturedSession) {
					tokensJSON, _ := json.Marshal(s.Tokens)
					cred := store.CapturedCredential{
						EngagementID: engID,
						SessionID:    s.ID,
						Phishlet:     s.Phishlet,
						Username:     s.Username,
						Password:     s.Password,
						TokensJSON:   string(tokensJSON),
						UserAgent:    s.UserAgent,
						RemoteAddr:   s.RemoteAddr,
						CapturedAt:   time.Unix(s.CreateTime, 0),
					}
					if err := db.InsertCredential(cred); err != nil {
						log.Printf("[poller] error storing credential: %v", err)
					} else {
						log.Printf("[poller] new credential captured: %s -> %s", s.Username, s.Phishlet)
						// Record in unified timeline
						db.InsertTimelineEvent(store.TimelineEvent{
							EngagementID: engID,
							Source:       "evilginx",
							EventType:    "credential_captured",
							Email:        s.Username,
							Detail:       fmt.Sprintf("Phishlet: %s, IP: %s", s.Phishlet, s.RemoteAddr),
							RemoteAddr:   s.RemoteAddr,
							Timestamp:    time.Unix(s.CreateTime, 0),
						})
						apiHandler.BroadcastEvent(map[string]any{
							"type":     "credential_captured",
							"username": s.Username,
							"phishlet": s.Phishlet,
							"time":     time.Now().Format(time.RFC3339),
						})
					}
				},
			)
			poller.Start(ctx)

			// Start dashboard server
			router := server.NewRouter(apiHandler)
			srv := &http.Server{
				Addr:    cfg.Dashboard.Listen,
				Handler: router,
			}

			go func() {
				sigCh := make(chan os.Signal, 1)
				signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
				<-sigCh
				fmt.Println("\nShutting down...")
				cancel()
				srv.Close()
			}()

			fmt.Printf("\nDashboard running at http://%s\n", cfg.Dashboard.Listen)
			fmt.Printf("Behind ops panel: https://dashboard.<your-domain>:8443  (nginx vhost; install.sh --with-ops-panel)\n")
			fmt.Printf("SSH tunnel fallback: ssh -L 9000:%s user@server\n", cfg.Dashboard.Listen)
			fmt.Println("Press Ctrl+C to stop")

			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				return fmt.Errorf("dashboard server: %w", err)
			}
			return nil
		},
	}
}

func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show engagement status, service health, and capture count",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadAndValidateConfig()
			if err != nil {
				return err
			}

			fmt.Println("=== Evilginx-Lab Status ===")
			fmt.Println()

			// Engagement info
			db, err := store.Open(cfg.Store.Path)
			if err != nil {
				fmt.Printf("Store: unavailable (%v)\n", err)
			} else {
				defer db.Close()
				eng, err := db.GetActiveEngagement()
				if err != nil {
					fmt.Printf("Engagement: error (%v)\n", err)
				} else if eng == nil {
					fmt.Println("Engagement: none active (run 'evilginx-lab init')")
				} else {
					fmt.Printf("Engagement: %s\n", eng.Name)
					fmt.Printf("  Client:   %s\n", eng.Client)
					fmt.Printf("  Domain:   %s\n", eng.Domain)
					fmt.Printf("  Phishlet: %s\n", eng.PhishletName)
					fmt.Printf("  Window:   %s to %s\n", eng.StartDate, eng.EndDate)
					fmt.Printf("  Status:   %s\n", eng.Status)

					count, _ := db.CredentialCount(eng.ID)
					fmt.Printf("  Captures: %d\n", count)
				}
			}

			fmt.Println()

			// Service health
			fmt.Println("Services:")
			for _, svc := range []string{"evilginx", "gophish", "mailhog", "evilginx-lab"} {
				status, _ := evilginx.ServiceStatus(svc)
				indicator := "[-]"
				if status == "active" {
					indicator = "[+]"
				}
				fmt.Printf("  %s %s: %s\n", indicator, svc, status)
			}

			fmt.Println()

			// Phishlets
			egClient := evilginx.NewClient(cfg.Evilginx.InstallDir, cfg.Evilginx.PhishletsDir)
			if egClient.IsInstalled() {
				phishlets, err := egClient.ListPhishlets()
				if err == nil && len(phishlets) > 0 {
					fmt.Printf("Phishlets: %d available (%s)\n", len(phishlets), strings.Join(phishlets, ", "))
				}
			} else {
				fmt.Println("Evilginx: binary not found")
			}

			// Gophish connectivity
			if cfg.Gophish.APIKey != "" {
				gp := gophish.NewClient(cfg.Gophish.AdminURL, cfg.Gophish.APIKey)
				if err := gp.Ping(); err != nil {
					fmt.Printf("Gophish API: unreachable (%v)\n", err)
				} else {
					campaigns, _ := gp.GetCampaigns()
					fmt.Printf("Gophish API: connected (%d campaigns)\n", len(campaigns))
				}
			}

			return nil
		},
	}
}

func serveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Start only the dashboard server (no service management)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadAndValidateConfig()
			if err != nil {
				return err
			}

			db, err := store.Open(cfg.Store.Path)
			if err != nil {
				return fmt.Errorf("opening store: %w", err)
			}
			defer db.Close()

			egClient := evilginx.NewClient(cfg.Evilginx.InstallDir, cfg.Evilginx.PhishletsDir)
			gpClient := gophish.NewClient(cfg.Gophish.AdminURL, cfg.Gophish.APIKey)
			apiHandler := handlers.NewAPIHandler(db, egClient, gpClient)

			router := server.NewRouter(apiHandler)
			srv := &http.Server{
				Addr:    cfg.Dashboard.Listen,
				Handler: router,
			}

			go func() {
				sigCh := make(chan os.Signal, 1)
				signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
				<-sigCh
				srv.Close()
			}()

			fmt.Printf("Dashboard at http://%s\n", cfg.Dashboard.Listen)
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				return fmt.Errorf("server: %w", err)
			}
			return nil
		},
	}
}

func loadAndValidateConfig() (config.EngagementConfig, error) {
	cfg, err := config.LoadConfig(cfgFile)
	if err != nil {
		return config.EngagementConfig{}, fmt.Errorf("loading config from %s: %w", cfgFile, err)
	}
	return cfg.WithDefaults(), nil
}

func resolvePublicIP() (string, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	// Use IPv4-only endpoints to ensure we get an IPv4 address
	for _, url := range []string{"https://api.ipify.org", "https://ipv4.icanhazip.com", "https://ifconfig.me"} {
		resp, err := client.Get(url)
		if err != nil {
			continue
		}
		defer resp.Body.Close()
		buf := make([]byte, 64)
		n, _ := resp.Body.Read(buf)
		ip := strings.TrimSpace(string(buf[:n]))
		parsed := net.ParseIP(ip)
		if parsed != nil && parsed.To4() != nil {
			return ip, nil
		}
	}
	return "", fmt.Errorf("could not determine public IPv4 from any resolver")
}

func restartService(name string) error {
	return evilginx.RestartService(name)
}
