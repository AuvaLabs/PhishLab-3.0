package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"sync"

	"github.com/AuvaLabs/PhishLab-3.0/internal/campaign"
	"github.com/AuvaLabs/PhishLab-3.0/internal/evilginx"
	"github.com/AuvaLabs/PhishLab-3.0/internal/gophish"
	"github.com/AuvaLabs/PhishLab-3.0/internal/store"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

// APIHandler holds dependencies for API route handlers
type APIHandler struct {
	DB           store.Repository
	Evilginx    *evilginx.Client
	Gophish     *gophish.Client
	Orchestrator *campaign.Orchestrator

	// WebSocket event broadcasting
	wsUpgrader websocket.Upgrader
	wsClients  map[*websocket.Conn]bool
	wsMu       sync.RWMutex
}

func NewAPIHandler(db store.Repository, eg *evilginx.Client, gp *gophish.Client) *APIHandler {
	return &APIHandler{
		DB:       db,
		Evilginx: eg,
		Gophish:  gp,
		wsUpgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // localhost only, no CORS concerns
			},
		},
		wsClients: make(map[*websocket.Conn]bool),
	}
}

// SetOrchestrator sets the campaign orchestrator (called after handler creation)
func (h *APIHandler) SetOrchestrator(o *campaign.Orchestrator) {
	h.Orchestrator = o
}

// BroadcastEvent sends an event to all connected WebSocket clients
func (h *APIHandler) BroadcastEvent(event any) {
	data, err := json.Marshal(event)
	if err != nil {
		log.Printf("[ws] error marshaling event: %v", err)
		return
	}

	h.wsMu.RLock()
	defer h.wsMu.RUnlock()

	for conn := range h.wsClients {
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			log.Printf("[ws] error writing to client: %v", err)
			conn.Close()
			delete(h.wsClients, conn)
		}
	}
}

// HandleWebSocket upgrades HTTP to WebSocket for real-time events
func (h *APIHandler) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := h.wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[ws] upgrade error: %v", err)
		return
	}

	h.wsMu.Lock()
	h.wsClients[conn] = true
	h.wsMu.Unlock()

	log.Printf("[ws] client connected from %s", r.RemoteAddr)

	// Keep connection alive; remove on close
	defer func() {
		h.wsMu.Lock()
		delete(h.wsClients, conn)
		h.wsMu.Unlock()
		conn.Close()
	}()

	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}
}

// HandleDashboard returns the dashboard summary
func (h *APIHandler) HandleDashboard(w http.ResponseWriter, r *http.Request) {
	eng, err := h.DB.GetActiveEngagement()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get engagement: "+err.Error())
		return
	}

	services := h.getServiceHealth()
	phishlets := h.getPhishletInfo()

	var creds []store.CapturedCredential
	var credCount int
	var campaigns []store.CampaignRecord
	var timeline []store.TimelineEvent
	if eng != nil {
		creds, _ = h.DB.GetCredentials(eng.ID)
		credCount, _ = h.DB.CredentialCount(eng.ID)
		campaigns, _ = h.DB.GetCampaigns(eng.ID)
		timeline, _ = h.DB.GetTimeline(eng.ID, 50)
	}
	if creds == nil {
		creds = []store.CapturedCredential{}
	}
	if campaigns == nil {
		campaigns = []store.CampaignRecord{}
	}
	if timeline == nil {
		timeline = []store.TimelineEvent{}
	}

	var campaignCount int
	if h.Gophish != nil {
		gpCampaigns, err := h.Gophish.GetCampaigns()
		if err == nil {
			campaignCount = len(gpCampaigns)
		}
	}

	summary := store.DashboardSummary{
		Engagement:      eng,
		Services:        services,
		Phishlets:       phishlets,
		Credentials:     creds,
		Campaigns:       campaigns,
		Timeline:        timeline,
		CredentialCount: credCount,
		CampaignCount:   campaignCount,
	}

	writeJSON(w, http.StatusOK, summary)
}

// HandleCredentials returns all captured credentials
func (h *APIHandler) HandleCredentials(w http.ResponseWriter, r *http.Request) {
	creds, err := h.DB.GetAllCredentials()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get credentials: "+err.Error())
		return
	}
	if creds == nil {
		creds = []store.CapturedCredential{}
	}
	writeJSON(w, http.StatusOK, creds)
}

// HandleServices returns service health status
func (h *APIHandler) HandleServices(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.getServiceHealth())
}

// HandlePhishlets returns available phishlet info
func (h *APIHandler) HandlePhishlets(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.getPhishletInfo())
}

// HandleGetCampaigns returns all campaigns for the active engagement
func (h *APIHandler) HandleGetCampaigns(w http.ResponseWriter, r *http.Request) {
	eng, err := h.DB.GetActiveEngagement()
	if err != nil || eng == nil {
		writeJSON(w, http.StatusOK, []store.CampaignRecord{})
		return
	}

	campaigns, err := h.DB.GetCampaigns(eng.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get campaigns: "+err.Error())
		return
	}
	if campaigns == nil {
		campaigns = []store.CampaignRecord{}
	}
	writeJSON(w, http.StatusOK, campaigns)
}

