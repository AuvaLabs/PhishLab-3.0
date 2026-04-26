package campaign

import (
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/AuvaLabs/PhishLab-3.0/internal/config"
	"github.com/AuvaLabs/PhishLab-3.0/internal/gophish"
	"github.com/AuvaLabs/PhishLab-3.0/internal/store"
)

// Orchestrator coordinates campaign creation across Gophish and the local store
type Orchestrator struct {
	db      store.Repository
	gp      *gophish.Client
	cfg     config.EngagementConfig
	eventCB func(store.TimelineEvent) // called on new events
}

// NewOrchestrator creates a campaign orchestrator
func NewOrchestrator(db store.Repository, gp *gophish.Client, cfg config.EngagementConfig, eventCB func(store.TimelineEvent)) *Orchestrator {
	return &Orchestrator{
		db:      db,
		gp:      gp,
		cfg:     cfg,
		eventCB: eventCB,
	}
}

// LaunchRequest describes a new campaign to launch
type LaunchRequest struct {
	Name     string `json:"name"`
	Template struct {
		Name    string `json:"name"`
		Subject string `json:"subject"`
		HTML    string `json:"html"`
		Text    string `json:"text"`
	} `json:"template"`
	LandingPage struct {
		Name        string `json:"name"`
		HTML        string `json:"html"`
		RedirectURL string `json:"redirect_url"`
	} `json:"landing_page"`
	LaunchDate string `json:"launch_date,omitempty"` // RFC3339; empty = immediate
}

// LaunchResult describes the outcome of a campaign launch
type LaunchResult struct {
	CampaignID int64  `json:"campaign_id"`
	GophishID  int64  `json:"gophish_id"`
	Status     string `json:"status"`
	PhishURL   string `json:"phish_url"`
}

// Launch creates all Gophish resources and launches a campaign
func (o *Orchestrator) Launch(req LaunchRequest) (LaunchResult, error) {
	if o.gp == nil {
		return LaunchResult{}, fmt.Errorf("gophish client not configured (set gophish.api_key)")
	}

	eng, err := o.db.GetActiveEngagement()
	if err != nil {
		return LaunchResult{}, fmt.Errorf("getting active engagement: %w", err)
	}
	if eng == nil {
		return LaunchResult{}, fmt.Errorf("no active engagement found (run 'evilginx-lab init' first)")
	}

	// 1. Create target group from config targets
	targets := make([]gophish.Target, len(o.cfg.Targets))
	for i, t := range o.cfg.Targets {
		targets[i] = gophish.Target{
			Email:     t.Email,
			FirstName: t.FirstName,
			LastName:  t.LastName,
			Position:  t.Position,
		}
	}
	if len(targets) == 0 {
		return LaunchResult{}, fmt.Errorf("no targets defined in engagement config")
	}

	groupName := fmt.Sprintf("%s - Targets", req.Name)
	group, err := o.gp.CreateGroup(gophish.Group{Name: groupName, Targets: targets})
	if err != nil {
		return LaunchResult{}, fmt.Errorf("creating target group: %w", err)
	}
	log.Printf("[orchestrator] target group created (ID: %d, targets: %d)", group.ID, len(targets))

	// 2. Create email template
	tmpl, err := o.gp.CreateTemplate(gophish.Template{
		Name:    req.Template.Name,
		Subject: req.Template.Subject,
		HTML:    req.Template.HTML,
		Text:    req.Template.Text,
	})
	if err != nil {
		return LaunchResult{}, fmt.Errorf("creating email template: %w", err)
	}
	log.Printf("[orchestrator] email template created (ID: %d)", tmpl.ID)

	// 3. Create landing page
	redirectURL := req.LandingPage.RedirectURL
	if redirectURL == "" {
		redirectURL = o.cfg.Domain.RedirectURL
	}
	page, err := o.gp.CreatePage(gophish.Page{
		Name:               req.LandingPage.Name,
		HTML:               req.LandingPage.HTML,
		CaptureCredentials: true,
		CapturePasswords:   true,
		RedirectURL:        redirectURL,
	})
	if err != nil {
		return LaunchResult{}, fmt.Errorf("creating landing page: %w", err)
	}
	log.Printf("[orchestrator] landing page created (ID: %d)", page.ID)

	// 4. Get sending profile
	profiles, err := o.gp.GetSendingProfiles()
	if err != nil {
		return LaunchResult{}, fmt.Errorf("getting sending profiles: %w", err)
	}
	if len(profiles) == 0 {
		return LaunchResult{}, fmt.Errorf("no sending profiles found (run 'evilginx-lab init' to create one)")
	}

	// 5. Build phish URL with RID tracking
	phishURL := buildPhishURL(o.cfg.Domain.Phishing)

	// 6. Launch campaign
	camp := gophish.Campaign{
		Name:     req.Name,
		Groups:   []gophish.Group{{Name: group.Name}},
		Template: gophish.Template{Name: tmpl.Name},
		Page:     gophish.Page{Name: page.Name},
		SMTP:     gophish.SendingProfile{Name: profiles[0].Name},
		URL:      phishURL,
	}
	if req.LaunchDate != "" {
		camp.LaunchDate = req.LaunchDate
	}

	created, err := o.gp.CreateCampaign(camp)
	if err != nil {
		return LaunchResult{}, fmt.Errorf("creating campaign: %w", err)
	}
	log.Printf("[orchestrator] campaign launched (Gophish ID: %d, status: %s)", created.ID, created.Status)

	// 7. Store campaign record locally
	record := store.CampaignRecord{
		EngagementID: eng.ID,
		GophishID:    created.ID,
		Name:         req.Name,
		Status:       "launched",
		TargetCount:  len(targets),
		PhishURL:     phishURL,
		TemplateName: req.Template.Name,
		LaunchedAt:   time.Now(),
	}
	localID, err := o.db.InsertCampaign(record)
	if err != nil {
		log.Printf("[orchestrator] warning: campaign launched but failed to store locally: %v", err)
	}

	// 8. Record launch event in timeline
	o.recordEvent(store.TimelineEvent{
		EngagementID: eng.ID,
		CampaignID:   localID,
		Source:       "gophish",
		EventType:    "campaign_launched",
		Detail:       fmt.Sprintf("Campaign '%s' launched with %d targets", req.Name, len(targets)),
		Timestamp:    time.Now(),
	})

	return LaunchResult{
		CampaignID: localID,
		GophishID:  created.ID,
		Status:     created.Status,
		PhishURL:   phishURL,
	}, nil
}

