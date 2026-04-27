package server

import (
	"net/http"

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

	if authH != nil {
		apiRouter.HandleFunc("/auth/whoami", authH.WhoAmI).Methods("GET")
		r.HandleFunc("/auth/google/login", authH.Login).Methods("GET")
		r.HandleFunc("/auth/google/callback", authH.Callback).Methods("GET")
		r.HandleFunc("/auth/logout", authH.Logout).Methods("GET", "POST")
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
  integrity="sha384-fnbX44GdY9gNN8L17Y2bW+T0tc/0Cmv4dXyNhDmw6T0HYlc0z5UIQGqEzlkdLwQM"
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
  <section class="tabs-wrap" aria-label="Engagement detail tabs">
    <div class="tabs-hdr" role="tablist" aria-label="Engagement views">
      <button class="tbtn" type="button" role="tab" id="tab-btn-timeline"    aria-selected="true"  aria-controls="tab-timeline"    data-tab="timeline"    tabindex="0">&#x23F1; Timeline</button>
      <button class="tbtn" type="button" role="tab" id="tab-btn-credentials" aria-selected="false" aria-controls="tab-credentials" data-tab="credentials" tabindex="-1">&#x1F511; Credentials</button>
      <button class="tbtn" type="button" role="tab" id="tab-btn-campaigns"   aria-selected="false" aria-controls="tab-campaigns"   data-tab="campaigns"   tabindex="-1">&#x1F4CB; Campaigns</button>
      <button class="tbtn" type="button" role="tab" id="tab-btn-templates"   aria-selected="false" aria-controls="tab-templates"   data-tab="templates"   tabindex="-1">&#x2709; Templates</button>
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
      <div style="overflow-x:auto">
        <table class="dtable">
          <thead><tr><th scope="col">Campaign</th><th scope="col">Status</th><th scope="col">Targets</th><th scope="col">Template</th><th scope="col">Phish URL</th><th scope="col">Launched</th></tr></thead>
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
       +'<td><button class="expandbtn" type="button" aria-expanded="false" aria-controls="rec-'+c.id+'" onclick="toggleRec('+c.id+')">Why?</button></td>'
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

initChart();load();setInterval(load,15000);connectWS();
</script>
</body>
</html>`
