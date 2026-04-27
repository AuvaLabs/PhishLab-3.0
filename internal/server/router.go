package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/AuvaLabs/PhishLab-3.0/internal/auth"
	"github.com/AuvaLabs/PhishLab-3.0/internal/server/handlers"
	"github.com/AuvaLabs/PhishLab-3.0/internal/server/middleware"
	"github.com/gorilla/mux"
)

// NewRouter wires the dashboard router. The auth.Handler is optional;
// pass nil to disable Google OAuth and rely on legacy nginx basic-auth
// gating. When non-nil, OAuth gates everything except /healthz and
// /auth/* and a session cookie is required for /api, /ws, and /.
func NewRouter(api *handlers.APIHandler, authH *auth.Handler) http.Handler {
	r := mux.NewRouter()

	apiRouter := r.PathPrefix("/api").Subrouter()
	apiRouter.HandleFunc("/dashboard", api.HandleDashboard).Methods("GET")
	apiRouter.HandleFunc("/credentials", api.HandleCredentials).Methods("GET")
	apiRouter.HandleFunc("/services", api.HandleServices).Methods("GET")
	apiRouter.HandleFunc("/phishlets", api.HandlePhishlets).Methods("GET")

	apiRouter.HandleFunc("/campaigns", api.HandleGetCampaigns).Methods("GET")
	apiRouter.HandleFunc("/campaigns", api.HandleLaunchCampaign).Methods("POST")
	apiRouter.HandleFunc("/campaigns/sync", api.HandleSyncCampaigns).Methods("POST")

	apiRouter.HandleFunc("/timeline", api.HandleGetTimeline).Methods("GET")

	apiRouter.HandleFunc("/templates", api.HandleGetTemplates).Methods("GET")
	apiRouter.HandleFunc("/templates", api.HandleCreateTemplate).Methods("POST")
	apiRouter.HandleFunc("/templates/{id:[0-9]+}", api.HandleDeleteTemplate).Methods("DELETE")

	// Stage 4 — all-in-one campaign UI (groups, profiles, pages, results)
	apiRouter.HandleFunc("/groups", api.HandleGetGroups).Methods("GET")
	apiRouter.HandleFunc("/groups", api.HandleCreateGroup).Methods("POST")
	apiRouter.HandleFunc("/profiles", api.HandleGetSendingProfiles).Methods("GET")
	apiRouter.HandleFunc("/pages", api.HandleGetPages).Methods("GET")
	apiRouter.HandleFunc("/campaigns/{id:[0-9]+}", api.HandleGetCampaignDetail).Methods("GET")
	apiRouter.HandleFunc("/lures", api.HandleGetLures).Methods("GET")
	apiRouter.HandleFunc("/lures", api.HandleCreateLure).Methods("POST")

	// Stage 6 - finishing touches
	apiRouter.HandleFunc("/captures/{session_id}/cookies", api.HandleDownloadCookies).Methods("GET")
	apiRouter.HandleFunc("/captures/{session_id}/replay", api.HandleReplaySession).Methods("POST")
	apiRouter.HandleFunc("/test-email", api.HandleSendTestEmail).Methods("POST")
	apiRouter.HandleFunc("/engagement/report", api.HandleEngagementReport).Methods("GET")
	apiRouter.HandleFunc("/dns/health", api.HandleDNSHealth).Methods("GET")
	apiRouter.HandleFunc("/phishlets/enable", api.HandleEnablePhishlet).Methods("POST")
	apiRouter.HandleFunc("/phishlets/{phishlet}/disable", api.HandleDisablePhishlet).Methods("POST")
	apiRouter.HandleFunc("/phishlets/state", api.HandleGetPhishletsState).Methods("GET")

	// Stage 5 — engagement metadata editable in dashboard
	apiRouter.HandleFunc("/engagement", api.HandleUpdateEngagement).Methods("PUT", "POST")
	apiRouter.HandleFunc("/engagement/clear", api.HandleClearEngagement).Methods("POST")

	if authH != nil {
		apiRouter.HandleFunc("/auth/whoami", authH.WhoAmI).Methods("GET")
		r.HandleFunc("/auth/google/login", authH.Login).Methods("GET")
		r.HandleFunc("/auth/google/callback", authH.Callback).Methods("GET")
		r.HandleFunc("/auth/logout", authH.Logout).Methods("GET", "POST")

		apiRouter.HandleFunc("/users", func(w http.ResponseWriter, req *http.Request) {
			emails, domains := authH.ListAllowed()
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"emails": emails, "domains": domains})
		}).Methods("GET")
		apiRouter.HandleFunc("/users", func(w http.ResponseWriter, req *http.Request) {
			var body struct{ Entry string `json:"entry"` }
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil || strings.TrimSpace(body.Entry) == "" {
				http.Error(w, `{"error":"entry required"}`, http.StatusBadRequest)
				return
			}
			if err := authH.AddAllowed(body.Entry); err != nil {
				http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"ok":true}`))
		}).Methods("POST")
		apiRouter.HandleFunc("/users/{entry}", func(w http.ResponseWriter, req *http.Request) {
			vars := mux.Vars(req)
			if err := authH.RemoveAllowed(vars["entry"]); err != nil {
				http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
				return
			}
			_, _ = w.Write([]byte(`{"ok":true}`))
		}).Methods("DELETE")
	}

	r.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	}).Methods("GET")

	r.HandleFunc("/ws", api.HandleWebSocket)

	r.PathPrefix("/").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(dashboardHTML))
	})

	var handler http.Handler = r
	if authH != nil && authH.Enabled() {
		handler = authH.Middleware(handler)
	}
	handler = middleware.RequestLogger(middleware.LocalhostOnly(handler))
	return handler
}

const dashboardHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>PhishLab &mdash; Engagement Dashboard</title>
<link rel="icon" type="image/svg+xml" href="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 64 64'%3E%3Crect width='64' height='64' rx='14' fill='%2300d4ff'/%3E%3Ctext x='50%25' y='56%25' text-anchor='middle' font-family='Arial' font-size='38' font-weight='700' fill='%230d0d11'%3EP%3C/text%3E%3C/svg%3E">
<script
  src="https://cdn.jsdelivr.net/npm/chart.js@4.4.0/dist/chart.umd.min.js"
  integrity="sha384-e6nUZLBkQ86NJ6TVVKAeSaK8jWa3NhkYWZFomE39AvDbQWeie9PlQqM3pmYW5d1g"
  crossorigin="anonymous"
  referrerpolicy="no-referrer"></script>
<style>
:root{
  --bg:#0d0d11;--bg2:#111116;--bg3:#16161d;
  --border:#1e1e2a;--border2:#252535;
  --cyan:#00d4ff;--cyan-dim:rgba(0,212,255,.08);--cyan-mid:rgba(0,212,255,.15);
  --green:#00e676;--green-dim:rgba(0,230,118,.08);
  --amber:#ffb300;--amber-dim:rgba(255,179,0,.08);
  --red:#ff1744;--red-dim:rgba(255,23,68,.08);
  --purple:#b388ff;--purple-dim:rgba(179,136,255,.08);
  --text:#e0e0ee;--muted:#6b6b80;--muted2:#4a4a5a;
  --focus:#7cdfff;
}
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;background:var(--bg);color:var(--text);min-height:100vh;font-size:14px}
.skip-link{position:absolute;left:-9999px;top:8px;background:var(--cyan);color:var(--bg);padding:8px 14px;border-radius:6px;font-weight:600;z-index:200}
.skip-link:focus{left:8px}
:focus-visible{outline:2px solid var(--focus);outline-offset:2px;border-radius:4px}
.topbar{display:flex;align-items:center;gap:16px;padding:0 24px;height:56px;background:var(--bg2);border-bottom:1px solid var(--border);position:sticky;top:0;z-index:100}
.logo{display:flex;align-items:center;gap:10px}
.logo-icon{width:30px;height:30px;background:linear-gradient(135deg,var(--cyan),#0055ff);border-radius:8px;display:flex;align-items:center;justify-content:center;font-size:16px}
.logo-text{font-size:15px;font-weight:700;letter-spacing:-.3px}
.tb-div{width:1px;height:24px;background:var(--border2)}
.eng-name{font-size:13px;font-weight:500;max-width:260px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.eng-id{font-size:11px;color:var(--muted);font-family:monospace}
.tb-right{margin-left:auto;display:flex;align-items:center;gap:12px}
.status-pill{display:flex;align-items:center;gap:6px;padding:4px 12px;border-radius:20px;font-size:11px;font-weight:600;letter-spacing:.5px;text-transform:uppercase}
.status-pill.active{background:var(--green-dim);border:1px solid rgba(0,230,118,.3);color:var(--green)}
.status-pill.off{background:var(--bg3);border:1px solid var(--border2);color:var(--muted)}
.pulse{width:6px;height:6px;border-radius:50%;background:var(--green);animation:pulse 2s infinite}
.pulse.off{background:var(--muted);animation:none}
@keyframes pulse{0%,100%{opacity:1}50%{opacity:.3}}
@media (prefers-reduced-motion: reduce){.pulse{animation:none}}
.sync-label{font-size:11px;color:var(--muted)}
.btn-sync{padding:5px 14px;background:var(--cyan-dim);border:1px solid rgba(0,212,255,.3);color:var(--cyan);border-radius:6px;cursor:pointer;font-size:12px;font-weight:500;transition:background .15s;font-family:inherit}
.btn-sync:hover,.btn-sync:focus-visible{background:var(--cyan-mid)}
.btn-sync[aria-busy="true"]{opacity:.6;cursor:wait}
.main{padding:20px 24px;max-width:1640px;margin:0 auto}
.err-banner{display:none;margin-bottom:16px;padding:12px 18px;border-radius:8px;background:var(--red-dim);border:1px solid rgba(255,23,68,.3);color:#ff6b6b;font-size:13px}
.metrics{display:grid;grid-template-columns:repeat(4,1fr);gap:12px;margin-bottom:20px}
.mcard{background:var(--bg2);border:1px solid var(--border);border-radius:10px;padding:16px 20px;display:flex;align-items:center;gap:16px}
.micon{width:42px;height:42px;border-radius:10px;display:flex;align-items:center;justify-content:center;font-size:20px;flex-shrink:0}
.micon.c{background:var(--cyan-dim)}.micon.g{background:var(--green-dim)}.micon.a{background:var(--amber-dim)}.micon.p{background:var(--purple-dim)}
.mval{font-size:30px;font-weight:700;line-height:1}
.mval.c{color:var(--cyan)}.mval.g{color:var(--green)}.mval.a{color:var(--amber)}.mval.p{color:var(--purple)}
.mlbl{font-size:11px;color:var(--muted);text-transform:uppercase;letter-spacing:.8px;margin-top:4px}
.skel{display:inline-block;height:1em;width:2.5ch;background:linear-gradient(90deg,var(--bg3) 0%,var(--border2) 50%,var(--bg3) 100%);background-size:200% 100%;animation:shimmer 1.4s infinite;border-radius:3px;color:transparent}
@keyframes shimmer{0%{background-position:-200% 0}100%{background-position:200% 0}}
@media (prefers-reduced-motion: reduce){.skel{animation:none;background:var(--bg3)}}
.cgrid{display:grid;grid-template-columns:1fr 320px;gap:16px;margin-bottom:20px}
.card{background:var(--bg2);border:1px solid var(--border);border-radius:10px;overflow:hidden}
.ch{display:flex;align-items:center;justify-content:space-between;padding:13px 18px;border-bottom:1px solid var(--border)}
.ct{font-size:11px;font-weight:700;text-transform:uppercase;letter-spacing:.9px;color:var(--muted)}
.cb{padding:16px 18px}
.chart-wrap{position:relative;height:230px}
.rpanel{display:flex;flex-direction:column;gap:12px}
.srow{display:flex;align-items:center;justify-content:space-between;padding:9px 0;border-bottom:1px solid var(--border)}
.srow:last-child{border-bottom:none}
.sname{display:flex;align-items:center;gap:8px;font-size:13px;font-weight:500}
.dot{width:8px;height:8px;border-radius:50%;flex-shrink:0}
.dot.active{background:var(--green);box-shadow:0 0 6px rgba(0,230,118,.5)}
.dot.inactive{background:var(--red)}.dot.unknown{background:var(--muted)}
.sbadge{font-size:11px;padding:2px 8px;border-radius:4px;font-weight:500}
.sbadge.active{color:var(--green);background:var(--green-dim)}
.sbadge.inactive{color:var(--red);background:var(--red-dim)}
.sbadge.unknown{color:var(--muted);background:var(--bg3)}
.irow{display:flex;justify-content:space-between;align-items:flex-start;padding:8px 0;border-bottom:1px solid var(--border);font-size:12px}
.irow:last-child{border-bottom:none}
.ilbl{color:var(--muted)}.ival{color:var(--text);font-weight:500;text-align:right;max-width:170px;word-break:break-word}
.ptags{display:flex;flex-wrap:wrap;gap:6px}
.ptag{padding:3px 10px;border-radius:4px;font-size:11px;font-weight:500;background:var(--bg3);border:1px solid var(--border2);color:var(--muted)}
.ptag.on{background:var(--cyan-dim);border-color:rgba(0,212,255,.3);color:var(--cyan)}
.pbar{height:3px;background:var(--border);border-radius:2px;margin-top:10px;overflow:hidden}
.pfill{height:100%;background:linear-gradient(90deg,var(--cyan),var(--green));border-radius:2px;transition:width .5s}
.tabs-wrap{background:var(--bg2);border:1px solid var(--border);border-radius:10px;overflow:hidden}
.tabs-hdr{display:flex;border-bottom:1px solid var(--border);overflow-x:auto}
.tbtn{padding:12px 20px;font-size:11px;font-weight:700;text-transform:uppercase;letter-spacing:.8px;color:var(--muted);cursor:pointer;border-bottom:2px solid transparent;white-space:nowrap;transition:color .15s,border-color .15s;background:transparent;border-top:none;border-left:none;border-right:none;font-family:inherit}
.tbtn:hover,.tbtn:focus-visible{color:var(--text)}
.tbtn[aria-selected="true"]{color:var(--cyan);border-bottom-color:var(--cyan)}
.tpane{display:none}.tpane.active{display:block}
.feed{max-height:440px;overflow-y:auto}
.tev{display:flex;align-items:flex-start;gap:12px;padding:11px 18px;border-bottom:1px solid var(--border);transition:background .1s}
.tev:hover{background:var(--bg3)}.tev:last-child{border-bottom:none}
.tico{width:32px;height:32px;border-radius:50%;display:flex;align-items:center;justify-content:center;font-size:14px;flex-shrink:0;margin-top:1px}
.tico.email_sent{background:var(--bg3);color:var(--muted)}
.tico.email_opened{background:rgba(0,212,255,.1);color:var(--cyan)}
.tico.link_clicked{background:var(--amber-dim);color:var(--amber)}
.tico.submitted_data{background:rgba(255,100,100,.1);color:#ff6464}
.tico.credential_captured{background:var(--red-dim);color:var(--red);border:1px solid rgba(255,23,68,.3)}
.tico.campaign_launched{background:var(--purple-dim);color:var(--purple)}
.tico.def{background:var(--bg3);color:var(--muted)}
.tbody2{flex:1;min-width:0}
.ttype{font-size:12px;font-weight:600;margin-bottom:3px}
.ttype.credential_captured{color:var(--red)}.ttype.link_clicked{color:var(--amber)}
.ttype.email_opened{color:var(--cyan)}.ttype.submitted_data{color:#ff6464}
.tmeta{font-size:11px;color:var(--muted);display:flex;gap:8px;flex-wrap:wrap;align-items:center}
.tsrc{padding:1px 6px;border-radius:3px;background:var(--bg3);border:1px solid var(--border2);color:var(--muted2);font-size:10px}
.ttime{margin-left:auto;font-size:11px;color:var(--muted2);white-space:nowrap;flex-shrink:0;padding-top:2px}
.dtable{width:100%;border-collapse:collapse}
.dtable th{padding:10px 14px;text-align:left;font-size:10px;font-weight:700;text-transform:uppercase;letter-spacing:1px;color:var(--muted);border-bottom:1px solid var(--border);background:var(--bg)}
.dtable td{padding:11px 14px;border-bottom:1px solid var(--border);font-size:13px;vertical-align:middle}
.dtable tr:last-child td{border-bottom:none}
.dtable tr:hover td{background:rgba(255,255,255,.015)}
.mono{font-family:'SF Mono','Fira Code',monospace;font-size:12px}
.code-c{color:var(--cyan)}.code-a{color:var(--amber)}
.mip{font-family:monospace;font-size:11px;color:var(--muted)}
.cbadge{display:inline-block;padding:2px 8px;border-radius:4px;font-size:10px;font-weight:700;text-transform:uppercase;letter-spacing:.5px}
.cbadge.launched{background:var(--green-dim);color:var(--green);border:1px solid rgba(0,230,118,.3)}
.cbadge.created{background:var(--cyan-dim);color:var(--cyan);border:1px solid rgba(0,212,255,.3)}
.cbadge.completed{background:var(--bg3);color:var(--muted);border:1px solid var(--border2)}
.cbadge.error{background:var(--red-dim);color:var(--red);border:1px solid rgba(255,23,68,.3)}
.cstat{display:inline-block;padding:3px 10px;border-radius:4px;font-size:10px;font-weight:700;text-transform:uppercase;letter-spacing:.5px;white-space:nowrap}
.cstat.visited{background:var(--bg3);color:var(--muted);border:1px solid var(--border2)}
.cstat.vulnerable{background:var(--amber-dim);color:var(--amber);border:1px solid rgba(255,179,0,.4)}
.cstat.exploitable{background:var(--red-dim);color:var(--red);border:1px solid rgba(255,23,68,.4);animation:pulse-red 2s ease-in-out infinite}
@keyframes pulse-red{0%,100%{box-shadow:0 0 0 0 rgba(255,23,68,.5)}50%{box-shadow:0 0 0 6px rgba(255,23,68,0)}}
@media(prefers-reduced-motion:reduce){.cstat.exploitable{animation:none}}
.recpane{background:var(--bg2);border:1px solid var(--border);border-radius:8px;padding:14px 16px;margin:10px 0}
.recpane h4{font-size:11px;text-transform:uppercase;letter-spacing:.8px;color:var(--cyan);margin-bottom:10px;font-weight:700}
.recpane ul{margin:0;padding-left:20px}
.recpane li{font-size:12px;color:var(--text);line-height:1.6;margin-bottom:8px}
.recpane li::marker{color:var(--cyan)}
.recpane .rec-meta{font-size:11px;color:var(--muted);margin-bottom:10px;padding-bottom:8px;border-bottom:1px solid var(--border)}
.expandbtn{background:transparent;border:1px solid var(--border2);color:var(--cyan);padding:3px 10px;border-radius:4px;font-size:11px;cursor:pointer;font-family:inherit}
.expandbtn:hover,.expandbtn:focus-visible{background:var(--cyan-dim);border-color:var(--cyan)}
.detailrow{background:var(--bg3) !important}
.detailrow td{padding:0 !important}
.detailrow.hide{display:none}
.tbar{display:flex;gap:10px;padding:12px 16px;border-bottom:1px solid var(--border);background:var(--bg2);flex-wrap:wrap}
.tbar-btn{padding:7px 14px;font-size:12px;font-weight:600;border-radius:6px;cursor:pointer;font-family:inherit;background:var(--bg3);color:var(--text);border:1px solid var(--border2);transition:all .15s}
.tbar-btn:hover,.tbar-btn:focus-visible{background:var(--bg);border-color:var(--cyan);color:var(--cyan)}
.tbar-btn.primary{background:var(--cyan-dim);border-color:rgba(0,212,255,.4);color:var(--cyan)}
.tbar-btn.primary:hover{background:var(--cyan);color:var(--bg)}
.modal{position:fixed;inset:0;background:rgba(0,0,0,.7);display:none;align-items:center;justify-content:center;z-index:300;padding:20px}
.modal.show{display:flex}
.modal-card{background:var(--bg2);border:1px solid var(--border);border-radius:10px;width:100%;max-width:560px;max-height:90vh;overflow-y:auto;box-shadow:0 20px 60px rgba(0,0,0,.5)}
.modal-h{display:flex;justify-content:space-between;align-items:center;padding:16px 22px;border-bottom:1px solid var(--border)}
.modal-h h3{font-size:14px;font-weight:700;text-transform:uppercase;letter-spacing:.8px;color:var(--cyan)}
.modal-x{background:transparent;border:none;color:var(--muted);font-size:22px;cursor:pointer;padding:0;width:30px;height:30px;border-radius:4px}
.modal-x:hover{color:var(--text);background:var(--bg3)}
.modal-b{padding:18px 22px}
.modal-f{padding:14px 22px;border-top:1px solid var(--border);display:flex;justify-content:flex-end;gap:10px}
.fld{margin-bottom:14px}
.fld label{display:block;font-size:11px;font-weight:600;text-transform:uppercase;letter-spacing:.6px;color:var(--muted);margin-bottom:6px}
.fld input,.fld select,.fld textarea{width:100%;padding:8px 12px;background:var(--bg);border:1px solid var(--border2);border-radius:5px;color:var(--text);font-size:13px;font-family:inherit;transition:border-color .15s}
.fld input:focus,.fld select:focus,.fld textarea:focus{outline:none;border-color:var(--cyan);box-shadow:0 0 0 2px var(--cyan-dim)}
.fld textarea{min-height:80px;resize:vertical;font-family:'SF Mono',monospace;font-size:12px}
.fld .hint{font-size:11px;color:var(--muted2);margin-top:4px}
.engcard{background:var(--bg2);border:1px solid var(--border);border-radius:10px;padding:18px 22px;margin-bottom:18px}
.engcard-h{display:flex;justify-content:space-between;align-items:center;margin-bottom:14px}
.engcard-h h3{font-size:13px;font-weight:700;text-transform:uppercase;letter-spacing:.8px;color:var(--cyan)}
.engcard-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(180px,1fr));gap:14px}
.engcard-grid .lbl{font-size:10px;text-transform:uppercase;letter-spacing:.6px;color:var(--muted);margin-bottom:3px}
.engcard-grid .val{font-size:13px;color:var(--text);font-weight:500}
.engcard-grid .val.dim{color:var(--muted2);font-style:italic}
.lurerow{display:flex;align-items:center;gap:10px;padding:10px 12px;background:var(--bg);border:1px solid var(--border2);border-radius:6px;margin-bottom:8px;flex-wrap:wrap}
.lurerow:last-child{margin-bottom:0}
.lure-ph{font-size:10px;font-weight:700;text-transform:uppercase;letter-spacing:.6px;color:var(--cyan);background:var(--cyan-dim);padding:3px 8px;border-radius:4px;border:1px solid rgba(0,212,255,.3)}
.lure-url{flex:1;font-family:'SF Mono',monospace;font-size:12px;color:var(--text);overflow:hidden;text-overflow:ellipsis;white-space:nowrap;min-width:0}
.lure-paused{font-size:10px;color:var(--amber);background:var(--amber-dim);padding:2px 6px;border-radius:3px;border:1px solid rgba(255,179,0,.3)}
.copybtn{background:transparent;border:1px solid var(--border2);color:var(--cyan);padding:4px 10px;border-radius:4px;font-size:11px;cursor:pointer;font-family:inherit;white-space:nowrap}
.copybtn:hover,.copybtn:focus-visible{background:var(--cyan-dim);border-color:var(--cyan)}
.copybtn.ok{background:var(--green-dim);color:var(--green);border-color:rgba(0,230,118,.4)}
.empty{text-align:center;padding:48px 20px;color:var(--muted);font-size:13px}
.empty-ico{font-size:32px;margin-bottom:12px;opacity:.35}
.btn-del{padding:3px 10px;background:var(--red-dim);border:1px solid rgba(255,23,68,.3);color:var(--red);border-radius:4px;cursor:pointer;font-size:11px;font-family:inherit}
.btn-del:hover,.btn-del:focus-visible{background:rgba(255,23,68,.2)}
.sr-only{position:absolute;width:1px;height:1px;padding:0;margin:-1px;overflow:hidden;clip:rect(0,0,0,0);white-space:nowrap;border:0}
::-webkit-scrollbar{width:5px;height:5px}
::-webkit-scrollbar-track{background:var(--bg2)}
::-webkit-scrollbar-thumb{background:var(--border2);border-radius:3px}
@media(max-width:1100px){.metrics{grid-template-columns:repeat(2,1fr)}.cgrid{grid-template-columns:1fr}}
@media(max-width:760px){.eng-name{max-width:140px}.tb-right .sync-label{display:none}.main{padding:14px}.tbtn{padding:10px 14px}.chart-wrap{height:200px}}
@media(max-width:480px){.metrics{grid-template-columns:1fr 1fr}.mval{font-size:24px}.topbar{padding:0 12px;gap:10px}.logo-text{display:none}}
</style>
</head>
<body>
<a class="skip-link" href="#main">Skip to main content</a>
<header class="topbar" role="banner">
  <div class="logo">
    <div class="logo-icon" aria-hidden="true">&#x1F41F;</div>
    <span class="logo-text">PhishLab</span>
  </div>
  <div class="tb-div" aria-hidden="true"></div>
  <div>
    <div class="eng-name" id="eng-name">Connecting&hellip;</div>
    <div class="eng-id" id="eng-id" aria-label="Engagement ID"></div>
  </div>
  <div class="tb-right">
    <div class="status-pill off" id="spill" role="status" aria-live="polite" aria-label="Engagement status"><div class="pulse off" id="spulse" aria-hidden="true"></div><span id="stext">&#x2014;</span></div>
    <span class="sync-label" id="sync-lbl"></span>
    <button class="btn-sync" id="btn-sync" type="button" onclick="sync()" aria-label="Sync campaign data from Gophish">&#x21BB;&nbsp;Sync</button>
    <button class="btn-sync" type="button" onclick="openUsersModal()" aria-label="Manage dashboard users">&#x1F465;&nbsp;Users</button>
    <a class="btn-sync" href="/auth/logout" aria-label="Sign out" style="text-decoration:none;display:inline-flex;align-items:center">&#x1F6AA;&nbsp;Logout</a>
  </div>
</header>
<main class="main" id="main">
  <div class="err-banner" id="err" role="alert"></div>
  <section class="metrics" aria-label="Key metrics">
    <div class="mcard"><div class="micon c" aria-hidden="true">&#x1F511;</div><div><div class="mval c" id="m0"><span class="skel">000</span></div><div class="mlbl">Credentials Captured</div></div></div>
    <div class="mcard"><div class="micon g" aria-hidden="true">&#x1F4E7;</div><div><div class="mval g" id="m1"><span class="skel">000</span></div><div class="mlbl">Campaigns</div></div></div>
    <div class="mcard"><div class="micon a" aria-hidden="true">&#x26A1;</div><div><div class="mval a" id="m2"><span class="skel">000</span></div><div class="mlbl">Timeline Events</div></div></div>
    <div class="mcard"><div class="micon p" aria-hidden="true">&#x2699;</div><div><div class="mval p" id="m3"><span class="skel">0/0</span></div><div class="mlbl">Services Online</div></div></div>
  </section>
  <section class="cgrid" aria-label="Engagement overview">
    <div class="card">
      <div class="ch"><span class="ct">Campaign Funnel</span><span style="font-size:11px;color:var(--muted)">Across all campaigns</span></div>
      <div class="cb"><div class="chart-wrap"><canvas id="fchart" aria-label="Funnel chart of campaign progression"></canvas></div></div>
    </div>
    <div class="rpanel">
      <div class="card">
        <div class="ch"><span class="ct">Services</span></div>
        <div class="cb" style="padding-top:4px;padding-bottom:4px" id="svc-list"></div>
      </div>
      <div class="card">
        <div class="ch"><span class="ct">Engagement</span></div>
        <div class="cb" style="padding-top:4px;padding-bottom:4px" id="eng-info"></div>
      </div>
      <div class="card">
        <div class="ch"><span class="ct">Phishlets</span></div>
        <div class="cb"><div class="ptags" id="ptags"></div></div>
      </div>
    </div>
  </section>
  <section class="engcard" aria-label="Engagement metadata">
    <div class="engcard-h">
      <h3>Engagement</h3>
      <div style="display:flex;gap:8px;flex-wrap:wrap">
        <button class="tbar-btn" type="button" onclick="openEngForm()">&#x270F; Edit / New</button>
        <button class="tbar-btn" type="button" onclick="downloadReport()" title="Download engagement report (Markdown)">&#x1F4C4; Report</button>
        <button class="tbar-btn" type="button" onclick="clearEngagement()" style="border-color:rgba(255,23,68,.4);color:var(--red)">&#x1F5D1; Clear Data</button>
      </div>
    </div>
    <div class="engcard-grid" id="eng-grid">
      <div><div class="lbl">Loading&#x2026;</div></div>
    </div>
    <div style="margin-top:8px;font-size:11px;color:var(--muted2)">Tip: change the Engagement ID in the form to create a new engagement record.</div>
  </section>
  <section class="engcard" aria-label="Active phishing lures">
    <div class="engcard-h">
      <h3>Active Lures</h3>
      <span style="font-size:11px;color:var(--muted2)">Phishing entry URLs &mdash; paste into Gophish "Phish URL" field</span>
    </div>
    <div id="lures-list"><div class="lbl">Loading&#x2026;</div></div>
  </section>
  <section class="engcard" aria-label="DNS deliverability health">
    <div class="engcard-h">
      <h3>DNS Health</h3>
      <button class="tbar-btn" type="button" onclick="loadDNS()">&#x21BB; Recheck</button>
    </div>
    <div id="dns-grid" class="engcard-grid"><div><div class="lbl">Checking&#x2026;</div></div></div>
  </section>
  <section class="tabs-wrap" aria-label="Engagement detail tabs">
    <div class="tabs-hdr" role="tablist" aria-label="Engagement views">
      <button class="tbtn" type="button" role="tab" id="tab-btn-timeline"    aria-selected="true"  aria-controls="tab-timeline"    data-tab="timeline"    tabindex="0">&#x23F1; Timeline</button>
      <button class="tbtn" type="button" role="tab" id="tab-btn-credentials" aria-selected="false" aria-controls="tab-credentials" data-tab="credentials" tabindex="-1">&#x1F511; Credentials</button>
      <button class="tbtn" type="button" role="tab" id="tab-btn-campaigns"   aria-selected="false" aria-controls="tab-campaigns"   data-tab="campaigns"   tabindex="-1">&#x1F4CB; Campaigns</button>
      <button class="tbtn" type="button" role="tab" id="tab-btn-templates"   aria-selected="false" aria-controls="tab-templates"   data-tab="templates"   tabindex="-1">&#x2709; Templates</button>
      <button class="tbtn" type="button" role="tab" id="tab-btn-phishlets"   aria-selected="false" aria-controls="tab-phishlets"   data-tab="phishlets"   tabindex="-1">&#x1F3A3; Phishlets</button>
    </div>
    <div class="tpane active" id="tab-timeline"    role="tabpanel" aria-labelledby="tab-btn-timeline">
      <div class="feed" id="feed"><div class="empty"><div class="empty-ico" aria-hidden="true">&#x23F3;</div>No events yet</div></div>
    </div>
    <div class="tpane" id="tab-credentials" role="tabpanel" aria-labelledby="tab-btn-credentials" hidden>
      <div style="overflow-x:auto">
        <table class="dtable">
          <thead><tr><th scope="col">Status</th><th scope="col">Captured</th><th scope="col">Phishlet</th><th scope="col">Username</th><th scope="col">Password</th><th scope="col">Cookies</th><th scope="col">Source IP</th><th scope="col"><span class="sr-only">Recommendations</span></th></tr></thead>
          <tbody id="creds-body"></tbody>
        </table>
      </div>
    </div>
    <div class="tpane" id="tab-campaigns" role="tabpanel" aria-labelledby="tab-btn-campaigns" hidden>
      <div class="tbar">
        <button class="tbar-btn primary" type="button" onclick="openCampForm()">&#x2795; Launch Campaign</button>
        <button class="tbar-btn" type="button" id="btn-sync" onclick="sync()">&#x21BB; Sync from Gophish</button>
      </div>
      <div style="overflow-x:auto">
        <table class="dtable">
          <thead><tr><th scope="col">Campaign</th><th scope="col">Status</th><th scope="col">Targets</th><th scope="col">Template</th><th scope="col">Phish URL</th><th scope="col">Launched</th><th scope="col"><span class="sr-only">Results</span></th></tr></thead>
          <tbody id="camp-body"></tbody>
        </table>
      </div>
    </div>
    <div class="tpane" id="tab-templates" role="tabpanel" aria-labelledby="tab-btn-templates" hidden>
      <div style="overflow-x:auto">
        <table class="dtable">
          <thead><tr><th scope="col">ID</th><th scope="col">Name</th><th scope="col">Subject</th><th scope="col"><span class="sr-only">Actions</span></th></tr></thead>
          <tbody id="tmpl-body"></tbody>
        </table>
      </div>
    </div>
    <div class="tpane" id="tab-phishlets" role="tabpanel" aria-labelledby="tab-btn-phishlets" hidden>
      <div class="tbar">
        <button class="tbar-btn" type="button" onclick="loadPhishlets()">&#x21BB; Refresh</button>
        <span style="font-size:11px;color:var(--muted2);align-self:center">Toggle phishlets and create lures without dropping to the evilginx CLI.</span>
      </div>
      <div style="overflow-x:auto">
        <table class="dtable">
          <thead><tr><th scope="col">Phishlet</th><th scope="col">Status</th><th scope="col">Hostname</th><th scope="col">Lures</th><th scope="col"><span class="sr-only">Actions</span></th></tr></thead>
          <tbody id="phishlets-body"></tbody>
        </table>
      </div>
    </div>
  </section>
</main>
<script>
var chart=null;
function esc(s){if(!s)return'';var d=document.createElement('div');d.textContent=String(s);return d.innerHTML;}
function fmt(ts){if(!ts||ts==='0001-01-01T00:00:00Z')return'—';var d=new Date(ts);return d.toLocaleString([],{month:'short',day:'numeric',hour:'2-digit',minute:'2-digit'});}
function fmtFull(ts){if(!ts||ts==='0001-01-01T00:00:00Z')return'—';return new Date(ts).toLocaleString();}

var tabBtns=Array.prototype.slice.call(document.querySelectorAll('[role="tab"]'));
function activateTab(btn){
  tabBtns.forEach(function(x){
    var on=x===btn;
    x.setAttribute('aria-selected',on?'true':'false');
    x.setAttribute('tabindex',on?'0':'-1');
    var pane=document.getElementById(x.getAttribute('aria-controls'));
    if(pane){pane.classList.toggle('active',on);if(on){pane.removeAttribute('hidden');}else{pane.setAttribute('hidden','');}}
  });
  if(btn.dataset.tab==='templates')loadTemplates();
  if(btn.dataset.tab==='phishlets')loadPhishlets();
}
tabBtns.forEach(function(b){
  b.addEventListener('click',function(){activateTab(b);});
  b.addEventListener('keydown',function(e){
    var i=tabBtns.indexOf(b);
    if(e.key==='ArrowRight'){e.preventDefault();var n=tabBtns[(i+1)%tabBtns.length];n.focus();activateTab(n);}
    else if(e.key==='ArrowLeft'){e.preventDefault();var p=tabBtns[(i-1+tabBtns.length)%tabBtns.length];p.focus();activateTab(p);}
    else if(e.key==='Home'){e.preventDefault();tabBtns[0].focus();activateTab(tabBtns[0]);}
    else if(e.key==='End'){e.preventDefault();var l=tabBtns[tabBtns.length-1];l.focus();activateTab(l);}
  });
});

function initChart(){
  if(typeof Chart==='undefined'){return;}
  var ctx=document.getElementById('fchart').getContext('2d');
  chart=new Chart(ctx,{
    type:'bar',
    data:{
      labels:['Sent','Opened','Clicked','Submitted','Captured'],
      datasets:[{
        data:[0,0,0,0,0],
        backgroundColor:['rgba(100,100,130,.5)','rgba(0,212,255,.5)','rgba(255,179,0,.5)','rgba(255,100,100,.5)','rgba(255,23,68,.65)'],
        borderColor:['rgba(100,100,130,.9)','rgba(0,212,255,.9)','rgba(255,179,0,.9)','rgba(255,100,100,.9)','rgba(255,23,68,.9)'],
        borderWidth:1,borderRadius:4,borderSkipped:false
      }]
    },
    options:{
      indexAxis:'y',responsive:true,maintainAspectRatio:false,
      plugins:{legend:{display:false},tooltip:{callbacks:{label:function(c){return'  '+c.parsed.x+' targets';}}}},
      scales:{
        x:{ticks:{color:'#6b6b80',font:{size:11}},grid:{color:'rgba(30,30,42,.9)'},border:{color:'#1e1e2a'}},
        y:{ticks:{color:'#9090a8',font:{size:12,weight:'600'}},grid:{display:false},border:{display:false}}
      }
    }
  });
}

function updateChart(timeline,credCount){
  if(!chart)return;
  var c={email_sent:0,email_opened:0,link_clicked:0,submitted_data:0};
  (timeline||[]).forEach(function(e){if(c[e.event_type]!==undefined)c[e.event_type]++;});
  chart.data.datasets[0].data=[c.email_sent,c.email_opened,c.link_clicked,c.submitted_data,credCount||0];
  chart.update();
}

function render(d){
  var eng=d.engagement;
  var pill=document.getElementById('spill'),pulse=document.getElementById('spulse'),st=document.getElementById('stext');
  if(eng&&eng.status==='active'){pill.className='status-pill active';pulse.className='pulse';st.textContent='Active';}
  else{pill.className='status-pill off';pulse.className='pulse off';st.textContent=eng?eng.status:'No Engagement';}
  document.getElementById('eng-name').textContent=eng?eng.name:'No engagement loaded';
  document.getElementById('eng-id').textContent=eng?eng.id:'';
  renderEng(eng);
  document.getElementById('m0').textContent=d.credential_count||0;
  document.getElementById('m1').textContent=d.campaign_count||0;
  document.getElementById('m2').textContent=(d.timeline||[]).length;
  var up=(d.services||[]).filter(function(s){return s.status==='active';}).length;
  document.getElementById('m3').textContent=up+'/'+(d.services||[]).length;

  var sh='';
  (d.services||[]).forEach(function(s){
    var c=s.status==='active'?'active':(s.status==='inactive'?'inactive':'unknown');
    sh+='<div class="srow"><span class="sname"><span class="dot '+c+'" aria-hidden="true"></span>'+esc(s.name)+'</span><span class="sbadge '+c+'">'+esc(s.status)+'</span></div>';
  });
  document.getElementById('svc-list').innerHTML=sh||'<div style="padding:10px 0;color:var(--muted);font-size:12px">No services</div>';

  if(eng){
    var progress=0;
    if(eng.start_date&&eng.end_date){
      var s2=new Date(eng.start_date).getTime(),e2=new Date(eng.end_date).getTime(),now=Date.now();
      progress=Math.min(100,Math.max(0,Math.round((now-s2)/(e2-s2)*100)));
    }
    document.getElementById('eng-info').innerHTML=
      '<div class="irow"><span class="ilbl">Client</span><span class="ival">'+esc(eng.client)+'</span></div>'+
      '<div class="irow"><span class="ilbl">Operator</span><span class="ival">'+esc(eng.operator)+'</span></div>'+
      '<div class="irow"><span class="ilbl">Domain</span><span class="ival">'+esc(eng.domain)+'</span></div>'+
      '<div class="irow"><span class="ilbl">Window</span><span class="ival">'+esc(eng.start_date)+' → '+esc(eng.end_date)+'</span></div>'+
      '<div class="pbar" role="progressbar" aria-label="Engagement window progress" aria-valuenow="'+progress+'" aria-valuemin="0" aria-valuemax="100"><div class="pfill" style="width:'+progress+'%"></div></div>';
  }

  var pt='';
  (d.phishlets||[]).forEach(function(p){pt+='<span class="ptag'+(p.enabled?' on':'')+'">'+esc(p.name)+'</span>';});
  document.getElementById('ptags').innerHTML=pt||'<span style="font-size:12px;color:var(--muted)">None loaded</span>';

  renderTimeline(d.timeline||[]);
  renderCreds(d.credentials||[]);
  renderCamps(d.campaigns||[]);
  updateChart(d.timeline,d.credential_count);
  document.getElementById('sync-lbl').textContent='Synced '+new Date().toLocaleTimeString();
}

var eIcons={email_sent:'✉',email_opened:'👁',link_clicked:'🔗',submitted_data:'📝',credential_captured:'🔑',campaign_launched:'🚀',target_status:'🎯',email_reported:'⚠'};
var eLabels={email_sent:'Email Sent',email_opened:'Email Opened',link_clicked:'Link Clicked',submitted_data:'Data Submitted',credential_captured:'Credential Captured',campaign_launched:'Campaign Launched',target_status:'Target Update',email_reported:'Email Reported'};

function renderTimeline(evts){
  if(!evts.length){document.getElementById('feed').innerHTML='<div class="empty"><div class="empty-ico" aria-hidden="true">⏳</div>No events yet — run a campaign to see the live feed.</div>';return;}
  var html='';
  evts.forEach(function(e){
    var cls=e.event_type||'def';
    var icon=eIcons[e.event_type]||'•';
    var lbl=eLabels[e.event_type]||e.event_type;
    var meta='';
    if(e.email)meta+='<span>'+esc(e.email)+'</span>';
    if(e.remote_addr)meta+='<span>'+esc(e.remote_addr)+'</span>';
    if(e.detail)meta+='<span>'+esc(e.detail)+'</span>';
    html+='<div class="tev"><div class="tico '+cls+'" aria-hidden="true">'+icon+'</div><div class="tbody2"><div class="ttype '+cls+'">'+lbl+'</div><div class="tmeta"><span class="tsrc">'+esc(e.source)+'</span>'+meta+'</div></div><time class="ttime" datetime="'+esc(e.timestamp)+'">'+fmt(e.timestamp)+'</time></div>';
  });
  document.getElementById('feed').innerHTML=html;
}

function renderCreds(creds){
  if(!creds.length){document.getElementById('creds-body').innerHTML='<tr><td colspan="8"><div class="empty"><div class="empty-ico" aria-hidden="true">&#x1F511;</div>No credentials captured yet</div></td></tr>';return;}
  var rows='';
  creds.forEach(function(c){
    var st=(c.status||'Visited').toLowerCase();
    var stLabel=c.status||'Visited';
    var cookieCount=c.cookie_count||0;
    var pwd=c.password?esc(c.password):'<span style="color:var(--muted2)">&#8212;</span>';
    var usr=c.username?esc(c.username):'<span style="color:var(--muted2)">(no username captured)</span>';
    var recsHtml='';
    if(c.recommendations&&c.recommendations.length){
      var lis=c.recommendations.map(function(r){return '<li>'+esc(r)+'</li>';}).join('');
      recsHtml='<div class="recpane"><div class="rec-meta">Session id <code>'+esc(c.session_id)+'</code> &middot; '+cookieCount+' cookie(s) captured &middot; UA: '+esc(c.user_agent||'').substring(0,100)+'</div><h4>Recommended Remediation</h4><ul>'+lis+'</ul></div>';
    }
    rows+='<tr data-cred="'+c.id+'">'
       +'<td><span class="cstat '+st+'">'+esc(stLabel)+'</span></td>'
       +'<td style="white-space:nowrap;color:var(--muted)">'+fmtFull(c.captured_at)+'</td>'
       +'<td><span class="ptag on">'+esc(c.phishlet)+'</span></td>'
       +'<td><span class="mono code-c">'+usr+'</span></td>'
       +'<td><span class="mono code-a">'+pwd+'</span></td>'
       +'<td style="text-align:center"><span class="mono">'+cookieCount+'</span></td>'
       +'<td><span class="mip">'+esc(c.remote_addr)+'</span></td>'
       +'<td style="white-space:nowrap"><button class="expandbtn" type="button" aria-expanded="false" aria-controls="rec-'+c.id+'" onclick="toggleRec('+c.id+')">Why?</button>'
       +(cookieCount?' <a class="expandbtn" href="/api/captures/'+esc(c.session_id)+'/cookies" download title="Cookie-Editor JSON">&#x2B07; Cookies</a>':'')
       +(c.exploitable?' <button class="expandbtn" type="button" onclick="replaySession(\''+esc(c.session_id)+'\')" title="Replay session against Microsoft to validate CSOC detections" style="border-color:rgba(255,23,68,.3);color:var(--red)">&#x26A1; Replay</button>':'')
       +'</td>'
       +'</tr>'
       +'<tr class="detailrow hide" id="rec-'+c.id+'"><td colspan="8">'+recsHtml+'</td></tr>';
  });
  document.getElementById('creds-body').innerHTML=rows;
}
function toggleRec(id){
  var row=document.getElementById('rec-'+id);
  var btn=document.querySelector('tr[data-cred="'+id+'"] .expandbtn');
  if(!row||!btn)return;
  var hidden=row.classList.toggle('hide');
  btn.setAttribute('aria-expanded',hidden?'false':'true');
  btn.textContent=hidden?'Why?':'Hide';
}

function renderCamps(camps){
  if(!camps.length){document.getElementById('camp-body').innerHTML='<tr><td colspan="6"><div class="empty"><div class="empty-ico" aria-hidden="true">📋</div>No campaigns yet</div></td></tr>';return;}
  var rows='';
  camps.forEach(function(c){
    rows+='<tr><td><strong>'+esc(c.name)+'</strong></td><td><span class="cbadge '+esc(c.status)+'">'+esc(c.status)+'</span></td><td>'+c.target_count+'</td><td>'+esc(c.template_name)+'</td><td style="font-family:monospace;font-size:11px;color:var(--muted)">'+esc(c.phish_url)+'</td><td style="white-space:nowrap;color:var(--muted)">'+fmt(c.launched_at)+'</td></tr>';
  });
  document.getElementById('camp-body').innerHTML=rows;
}

function loadTemplates(){
  fetch('/api/templates').then(function(r){return r.json();}).then(function(tmps){
    if(!tmps||!tmps.length){document.getElementById('tmpl-body').innerHTML='<tr><td colspan="4"><div class="empty"><div class="empty-ico" aria-hidden="true">✉</div>No templates</div></td></tr>';return;}
    var rows='';
    tmps.forEach(function(t){rows+='<tr><td style="color:var(--muted);font-family:monospace">'+t.id+'</td><td><strong>'+esc(t.name)+'</strong></td><td style="color:var(--muted)">'+esc(t.subject||'')+'</td><td><button class="btn-del" type="button" aria-label="Delete template '+esc(t.name)+'" onclick="delTmpl('+t.id+')">Delete</button></td></tr>';});
    document.getElementById('tmpl-body').innerHTML=rows;
  }).catch(function(){});
}

function delTmpl(id){if(!confirm('Delete template #'+id+'?'))return;fetch('/api/templates/'+id,{method:'DELETE'}).then(loadTemplates);}
function sync(){var b=document.getElementById('btn-sync');b.setAttribute('aria-busy','true');fetch('/api/campaigns/sync',{method:'POST'}).then(load).catch(function(){}).finally(function(){b.removeAttribute('aria-busy');});}

function load(){
  fetch('/api/dashboard')
    .then(function(r){if(!r.ok)throw new Error('HTTP '+r.status);return r.json();})
    .then(function(d){render(d);document.getElementById('err').style.display='none';})
    .catch(function(e){var b=document.getElementById('err');b.textContent='Dashboard API error: '+e.message;b.style.display='block';});
}

function connectWS(){
  var proto=location.protocol==='https:'?'wss:':'ws:';
  var ws=new WebSocket(proto+'//'+location.host+'/ws');
  ws.onmessage=function(){load();};
  ws.onclose=function(){setTimeout(connectWS,3000);};
}

// ===== Stage 4 + 5: campaign create / group create / engagement edit =====
function showModal(id){document.getElementById(id).classList.add('show');}
function hideModal(id){document.getElementById(id).classList.remove('show');}
function fillSelect(id,items,key){
  var s=document.getElementById(id);s.innerHTML='';
  if(!items||!items.length){s.innerHTML='<option value="">(none — create one first)</option>';return;}
  items.forEach(function(it){var o=document.createElement('option');o.value=it[key];o.textContent=it[key];s.appendChild(o);});
}
var campRecipMode='paste';
function setRecipMode(m){
  campRecipMode=m;
  document.getElementById('camp-recip-paste').style.display=m==='paste'?'block':'none';
  document.getElementById('camp-recip-group').style.display=m==='group'?'block':'none';
  ['camp-mode-paste','camp-mode-group'].forEach(function(id){
    var b=document.getElementById(id); if(!b)return;
    var on=id==='camp-mode-'+m;
    b.classList.toggle('primary',on);
  });
}
function openCampForm(){
  Promise.all([
    fetch('/api/templates').then(function(r){return r.json();}),
    fetch('/api/groups').then(function(r){return r.json();}),
    fetch('/api/profiles').then(function(r){return r.json();}),
    fetch('/api/pages').then(function(r){return r.json();})
  ]).then(function(arr){
    var tmpls=arr[0]||[], groups=(arr[1]||[]).filter(function(g){return !/^test\b/i.test(g.name);}), profiles=arr[2]||[], pages=arr[3]||[];
    fillSelect('camp-tmpl',tmpls,'name');
    fillSelect('camp-group',groups,'name');
    fillSelect('camp-smtp',profiles,'name');
    fillSelect('camp-page',pages,'name');
    var hint=document.getElementById('camp-tmpl-hint');
    if(!tmpls.length){hint.innerHTML='<span style="color:var(--amber)">No templates found.</span> Create one on the Templates tab before launching.';}
    else hint.textContent='';
    if(!profiles.length){alert('No sending profile is configured. Add SMTP credentials in Gophish first.');}
    setRecipMode('paste');
    showModal('modal-camp');
  });
}
function parsePastedRecipients(raw){
  return raw.split('\n').map(function(line){
    line=line.trim(); if(!line)return null;
    var p=line.split(',').map(function(s){return s.trim();});
    if(!p[0]||p[0].indexOf('@')<0)return null;
    return {email:p[0],first_name:p[1]||'',last_name:p[2]||'',position:p[3]||''};
  }).filter(Boolean);
}
function submitCamp(){
  var name=document.getElementById('camp-name').value.trim();
  if(!name){
    var ts=new Date().toISOString().replace(/[:T]/g,'-').slice(0,16);
    name='Campaign '+ts;
  }
  var tmpl=document.getElementById('camp-tmpl').value;
  var smtp=document.getElementById('camp-smtp').value;
  var page=document.getElementById('camp-page').value;
  var url=document.getElementById('camp-url').value.trim();
  if(!tmpl||!smtp||!page||!url){alert('Template, sending profile, landing page, and lure URL are required.');return;}

  // Resolve recipients to a Gophish group
  var groupPromise;
  if(campRecipMode==='group'){
    var g=document.getElementById('camp-group').value;
    if(!g){alert('Select a group, or switch to "Paste emails".');return;}
    groupPromise=Promise.resolve(g);
  } else {
    var raw=document.getElementById('camp-emails').value;
    var targets=parsePastedRecipients(raw);
    if(!targets.length){alert('Paste at least one email address.');return;}
    var groupName=name+' (recipients)';
    groupPromise=fetch('/api/groups',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({name:groupName,targets:targets})})
      .then(function(r){if(!r.ok)return r.json().then(function(j){throw new Error('group: '+(j.error||'HTTP '+r.status));});return r.json();})
      .then(function(){return groupName;});
  }
  groupPromise.then(function(groupName){
    var body={
      name:name,
      template:{name:tmpl},
      page:{name:page},
      smtp:{name:smtp},
      groups:[{name:groupName}],
      url:url,
      launch_date:new Date().toISOString()
    };
    return fetch('/api/campaigns',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(body)});
  }).then(function(r){
    if(!r.ok)return r.json().then(function(j){throw new Error(j.error||'HTTP '+r.status);});
    return r.json();
  }).then(function(){hideModal('modal-camp');load();}).catch(function(e){alert('Launch failed: '+e.message);});
}
function openGroupForm(){showModal('modal-group');}
function submitGroup(){
  var name=document.getElementById('group-name').value.trim();
  var raw=document.getElementById('group-targets').value;
  var targets=raw.split('\n').map(function(line){
    var p=line.split(',').map(function(s){return s.trim();});
    if(!p[0]||p[0].indexOf('@')<0)return null;
    return {email:p[0],first_name:p[1]||'',last_name:p[2]||'',position:p[3]||''};
  }).filter(Boolean);
  if(!name||!targets.length){alert('Name + at least 1 target line (email,first,last,position) required');return;}
  fetch('/api/groups',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({name:name,targets:targets})})
    .then(function(r){if(!r.ok)return r.json().then(function(j){throw new Error(j.error||'HTTP '+r.status);});return r.json();})
    .then(function(){hideModal('modal-group');alert('Group "'+name+'" created with '+targets.length+' targets');})
    .catch(function(e){alert('Create failed: '+e.message);});
}
function downloadReport(){window.location='/api/engagement/report';}
function replaySession(sid){
  var msg = 'Attack-emulation: replay captured cookies from THIS dashboard host against outlook.office.com to test CSOC detection on the target tenant.\n\n'
          + 'Session: '+sid+'\n\n'
          + 'This sends authenticated traffic from this server\'s public IP. Pre-warn your CSOC team and share the source IP so they can correlate the alert.\n\n'
          + 'Continue?';
  if(!confirm(msg))return;
  var btn=document.querySelector('button[onclick*="replaySession(\''+sid+'\')"]');
  if(btn){btn.disabled=true;btn.textContent='⚡ Running…';}
  fetch('/api/captures/'+encodeURIComponent(sid)+'/replay',{method:'POST'})
    .then(function(r){return r.json();})
    .then(function(d){
      var summary=(d.success?'✅ SUCCESS - cookies authenticated':'❌ FAILED - cookies bounced to login or errored')+'\n\n'
              +'Username:    '+(d.username||'(none)')+'\n'
              +'Target:      '+d.target+'\n'
              +'Final URL:   '+d.final_url+'\n'
              +'Final host:  '+d.final_host+'\n'
              +'HTTP status: '+d.status_code+'\n'
              +'Auth\'d:      '+d.authenticated+'\n'
              +'Duration:    '+d.duration_ms+'ms\n'
              +'Started at:  '+d.started_at+'\n'
              +'Source:      '+(d.source_ip||'(this host)')+'\n\n'
              +'Redirect chain ('+(d.redirect_chain||[]).length+' hops):\n'+(d.redirect_chain||[]).join('\n')
              +(d.error?'\n\nError: '+d.error:'')
              +'\n\nShare the timestamp + source IP with your CSOC; they should now check their SIEM for sign-in / Conditional Access / token-replay alerts.';
      alert(summary);
    })
    .catch(function(e){alert('Replay request failed: '+e.message);})
    .finally(function(){if(btn){btn.disabled=false;btn.textContent='⚡ Replay';}});
}
function sendTestFromForm(){
  var smtp=document.getElementById('camp-smtp').value;
  if(!smtp){alert('Pick a sending profile under Advanced first.');return;}
  var to=prompt('Send test email to:','');
  if(!to)return;
  fetch('/api/test-email',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({to:to,profile_name:smtp})})
    .then(function(r){if(!r.ok)return r.json().then(function(j){throw new Error(j.error||'HTTP '+r.status);});return r.json();})
    .then(function(d){alert('Test email sent to '+d.sent_to+' via '+d.via);})
    .catch(function(e){alert('Send failed: '+e.message);});
}
function loadDNS(){
  var grid=document.getElementById('dns-grid');if(!grid)return;
  grid.innerHTML='<div><div class="lbl">Checking&#x2026;</div></div>';
  fetch('/api/dns/health').then(function(r){return r.json();}).then(function(d){
    if(!d.results||!d.results.length){grid.innerHTML='<div><div class="lbl">No data</div></div>';return;}
    grid.innerHTML='<div style="grid-column:1 / -1;font-size:11px;color:var(--muted2);margin-bottom:6px">Domain: <code>'+esc(d.domain)+'</code></div>'+
      d.results.map(function(c){
        var color=c.status==='pass'?'var(--green)':(c.status==='warn'?'var(--amber)':'var(--red)');
        var icon=c.status==='pass'?'&#x2714;':(c.status==='warn'?'&#x26A0;':'&#x2716;');
        return '<div><div class="lbl">'+esc(c.name)+'</div><div class="val" style="color:'+color+'">'+icon+' '+c.status.toUpperCase()+'</div><div style="font-size:10px;color:var(--muted2);font-family:monospace;margin-top:3px;word-break:break-all">'+esc(c.detail)+'</div></div>';
      }).join('');
  }).catch(function(){grid.innerHTML='<div><div class="lbl" style="color:var(--amber)">DNS check failed</div></div>';});
}
function clearEngagement(){
  if(!confirm('Wipe ALL captured credentials and timeline events for the active engagement?\n\nThe engagement record itself stays. This cannot be undone.'))return;
  fetch('/api/engagement/clear',{method:'POST'})
    .then(function(r){if(!r.ok)return r.json().then(function(j){throw new Error(j.error||'HTTP '+r.status);});return r.json();})
    .then(function(d){alert('Cleared '+d.credentials_cleared+' credential(s) and '+d.timeline_cleared+' timeline event(s) from engagement '+d.engagement_id);load();})
    .catch(function(e){alert('Clear failed: '+e.message);});
}
function openEngForm(){
  fetch('/api/dashboard').then(function(r){return r.json();}).then(function(d){
    var e=d.engagement||{};
    ['id','name','client','operator','start_date','end_date','domain','phishlet_name','roe_reference','notes'].forEach(function(k){
      var el=document.getElementById('eng-f-'+k);if(el)el.value=e[k]||'';
    });
    showModal('modal-eng');
  });
}
function submitEng(){
  var body={};
  ['id','name','client','operator','start_date','end_date','domain','phishlet_name','roe_reference','notes'].forEach(function(k){
    body[k]=document.getElementById('eng-f-'+k).value;
  });
  fetch('/api/engagement',{method:'PUT',headers:{'Content-Type':'application/json'},body:JSON.stringify(body)})
    .then(function(r){if(!r.ok)return r.json().then(function(j){throw new Error(j.error||'HTTP '+r.status);});return r.json();})
    .then(function(){hideModal('modal-eng');load();})
    .catch(function(e){alert('Save failed: '+e.message);});
}
function openUsersModal(){loadUsers();showModal('modal-users');}
function loadUsers(){
  fetch('/api/users').then(function(r){if(!r.ok)throw new Error('HTTP '+r.status);return r.json();}).then(function(d){
    var box=document.getElementById('users-list');if(!box)return;
    var rows=[];
    (d.emails||[]).forEach(function(e){rows.push({val:e,kind:'email'});});
    (d.domains||[]).forEach(function(e){rows.push({val:e,kind:'domain'});});
    if(!rows.length){box.innerHTML='<div class="lbl">No users yet — add one below.</div>';return;}
    box.innerHTML=rows.map(function(r){
      var kindBadge=r.kind==='email'?'<span class="lure-ph">User</span>':'<span class="lure-ph" style="color:var(--purple);background:var(--purple-dim);border-color:rgba(179,136,255,.3)">Domain</span>';
      return '<div class="lurerow">'+kindBadge+'<span class="lure-url" title="'+esc(r.val)+'">'+esc(r.val)+'</span><button class="copybtn" type="button" onclick="removeUser(\''+esc(r.val).replace(/\x27/g,"\\x27")+'\')">Remove</button></div>';
    }).join('');
  }).catch(function(){
    var box=document.getElementById('users-list');if(box)box.innerHTML='<div class="lbl" style="color:var(--amber)">Failed to load users (OAuth may be disabled).</div>';
  });
}
function addUser(ev){
  ev.preventDefault();
  var v=document.getElementById('user-entry').value.trim();
  if(!v){return false;}
  fetch('/api/users',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({entry:v})})
    .then(function(r){if(!r.ok)return r.json().then(function(j){throw new Error(j.error||'HTTP '+r.status);});return r.json();})
    .then(function(){document.getElementById('user-entry').value='';loadUsers();})
    .catch(function(e){alert('Add failed: '+e.message);});
  return false;
}
function removeUser(entry){
  if(!confirm('Remove '+entry+' from the dashboard allowlist?'))return;
  fetch('/api/users/'+encodeURIComponent(entry),{method:'DELETE'})
    .then(function(r){if(!r.ok)return r.json().then(function(j){throw new Error(j.error||'HTTP '+r.status);});return r.json();})
    .then(loadUsers).catch(function(e){alert('Remove failed: '+e.message);});
}
function loadPhishlets(){
  fetch('/api/phishlets/state').then(function(r){return r.json();}).then(function(arr){
    var box=document.getElementById('phishlets-body');if(!box)return;
    if(!arr||!arr.length){box.innerHTML='<tr><td colspan="5"><div class="empty"><div class="empty-ico" aria-hidden="true">&#x1F3A3;</div>No phishlets discovered (check evilginx state dir)</div></td></tr>';return;}
    box.innerHTML=arr.map(function(p){
      var statusBadge=p.enabled
        ? '<span class="cstat exploitable" style="animation:none">Enabled</span>'
        : '<span class="cstat visited">Disabled</span>';
      var actions=p.enabled
        ? '<button class="copybtn" type="button" onclick="createLure(\''+esc(p.name).replace(/\x27/g,"\\x27")+'\')">+ Lure</button> '
          +'<button class="copybtn" type="button" onclick="disablePhishlet(\''+esc(p.name).replace(/\x27/g,"\\x27")+'\')">Disable</button>'
        : '<button class="copybtn" type="button" onclick="enablePhishlet(\''+esc(p.name).replace(/\x27/g,"\\x27")+'\')">Enable</button>';
      return '<tr>'
         +'<td><span class="lure-ph">'+esc(p.name)+'</span></td>'
         +'<td>'+statusBadge+'</td>'
         +'<td><span class="mono code-c">'+(p.hostname?esc(p.hostname):'<span style="color:var(--muted2)">(unset)</span>')+'</span></td>'
         +'<td style="text-align:center"><span class="mono">'+p.lures+'</span></td>'
         +'<td style="white-space:nowrap">'+actions+'</td>'
         +'</tr>';
    }).join('');
  }).catch(function(e){
    var box=document.getElementById('phishlets-body');
    if(box)box.innerHTML='<tr><td colspan="5"><div class="empty" style="color:var(--amber)">Failed to load phishlets: '+esc(e.message)+'</div></td></tr>';
  });
}
function enablePhishlet(name){
  var hostname=prompt('Hostname for "'+name+'" phishlet (e.g. cyb3rdefence.com — used as the apex; phishlet uses login.<host>):');
  if(!hostname)return;
  fetch('/api/phishlets/enable',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({phishlet:name,hostname:hostname})})
    .then(function(r){if(!r.ok)return r.json().then(function(j){throw new Error(j.error||'HTTP '+r.status);});return r.json();})
    .then(function(){alert('Phishlet '+name+' enabled. Evilginx is restarting.');setTimeout(loadPhishlets,3000);loadLures();})
    .catch(function(e){alert('Enable failed: '+e.message);});
}
function disablePhishlet(name){
  if(!confirm('Disable "'+name+'" phishlet? Active lures will stop responding.'))return;
  fetch('/api/phishlets/'+encodeURIComponent(name)+'/disable',{method:'POST'})
    .then(function(r){if(!r.ok)return r.json().then(function(j){throw new Error(j.error||'HTTP '+r.status);});return r.json();})
    .then(function(){alert('Phishlet '+name+' disabled. Evilginx is restarting.');setTimeout(loadPhishlets,3000);loadLures();})
    .catch(function(e){alert('Disable failed: '+e.message);});
}
function createLure(phishlet){
  var path=prompt('Custom lure path? (Leave blank to auto-generate)','');
  fetch('/api/lures',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({phishlet:phishlet,path:path||''})})
    .then(function(r){if(!r.ok)return r.json().then(function(j){throw new Error(j.error||'HTTP '+r.status);});return r.json();})
    .then(function(d){alert('Lure created: '+d.path+'\nVisible in the Active Lures card. Use it in a campaign via "Use in Campaign".');setTimeout(loadPhishlets,3000);loadLures();})
    .catch(function(e){alert('Create lure failed: '+e.message);});
}
function loadLures(){
  fetch('/api/lures').then(function(r){return r.json();}).then(function(arr){
    var box=document.getElementById('lures-list');if(!box)return;
    if(!arr||!arr.length){box.innerHTML='<div class="lbl">No lures configured. Run <code>lures create &lt;phishlet&gt;</code> on the PhishLab host.</div>';return;}
    box.innerHTML=arr.map(function(l,i){
      var paused=l.paused?'<span class="lure-paused">Paused</span>':'';
      return '<div class="lurerow">'
        +'<span class="lure-ph">'+esc(l.phishlet)+'</span>'
        +'<span class="lure-url" title="'+esc(l.url)+'">'+esc(l.url)+'</span>'
        +paused
        +'<button class="copybtn" type="button" data-url="'+esc(l.url)+'" data-i="'+i+'" onclick="copyLure(this)">Copy</button>'
        +'<button class="copybtn" type="button" onclick="useLureForCampaign(\''+esc(l.url).replace(/\x27/g,"\\x27")+'\')">Use in Campaign</button>'
        +'</div>';
    }).join('');
  });
}
function copyLure(btn){
  var url=btn.getAttribute('data-url');
  if(navigator.clipboard&&navigator.clipboard.writeText){
    navigator.clipboard.writeText(url).then(function(){
      btn.classList.add('ok');btn.textContent='Copied';
      setTimeout(function(){btn.classList.remove('ok');btn.textContent='Copy';},1500);
    });
  } else {
    var ta=document.createElement('textarea');ta.value=url;document.body.appendChild(ta);ta.select();document.execCommand('copy');document.body.removeChild(ta);
    btn.classList.add('ok');btn.textContent='Copied';
    setTimeout(function(){btn.classList.remove('ok');btn.textContent='Copy';},1500);
  }
}
function useLureForCampaign(url){
  openCampForm();
  setTimeout(function(){var u=document.getElementById('camp-url');if(u)u.value=url;},80);
}
function renderEng(eng){
  var g=document.getElementById('eng-grid');if(!g)return;
  if(!eng){g.innerHTML='<div><div class="lbl">No active engagement</div><div class="val dim">Click Edit to create one</div></div>';return;}
  var fields=[
    ['Engagement','id'],['Client','client'],['Operator','operator'],
    ['Window','_window'],['Domain','domain'],['Phishlet','phishlet_name'],
    ['RoE','roe_reference'],['Status','status']
  ];
  g.innerHTML=fields.map(function(f){
    var v;
    if(f[1]==='_window')v=(eng.start_date||'?')+' — '+(eng.end_date||'?');
    else v=eng[f[1]]||'';
    var cls=v?'val':'val dim';if(!v)v='(not set)';
    return '<div><div class="lbl">'+esc(f[0])+'</div><div class="'+cls+'">'+esc(v)+'</div></div>';
  }).join('');
}

initChart();load();loadLures();loadDNS();setInterval(load,15000);setInterval(loadLures,30000);setInterval(loadDNS,300000);connectWS();
</script>
<div class="modal" id="modal-camp" role="dialog" aria-modal="true" aria-labelledby="modal-camp-h">
  <div class="modal-card">
    <div class="modal-h"><h3 id="modal-camp-h">Launch Campaign</h3><button class="modal-x" type="button" onclick="hideModal('modal-camp')" aria-label="Close">&times;</button></div>
    <div class="modal-b">
      <div class="fld"><label for="camp-name">Campaign name</label><input id="camp-name" type="text" placeholder="auto-generated if blank"><div class="hint">Defaults to the engagement id + timestamp.</div></div>

      <div class="fld">
        <label>Recipients</label>
        <div style="display:flex;gap:6px;margin-bottom:8px;flex-wrap:wrap">
          <button type="button" class="tbar-btn" id="camp-mode-paste" onclick="setRecipMode('paste')">Paste emails</button>
          <button type="button" class="tbar-btn" id="camp-mode-group" onclick="setRecipMode('group')">Use existing group</button>
        </div>
        <div id="camp-recip-paste"><textarea id="camp-emails" placeholder="alice@target.com,Alice,Smith,Engineer&#10;bob@target.com,Bob,Jones,IT Manager&#10;&#10;Or just one email per line:&#10;charlie@target.com"></textarea><div class="hint">One per line. Format: <code>email,first,last,position</code> (only email is required).</div></div>
        <div id="camp-recip-group" style="display:none"><select id="camp-group"></select><div class="hint">Re-uses an existing Gophish group.</div></div>
      </div>

      <div class="fld"><label for="camp-tmpl">Email template</label><select id="camp-tmpl"></select><div class="hint" id="camp-tmpl-hint">Don't see a template? Create one on the Templates tab first.</div></div>

      <div class="fld"><label for="camp-url">Phish URL (lure)</label><input id="camp-url" type="url" placeholder="https://login.your-phishing-domain.com/abcdEFGH"><div class="hint">Click "Use in Campaign" on a lure row above to auto-fill.</div></div>

      <details style="margin-top:14px"><summary style="cursor:pointer;font-size:12px;color:var(--muted);user-select:none">Advanced (sender / landing page)</summary>
        <div style="margin-top:10px">
          <div class="fld"><label for="camp-smtp">Sending profile (sender)</label><select id="camp-smtp"></select></div>
          <div class="fld"><label for="camp-page">Gophish landing page</label><select id="camp-page"></select><div class="hint">Usually doesn't matter for evilginx — clicks go directly to the lure URL.</div></div>
        </div>
      </details>
    </div>
    <div class="modal-f">
      <button class="tbar-btn" type="button" onclick="hideModal('modal-camp')">Cancel</button>
      <button class="tbar-btn" type="button" onclick="sendTestFromForm()" title="Send a test email to one address using the selected sending profile">&#x2709; Test Send</button>
      <button class="tbar-btn primary" type="button" onclick="submitCamp()">Launch &amp; Send</button>
    </div>
  </div>
</div>
<div class="modal" id="modal-group" role="dialog" aria-modal="true" aria-labelledby="modal-group-h">
  <div class="modal-card">
    <div class="modal-h"><h3 id="modal-group-h">New Target Group</h3><button class="modal-x" type="button" onclick="hideModal('modal-group')" aria-label="Close">&times;</button></div>
    <div class="modal-b">
      <div class="fld"><label for="group-name">Group name</label><input id="group-name" type="text" placeholder="e.g. Pilot - Engineering"></div>
      <div class="fld"><label for="group-targets">Targets (one per line: <code>email,first,last,position</code>)</label><textarea id="group-targets" placeholder="alice@target.com,Alice,Smith,Engineer
bob@target.com,Bob,Jones,Manager"></textarea></div>
    </div>
    <div class="modal-f">
      <button class="tbar-btn" type="button" onclick="hideModal('modal-group')">Cancel</button>
      <button class="tbar-btn primary" type="button" onclick="submitGroup()">Create</button>
    </div>
  </div>
</div>
<div class="modal" id="modal-users" role="dialog" aria-modal="true" aria-labelledby="modal-users-h">
  <div class="modal-card">
    <div class="modal-h"><h3 id="modal-users-h">Dashboard Users</h3><button class="modal-x" type="button" onclick="hideModal('modal-users')" aria-label="Close">&times;</button></div>
    <div class="modal-b">
      <div style="font-size:12px;color:var(--muted);margin-bottom:14px">Who can sign in via Google OAuth. Adds take effect immediately &mdash; no service restart.</div>
      <div id="users-list"><div class="lbl">Loading&#x2026;</div></div>
      <form onsubmit="addUser(event)" style="display:flex;gap:8px;margin-top:14px;align-items:center;flex-wrap:wrap">
        <input type="text" id="user-entry" placeholder="user@example.com  OR  example.com (whole domain)" style="flex:1;min-width:240px;padding:8px 12px;background:var(--bg);border:1px solid var(--border2);border-radius:5px;color:var(--text);font-size:13px;font-family:inherit">
        <button class="tbar-btn primary" type="submit">&#x2795; Add User</button>
      </form>
      <div style="margin-top:6px;font-size:11px;color:var(--muted2)">Email: only that single user. Domain: anyone with that Workspace domain. Both formats accepted.</div>
    </div>
    <div class="modal-f">
      <button class="tbar-btn" type="button" onclick="hideModal('modal-users')">Done</button>
    </div>
  </div>
</div>
<div class="modal" id="modal-eng" role="dialog" aria-modal="true" aria-labelledby="modal-eng-h">
  <div class="modal-card">
    <div class="modal-h"><h3 id="modal-eng-h">Edit Engagement</h3><button class="modal-x" type="button" onclick="hideModal('modal-eng')" aria-label="Close">&times;</button></div>
    <div class="modal-b">
      <div class="fld"><label for="eng-f-id">Engagement ID</label><input id="eng-f-id" type="text" placeholder="ENG-2026-002"></div>
      <div class="fld"><label for="eng-f-name">Engagement name</label><input id="eng-f-name" type="text"></div>
      <div class="fld"><label for="eng-f-client">Client</label><input id="eng-f-client" type="text"></div>
      <div class="fld"><label for="eng-f-operator">Operator</label><input id="eng-f-operator" type="text" placeholder="operator@example.com"></div>
      <div class="fld" style="display:grid;grid-template-columns:1fr 1fr;gap:12px"><div><label for="eng-f-start_date">Start (YYYY-MM-DD)</label><input id="eng-f-start_date" type="text"></div><div><label for="eng-f-end_date">End (YYYY-MM-DD)</label><input id="eng-f-end_date" type="text"></div></div>
      <div class="fld"><label for="eng-f-domain">Phishing domain</label><input id="eng-f-domain" type="text"></div>
      <div class="fld"><label for="eng-f-phishlet_name">Phishlet</label><input id="eng-f-phishlet_name" type="text"></div>
      <div class="fld"><label for="eng-f-roe_reference">RoE reference</label><input id="eng-f-roe_reference" type="text" placeholder="path or doc id"></div>
      <div class="fld"><label for="eng-f-notes">Notes</label><textarea id="eng-f-notes"></textarea></div>
    </div>
    <div class="modal-f">
      <button class="tbar-btn" type="button" onclick="hideModal('modal-eng')">Cancel</button>
      <button class="tbar-btn primary" type="button" onclick="submitEng()">Save</button>
    </div>
  </div>
</div>
</body>
</html>`