// SyncCampaignEvents polls Gophish for campaign timeline events and stores new ones
func (o *Orchestrator) SyncCampaignEvents(engagementID string) error {
	if o.gp == nil {
		return nil
	}

	campaigns, err := o.db.GetCampaigns(engagementID)
	if err != nil {
		return fmt.Errorf("getting local campaigns: %w", err)
	}

	for _, camp := range campaigns {
		if camp.GophishID == 0 {
			continue
		}
		gpCamp, err := o.gp.GetCampaign(camp.GophishID)
		if err != nil {
			log.Printf("[orchestrator] error fetching Gophish campaign %d: %v", camp.GophishID, err)
			continue
		}

		// Update status if changed
		if gpCamp.Status != "" && gpCamp.Status != camp.Status {
			if err := o.db.UpdateCampaignStatus(camp.ID, gpCamp.Status); err != nil {
				log.Printf("[orchestrator] error updating campaign status: %v", err)
			}
		}

		// Sync timeline events
		for _, evt := range gpCamp.Timeline {
			eventType := normalizeGophishEvent(evt.Message)
			o.recordEvent(store.TimelineEvent{
				EngagementID: engagementID,
				CampaignID:   camp.ID,
				Source:       "gophish",
				EventType:    eventType,
				Email:        evt.Email,
				Detail:       evt.Details,
				Timestamp:    parseGophishTime(evt.Time),
			})
		}

		// Sync result RIDs
		for _, result := range gpCamp.Results {
			if result.Status != "" {
				o.recordEvent(store.TimelineEvent{
					EngagementID: engagementID,
					CampaignID:   camp.ID,
					Source:       "gophish",
					EventType:    "target_status",
					Email:        result.Email,
					Detail:       result.Status,
					RemoteAddr:   result.IP,
					Timestamp:    time.Now(),
				})
			}
		}
	}

	return nil
}

func (o *Orchestrator) recordEvent(evt store.TimelineEvent) {
	if err := o.db.InsertTimelineEvent(evt); err != nil {
		log.Printf("[orchestrator] error recording event: %v", err)
		return
	}
	if o.eventCB != nil {
		o.eventCB(evt)
	}
}

func buildPhishURL(domain string) string {
	if strings.HasPrefix(domain, "http") {
		return domain
	}
	return "https://" + domain
}

func normalizeGophishEvent(message string) string {
	msg := strings.ToLower(message)
	switch {
	case strings.Contains(msg, "sent"):
		return "email_sent"
	case strings.Contains(msg, "opened"), strings.Contains(msg, "open"):
		return "email_opened"
	case strings.Contains(msg, "clicked"):
		return "link_clicked"
	case strings.Contains(msg, "submitted"):
		return "submitted_data"
	case strings.Contains(msg, "reported"):
		return "email_reported"
	default:
		return "unknown"
	}
}

func parseGophishTime(t string) time.Time {
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05.000000-07:00", time.RFC3339Nano} {
		if parsed, err := time.Parse(layout, t); err == nil {
			return parsed
		}
	}
	return time.Now()
}

// ExtractRIDFromURL parses a Gophish phishing URL to extract the RID parameter
func ExtractRIDFromURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Query().Get("rid")
}