// HandleLaunchCampaign creates and launches a new campaign via the orchestrator
func (h *APIHandler) HandleLaunchCampaign(w http.ResponseWriter, r *http.Request) {
	if h.Orchestrator == nil {
		writeError(w, http.StatusServiceUnavailable, "campaign orchestrator not configured")
		return
	}

	var req campaign.LaunchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "campaign name is required")
		return
	}
	if req.Template.Name == "" {
		writeError(w, http.StatusBadRequest, "template name is required")
		return
	}

	result, err := h.Orchestrator.Launch(req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "campaign launch failed: "+err.Error())
		return
	}

	h.BroadcastEvent(map[string]any{
		"type":        "campaign_launched",
		"campaign_id": result.CampaignID,
		"gophish_id":  result.GophishID,
		"status":      result.Status,
	})

	writeJSON(w, http.StatusCreated, result)
}

// HandleSyncCampaigns triggers a sync of Gophish campaign events
func (h *APIHandler) HandleSyncCampaigns(w http.ResponseWriter, r *http.Request) {
	if h.Orchestrator == nil {
		writeError(w, http.StatusServiceUnavailable, "campaign orchestrator not configured")
		return
	}

	eng, err := h.DB.GetActiveEngagement()
	if err != nil || eng == nil {
		writeError(w, http.StatusNotFound, "no active engagement")
		return
	}

	if err := h.Orchestrator.SyncCampaignEvents(eng.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "sync failed: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "synced"})
}

// HandleGetTimeline returns timeline events for the active engagement
func (h *APIHandler) HandleGetTimeline(w http.ResponseWriter, r *http.Request) {
	eng, err := h.DB.GetActiveEngagement()
	if err != nil || eng == nil {
		writeJSON(w, http.StatusOK, []store.TimelineEvent{})
		return
	}

	limit := 200
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	// Check for campaign_id filter
	var events []store.TimelineEvent
	if cid := r.URL.Query().Get("campaign_id"); cid != "" {
		campaignID, err := strconv.ParseInt(cid, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid campaign_id")
			return
		}
		events, err = h.DB.GetTimelineByCampaign(campaignID, limit)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to get timeline: "+err.Error())
			return
		}
	} else {
		events, err = h.DB.GetTimeline(eng.ID, limit)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to get timeline: "+err.Error())
			return
		}
	}

	if events == nil {
		events = []store.TimelineEvent{}
	}
	writeJSON(w, http.StatusOK, events)
}

// HandleGetTemplates returns all Gophish email templates
func (h *APIHandler) HandleGetTemplates(w http.ResponseWriter, r *http.Request) {
	if h.Gophish == nil {
		writeJSON(w, http.StatusOK, []gophish.Template{})
		return
	}

	templates, err := h.Gophish.GetTemplates()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get templates: "+err.Error())
		return
	}
	if templates == nil {
		templates = []gophish.Template{}
	}
	writeJSON(w, http.StatusOK, templates)
}

// HandleCreateTemplate creates a new Gophish email template
func (h *APIHandler) HandleCreateTemplate(w http.ResponseWriter, r *http.Request) {
	if h.Gophish == nil {
		writeError(w, http.StatusServiceUnavailable, "gophish client not configured")
		return
	}

	var tmpl gophish.Template
	if err := json.NewDecoder(r.Body).Decode(&tmpl); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if tmpl.Name == "" {
		writeError(w, http.StatusBadRequest, "template name is required")
		return
	}

	created, err := h.Gophish.CreateTemplate(tmpl)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create template: "+err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

// HandleDeleteTemplate deletes a Gophish email template
func (h *APIHandler) HandleDeleteTemplate(w http.ResponseWriter, r *http.Request) {
	if h.Gophish == nil {
		writeError(w, http.StatusServiceUnavailable, "gophish client not configured")
		return
	}

	idStr := mux.Vars(r)["id"]
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid template id")
		return
	}

	if err := h.Gophish.DeleteTemplate(id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete template: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *APIHandler) getServiceHealth() []store.ServiceHealth {
	services := []store.ServiceHealth{}
	for _, name := range []string{"evilginx", "gophish", "mailhog"} {
		status, _ := getSystemdStatus(name)
		services = append(services, store.ServiceHealth{
			Name:   name,
			Status: status,
		})
	}
	return services
}

func (h *APIHandler) getPhishletInfo() []store.PhishletInfo {
	var infos []store.PhishletInfo
	if h.Evilginx == nil {
		return infos
	}

	names, err := h.Evilginx.ListPhishlets()
	if err != nil {
		return infos
	}
	for _, name := range names {
		infos = append(infos, store.PhishletInfo{
			Name:    name,
			Enabled: false,
		})
	}
	return infos
}

func getSystemdStatus(service string) (string, error) {
	return evilginx.ServiceStatus(service)
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
