package server

import (
	"net/http"

	"github.com/CyberOneHQ/evilginx-lab/internal/server/handlers"
	"github.com/CyberOneHQ/evilginx-lab/internal/server/middleware"
	"github.com/gorilla/mux"
)

func NewRouter(api *handlers.APIHandler) http.Handler {
	r := mux.NewRouter()

	// API routes
	apiRouter := r.PathPrefix("/api").Subrouter()
	apiRouter.HandleFunc("/dashboard", api.HandleDashboard).Methods("GET")
	apiRouter.HandleFunc("/credentials", api.HandleCredentials).Methods("GET")
	apiRouter.HandleFunc("/services", api.HandleServices).Methods("GET")
	apiRouter.HandleFunc("/phishlets", api.HandlePhishlets).Methods("GET")

	// Campaign routes
	apiRouter.HandleFunc("/campaigns", api.HandleGetCampaigns).Methods("GET")
	apiRouter.HandleFunc("/campaigns", api.HandleLaunchCampaign).Methods("POST")
	apiRouter.HandleFunc("/campaigns/sync", api.HandleSyncCampaigns).Methods("POST")

	// Timeline routes
	apiRouter.HandleFunc("/timeline", api.HandleGetTimeline).Methods("GET")

	// Email template routes
	apiRouter.HandleFunc("/templates", api.HandleGetTemplates).Methods("GET")
	apiRouter.HandleFunc("/templates", api.HandleCreateTemplate).Methods("POST")
	apiRouter.HandleFunc("/templates/{id:[0-9]+}", api.HandleDeleteTemplate).Methods("DELETE")

	// WebSocket
	r.HandleFunc("/ws", api.HandleWebSocket)

	// Dashboard UI - inline HTML for zero-dependency deployment
	r.PathPrefix("/").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(dashboardHTML))
	})

	// Apply middleware
	handler := middleware.RequestLogger(middleware.LocalhostOnly(r))
	return handler
}

const dashboardHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Evilginx-Lab Dashboard</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;background:#0a0a0f;color:#e0e0e0;min-height:100vh}
.container{max-width:1400px;margin:0 auto;padding:24px}
header{display:flex;align-items:center;justify-content:space-between;margin-bottom:32px;padding-bottom:16px;border-bottom:1px solid #1a1a2e}
h1{font-size:24px;color:#00d4ff;font-weight:600}
.badge{background:#1a1a2e;padding:4px 12px;border-radius:12px;font-size:12px;color:#888}
.grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(250px,1fr));gap:20px;margin-bottom:32px}
.card{background:#12121a;border:1px solid #1a1a2e;border-radius:12px;padding:20px}
.card h2{font-size:14px;color:#888;text-transform:uppercase;letter-spacing:1px;margin-bottom:16px}
.metric{font-size:36px;font-weight:700;color:#00d4ff}
.metric.warning{color:#ff6b35}
.service{display:flex;justify-content:space-between;align-items:center;padding:8px 0;border-bottom:1px solid #1a1a2e}
.service:last-child{border-bottom:none}
.status{display:inline-block;width:8px;height:8px;border-radius:50%;margin-right:8px}
.status.active{background:#00ff88}
.status.inactive{background:#ff4444}
.status.unknown{background:#888}
table{width:100%;border-collapse:collapse}
th,td{text-align:left;padding:10px 12px;border-bottom:1px solid #1a1a2e}
th{color:#888;font-size:12px;text-transform:uppercase;letter-spacing:1px}
td{font-size:14px}
.empty{text-align:center;padding:40px;color:#555}
#error-banner{display:none;background:#2a1a1a;border:1px solid #ff4444;color:#ff4444;padding:12px 20px;border-radius:8px;margin-bottom:20px}
.tabs{display:flex;gap:8px;margin-bottom:20px}
.tab{padding:8px 16px;background:#1a1a2e;border:1px solid #2a2a3e;border-radius:8px;cursor:pointer;color:#888;font-size:13px;transition:all 0.2s}
.tab.active{background:#00d4ff22;border-color:#00d4ff;color:#00d4ff}
.tab:hover{border-color:#00d4ff88}
.tab-content{display:none}
.tab-content.active{display:block}
.event-row{display:flex;align-items:center;gap:12px;padding:10px 0;border-bottom:1px solid #1a1a2e;font-size:13px}
.event-row:last-child{border-bottom:none}
.event-icon{width:28px;height:28px;border-radius:50%;display:flex;align-items:center;justify-content:center;font-size:12px;flex-shrink:0}
.event-icon.email_sent{background:#1a3a2e;color:#00ff88}
.event-icon.email_opened{background:#1a2a3e;color:#00aaff}
.event-icon.link_clicked{background:#2a2a1e;color:#ffaa00}
.event-icon.submitted_data{background:#2a1a2e;color:#ff66cc}
.event-icon.credential_captured{background:#3a1a1a;color:#ff4444}
.event-icon.campaign_launched{background:#1a1a3a;color:#aa88ff}
.event-time{color:#555;min-width:140px}
.event-detail{flex:1}
.event-source{font-size:11px;padding:2px 6px;border-radius:4px;background:#1a1a2e;color:#666}
.camp-status{padding:2px 8px;border-radius:6px;font-size:11px;font-weight:600}
.camp-status.launched{background:#1a3a2e;color:#00ff88}
.camp-status.created{background:#1a2a3e;color:#00aaff}
.camp-status.completed{background:#1a1a2e;color:#888}
.camp-status.error{background:#3a1a1a;color:#ff4444}
btn{display:inline-block;padding:8px 16px;background:#00d4ff22;border:1px solid #00d4ff;color:#00d4ff;border-radius:8px;cursor:pointer;font-size:13px;transition:all 0.2s}
btn:hover{background:#00d4ff44}
.btn{display:inline-block;padding:8px 16px;background:#00d4ff22;border:1px solid #00d4ff;color:#00d4ff;border-radius:8px;cursor:pointer;font-size:13px;transition:all 0.2s}
.btn:hover{background:#00d4ff44}
.btn.secondary{background:#1a1a2e;border-color:#2a2a3e;color:#888}
.btn.secondary:hover{border-color:#00d4ff88;color:#00d4ff}
</style>
</head>
<body>
<div class="container">
<header>
<h1>Evilginx-Lab</h1>
<span class="badge" id="engagement-badge">No engagement loaded</span>
</header>
<div id="error-banner"></div>
<div class="grid">
<div class="card">
<h2>Captured Credentials</h2>
<div class="metric" id="cred-count">--</div>
</div>
<div class="card">
<h2>Campaigns</h2>
<div class="metric" id="campaign-count">--</div>
</div>
<div class="card">
<h2>Services</h2>
<div id="services-list"></div>
</div>
<div class="card">
<h2>Phishlets</h2>
<div id="phishlets-list"></div>
</div>
</div>

<div class="tabs">
<div class="tab active" data-tab="timeline">Timeline</div>
<div class="tab" data-tab="credentials">Credentials</div>
<div class="tab" data-tab="campaigns">Campaigns</div>
<div class="tab" data-tab="templates">Templates</div>
</div>

<div class="tab-content active" id="tab-timeline">
<div class="card">
<h2>Campaign Timeline <button class="btn secondary" style="float:right;font-size:11px;padding:4px 10px" onclick="syncCampaigns()">Sync</button></h2>
<div id="timeline-list"><div class="empty">No events yet</div></div>
</div>
</div>

<div class="tab-content" id="tab-credentials">
<div class="card">
<h2>Captured Credentials</h2>
<table>
<thead><tr><th>Time</th><th>Phishlet</th><th>Username</th><th>Password</th><th>Source IP</th></tr></thead>
<tbody id="creds-table"><tr><td colspan="5" class="empty">No credentials captured yet</td></tr></tbody>
</table>
</div>
</div>

<div class="tab-content" id="tab-campaigns">
<div class="card">
<h2>Campaigns</h2>
<table>
<thead><tr><th>Name</th><th>Status</th><th>Targets</th><th>Template</th><th>URL</th><th>Launched</th></tr></thead>
<tbody id="campaigns-table"><tr><td colspan="6" class="empty">No campaigns yet</td></tr></tbody>
</table>
</div>
</div>

<div class="tab-content" id="tab-templates">
<div class="card">
<h2>Email Templates</h2>
<table>
<thead><tr><th>ID</th><th>Name</th><th>Subject</th><th>Actions</th></tr></thead>
<tbody id="templates-table"><tr><td colspan="4" class="empty">No templates found</td></tr></tbody>
</table>
</div>
</div>

</div>
<script>
const $ = s => document.querySelector(s);
const $$ = s => document.querySelectorAll(s);

// Tab switching
$$('.tab').forEach(tab => {
  tab.addEventListener('click', () => {
    $$('.tab').forEach(t => t.classList.remove('active'));
    $$('.tab-content').forEach(c => c.classList.remove('active'));
    tab.classList.add('active');
    $('#tab-' + tab.dataset.tab).classList.add('active');
    if (tab.dataset.tab === 'templates') fetchTemplates();
  });
});

async function fetchDashboard() {
  try {
    const res = await fetch('/api/dashboard');
    if (!res.ok) throw new Error('API returned ' + res.status);
    const data = await res.json();
    render(data);
    $('#error-banner').style.display = 'none';
  } catch (err) {
    $('#error-banner').textContent = 'Dashboard API error: ' + err.message;
    $('#error-banner').style.display = 'block';
  }
}

function render(d) {
  if (d.engagement) {
    $('#engagement-badge').textContent = d.engagement.name + ' (' + d.engagement.status + ')';
  }
  $('#cred-count').textContent = d.credential_count || 0;
  $('#campaign-count').textContent = d.campaign_count || 0;

  // Services
  let shtml = '';
  (d.services || []).forEach(s => {
    const cls = s.status === 'active' ? 'active' : (s.status === 'inactive' ? 'inactive' : 'unknown');
    shtml += '<div class="service"><span><span class="status ' + cls + '"></span>' + s.name + '</span><span>' + s.status + '</span></div>';
  });
  $('#services-list').innerHTML = shtml || '<div class="empty">No services found</div>';

  // Phishlets
  let phtml = '';
  (d.phishlets || []).forEach(p => {
    phtml += '<div class="service"><span>' + p.name + '</span><span>' + (p.enabled ? 'enabled' : 'available') + '</span></div>';
  });
  $('#phishlets-list').innerHTML = phtml || '<div class="empty">No phishlets found</div>';

  // Credentials table
  renderCredentials(d.credentials || []);

  // Campaigns table
  renderCampaigns(d.campaigns || []);

  // Timeline
  renderTimeline(d.timeline || []);
}

function renderCredentials(creds) {
  if (creds.length === 0) {
    $('#creds-table').innerHTML = '<tr><td colspan="5" class="empty">No credentials captured yet</td></tr>';
  } else {
    let rows = '';
    creds.forEach(c => {
      const t = new Date(c.captured_at).toLocaleString();
      rows += '<tr><td>' + t + '</td><td>' + esc(c.phishlet) + '</td><td>' + esc(c.username) + '</td><td>' + esc(c.password) + '</td><td>' + esc(c.remote_addr) + '</td></tr>';
    });
    $('#creds-table').innerHTML = rows;
  }
}

function renderCampaigns(campaigns) {
  if (campaigns.length === 0) {
    $('#campaigns-table').innerHTML = '<tr><td colspan="6" class="empty">No campaigns yet</td></tr>';
  } else {
    let rows = '';
    campaigns.forEach(c => {
      const t = new Date(c.launched_at).toLocaleString();
      rows += '<tr><td>' + esc(c.name) + '</td><td><span class="camp-status ' + esc(c.status) + '">' + esc(c.status) + '</span></td><td>' + c.target_count + '</td><td>' + esc(c.template_name) + '</td><td>' + esc(c.phish_url) + '</td><td>' + t + '</td></tr>';
    });
    $('#campaigns-table').innerHTML = rows;
  }
}

const eventIcons = {
  email_sent: '\u2709',
  email_opened: '\uD83D\uDC41',
  link_clicked: '\uD83D\uDD17',
  submitted_data: '\uD83D\uDCDD',
  credential_captured: '\uD83D\uDD11',
  campaign_launched: '\uD83D\uDE80',
  target_status: '\uD83C\uDFAF',
  email_reported: '\u26A0',
  unknown: '\u2022'
};

function renderTimeline(events) {
  if (events.length === 0) {
    $('#timeline-list').innerHTML = '<div class="empty">No events yet</div>';
    return;
  }
  let html = '';
  events.forEach(e => {
    const t = new Date(e.timestamp).toLocaleString();
    const icon = eventIcons[e.event_type] || '\u2022';
    const email = e.email ? ' &middot; ' + esc(e.email) : '';
    html += '<div class="event-row">' +
      '<div class="event-icon ' + esc(e.event_type) + '">' + icon + '</div>' +
      '<div class="event-time">' + t + '</div>' +
      '<div class="event-detail">' + esc(e.event_type.replace(/_/g, ' ')) + email +
      (e.detail ? ' &middot; ' + esc(e.detail) : '') + '</div>' +
      '<div class="event-source">' + esc(e.source) + '</div>' +
      '</div>';
  });
  $('#timeline-list').innerHTML = html;
}

async function fetchTemplates() {
  try {
    const res = await fetch('/api/templates');
    if (!res.ok) return;
    const templates = await res.json();
    if (templates.length === 0) {
      $('#templates-table').innerHTML = '<tr><td colspan="4" class="empty">No templates found</td></tr>';
      return;
    }
    let rows = '';
    templates.forEach(t => {
      rows += '<tr><td>' + t.id + '</td><td>' + esc(t.name) + '</td><td>' + esc(t.subject || '') + '</td><td><button class="btn secondary" style="font-size:11px;padding:2px 8px" onclick="deleteTemplate(' + t.id + ')">Delete</button></td></tr>';
    });
    $('#templates-table').innerHTML = rows;
  } catch (err) {}
}

async function deleteTemplate(id) {
  if (!confirm('Delete template #' + id + '?')) return;
  await fetch('/api/templates/' + id, { method: 'DELETE' });
  fetchTemplates();
}

async function syncCampaigns() {
  try {
    await fetch('/api/campaigns/sync', { method: 'POST' });
    fetchDashboard();
  } catch (err) {}
}

function esc(s) {
  if (!s) return '';
  const d = document.createElement('div');
  d.textContent = s;
  return d.innerHTML;
}

// WebSocket for real-time updates
function connectWS() {
  const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
  const ws = new WebSocket(proto + '//' + location.host + '/ws');
  ws.onmessage = () => fetchDashboard();
  ws.onclose = () => setTimeout(connectWS, 3000);
}

fetchDashboard();
setInterval(fetchDashboard, 10000);
connectWS();
</script>
</body>
</html>`
