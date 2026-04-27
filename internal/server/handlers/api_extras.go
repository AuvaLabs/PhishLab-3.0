package handlers

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"

	"github.com/AuvaLabs/PhishLab-3.0/internal/gophish"
	"github.com/AuvaLabs/PhishLab-3.0/internal/store"
	"github.com/gorilla/mux"
)

// landingSubdomain maps a phishlet name to the subdomain its lure
// landing page is served on. Sourced from the upstream phishlets'
// is_landing proxy_hosts entries; kept as a lookup table to avoid
// reading and parsing every phishlet YAML on each /api/lures call.
var landingSubdomain = map[string]string{
	"o365":                "login",
	"microsoft-o365-adfs": "login",
	"microsoft-live":      "login",
	"google":              "accounts",
	"github":              "github",
	"linkedin":            "www",
	"twitter":             "twitter",
	"facebook":            "www",
	"instagram":           "www",
	"okta":                "login",
}

// LureView is the dashboard-facing summary of an Evilginx lure.
type LureView struct {
	Phishlet string `json:"phishlet"`
	Path     string `json:"path"`
	URL      string `json:"url"`
	Paused   int    `json:"paused"`
	Info     string `json:"info"`
}

// HandleGetLures reads the live Evilginx config and returns the
// list of currently-configured lures with their full landing URLs
// so the operator can copy/paste straight from the dashboard.
func (h *APIHandler) HandleGetLures(w http.ResponseWriter, r *http.Request) {
	const cfgPath = "/opt/evilginx2/state/config.json"
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		writeJSON(w, http.StatusOK, []LureView{})
		return
	}
	var cfg struct {
		General struct {
			Domain string `json:"domain"`
		} `json:"general"`
		Phishlets map[string]struct {
			Hostname string `json:"hostname"`
			Enabled  bool   `json:"enabled"`
		} `json:"phishlets"`
		Lures []struct {
			Hostname string `json:"hostname"`
			Path     string `json:"path"`
			Phishlet string `json:"phishlet"`
			Paused   int    `json:"paused"`
			Info     string `json:"info"`
		} `json:"lures"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		writeError(w, http.StatusInternalServerError, "parse evilginx config: "+err.Error())
		return
	}
	out := make([]LureView, 0, len(cfg.Lures))
	for _, l := range cfg.Lures {
		host := l.Hostname
		if host == "" {
			ph, ok := cfg.Phishlets[l.Phishlet]
			if ok && ph.Hostname != "" {
				host = ph.Hostname
			} else {
				host = cfg.General.Domain
			}
		}
		sub := landingSubdomain[l.Phishlet]
		landing := host
		if sub != "" {
			landing = sub + "." + host
		}
		out = append(out, LureView{
			Phishlet: l.Phishlet,
			Path:     l.Path,
			URL:      "https://" + landing + l.Path,
			Paused:   l.Paused,
			Info:     l.Info,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// HandleGetGroups proxies the Gophish groups list to the dashboard.
func (h *APIHandler) HandleGetGroups(w http.ResponseWriter, r *http.Request) {
	if h.Gophish == nil {
		writeJSON(w, http.StatusOK, []gophish.Group{})
		return
	}
	groups, err := h.Gophish.GetGroups()
	if err != nil {
		writeError(w, http.StatusBadGateway, "gophish error: "+err.Error())
		return
	}
	if groups == nil {
		groups = []gophish.Group{}
	}
	writeJSON(w, http.StatusOK, groups)
}

// HandleCreateGroup creates a Gophish group of targets from a JSON
// body of the form:
//
//	{"name":"Pilot Group","targets":[
//	   {"email":"a@x.com","first_name":"A","last_name":"X","position":"IT"}
//	]}
func (h *APIHandler) HandleCreateGroup(w http.ResponseWriter, r *http.Request) {
	if h.Gophish == nil {
		writeError(w, http.StatusServiceUnavailable, "gophish not configured")
		return
	}
	var g gophish.Group
	if err := json.NewDecoder(r.Body).Decode(&g); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body: "+err.Error())
		return
	}
	if g.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	created, err := h.Gophish.CreateGroup(g)
	if err != nil {
		writeError(w, http.StatusBadGateway, "gophish error: "+err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

// HandleGetSendingProfiles proxies the Gophish sending-profiles list.
func (h *APIHandler) HandleGetSendingProfiles(w http.ResponseWriter, r *http.Request) {
	if h.Gophish == nil {
		writeJSON(w, http.StatusOK, []gophish.SendingProfile{})
		return
	}
	profiles, err := h.Gophish.GetSendingProfiles()
	if err != nil {
		writeError(w, http.StatusBadGateway, "gophish error: "+err.Error())
		return
	}
	if profiles == nil {
		profiles = []gophish.SendingProfile{}
	}
	writeJSON(w, http.StatusOK, profiles)
}

// HandleGetPages proxies the Gophish landing-pages list.
func (h *APIHandler) HandleGetPages(w http.ResponseWriter, r *http.Request) {
	if h.Gophish == nil {
		writeJSON(w, http.StatusOK, []gophish.Page{})
		return
	}
	pages, err := h.Gophish.GetPages()
	if err != nil {
		writeError(w, http.StatusBadGateway, "gophish error: "+err.Error())
		return
	}
	if pages == nil {
		pages = []gophish.Page{}
	}
	writeJSON(w, http.StatusOK, pages)
}

// HandleGetCampaignDetail returns a single Gophish campaign by id,
// including its full result list (per-target send/open/click/submit).
// The dashboard uses this to populate the campaign-results modal.
func (h *APIHandler) HandleGetCampaignDetail(w http.ResponseWriter, r *http.Request) {
	if h.Gophish == nil {
		writeError(w, http.StatusServiceUnavailable, "gophish not configured")
		return
	}
	idStr := mux.Vars(r)["id"]
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad campaign id")
		return
	}
	camp, err := h.Gophish.GetCampaign(id)
	if err != nil {
		writeError(w, http.StatusBadGateway, "gophish error: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, camp)
}

// engagementUpdateRequest mirrors the editable subset of the Engagement
// model. Status and timestamps are managed server-side.
type engagementUpdateRequest struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Client       string `json:"client"`
	Operator     string `json:"operator"`
	StartDate    string `json:"start_date"`
	EndDate      string `json:"end_date"`
	Domain       string `json:"domain"`
	PhishletName string `json:"phishlet_name"`
	RoEReference string `json:"roe_reference"`
	Notes        string `json:"notes"`
}

// HandleUpdateEngagement updates the active engagement record from a
// JSON body. The runtime DB is the source of truth for the dashboard;
// the operator can re-sync `evilginx-lab.yaml` separately if desired
// (the deploy command upserts from yaml on every restart).
func (h *APIHandler) HandleUpdateEngagement(w http.ResponseWriter, r *http.Request) {
	var req engagementUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body: "+err.Error())
		return
	}
	if req.ID == "" {
		eng, _ := h.DB.GetActiveEngagement()
		if eng == nil {
			writeError(w, http.StatusBadRequest, "id is required and no active engagement exists")
			return
		}
		req.ID = eng.ID
	}
	if err := h.DB.UpsertEngagement(store.Engagement{
		ID:           req.ID,
		Name:         req.Name,
		Client:       req.Client,
		Operator:     req.Operator,
		StartDate:    req.StartDate,
		EndDate:      req.EndDate,
		Domain:       req.Domain,
		PhishletName: req.PhishletName,
		RoEReference: req.RoEReference,
		Notes:        req.Notes,
		Status:       "active",
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "upsert failed: "+err.Error())
		return
	}
	updated, _ := h.DB.GetEngagement(req.ID)
	writeJSON(w, http.StatusOK, updated)
}
