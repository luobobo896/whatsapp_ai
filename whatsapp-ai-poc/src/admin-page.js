export function adminPage() {
  return `<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>WhatsApp AI Ops</title>
  <style>
/* ===== DESIGN TOKENS ===== */
:root {
  --primary: #128C7E; --primary-active: #075E54; --primary-light: #25D366;
  --primary-soft: #e1f5f0; --primary-ghost: #f0faf7;
  --ink: #1a1f1c; --body: #3d403d; --muted: #6b736d; --muted-soft: #949e96;
  --line: #e2e6e3; --line-soft: #eef0ee;
  --canvas: #f5f7fa; --surface: #ffffff; --surface-soft: #f9fafb;
  --nav: #0d1f17; --nav2: #152a20; --nav-line: rgba(255,255,255,0.08);
  --nav-text: #b8c9bf; --nav-muted: #7a9685; --nav-active: #ffffff;
  --success: #1fa855; --success-soft: #e6f7ed;
  --warning: #e8a317; --warning-soft: #fef7e6;
  --error: #d94535; --error-soft: #fdecea;
  --info: #2d6a8e; --info-soft: #e4eff6;
  --chat-in: #f0f2f5; --chat-out: #d9fdd3;
  --radius-sm: 6px; --radius-md: 8px; --radius-lg: 12px; --radius-xl: 16px; --radius-pill: 9999px;
  --shadow-sm: 0 1px 3px rgba(0,0,0,0.04); --shadow-md: 0 4px 12px rgba(0,0,0,0.08); --shadow-lg: 0 20px 50px rgba(0,0,0,0.25);
  --font: "Inter", system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", "Microsoft YaHei", sans-serif;
  --font-mono: "JetBrains Mono", "Consolas", "Microsoft YaHei", monospace;
}

/* ===== RESET ===== */
*,*::before,*::after{box-sizing:border-box}
body{margin:0;min-width:320px;font-family:var(--font);background:var(--canvas);color:var(--ink);-webkit-font-smoothing:antialiased}

/* ===== TYPOGRAPHY ===== */
h1,h2,h3,h4{margin:0;line-height:1.25}
h2{font-size:28px;font-weight:700;letter-spacing:-0.3px}
h3{font-size:18px;font-weight:600}
p{margin:0;line-height:1.55}
a{color:var(--primary);text-decoration:none}

/* ===== BUTTONS ===== */
button,input,select,textarea{font:inherit;color:inherit}
button{border:0;border-radius:var(--radius-md);min-height:36px;padding:0 16px;background:var(--primary);color:#fff;cursor:pointer;font-weight:600;font-size:14px;display:inline-flex;align-items:center;justify-content:center;gap:6px;transition:background .15s;white-space:nowrap}
button:hover{background:var(--primary-active)}
button:disabled{opacity:0.5;cursor:not-allowed}
.btn-secondary{background:var(--surface);color:var(--ink);border:1px solid var(--line)}
.btn-secondary:hover{background:var(--surface-soft)}
.btn-danger{background:var(--error);color:#fff}
.btn-danger:hover{background:#c0392b}
.btn-ghost{background:transparent;color:var(--muted);border:1px solid transparent}
.btn-ghost:hover{background:var(--surface-soft);color:var(--ink)}
.btn-sm{min-height:28px;padding:0 10px;font-size:12px;border-radius:var(--radius-sm)}

/* ===== INPUTS ===== */
input[type="text"],input[type="number"],input[type="password"],input[type="search"],select,textarea{
  height:36px;padding:0 12px;border:1px solid var(--line);border-radius:var(--radius-md);
  background:var(--surface);color:var(--ink);font-size:14px;width:100%;transition:border-color .15s
}
input:focus,select:focus,textarea:focus{outline:none;border-color:var(--primary);box-shadow:0 0 0 3px rgba(18,140,126,0.1)}
input[type="number"]{width:100px}
textarea{height:auto;min-height:120px;padding:12px;resize:vertical;font-family:var(--font-mono);font-size:13px;line-height:1.6}
select{appearance:none;background-image:url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='12' height='12' viewBox='0 0 12 12'%3E%3Cpath d='M3 4.5L6 7.5L9 4.5' fill='none' stroke='%236b736d' stroke-width='1.5' stroke-linecap='round' stroke-linejoin='round'/%3E%3C/svg%3E");background-repeat:no-repeat;background-position:right 10px center;padding-right:30px}
input[type="checkbox"],input[type="radio"]{width:auto;height:auto;accent-color:var(--primary)}

/* ===== LABELS ===== */
.field{display:grid;gap:5px}
.field-label{font-size:12px;font-weight:700;color:var(--muted);text-transform:uppercase;letter-spacing:0.04em}

/* ===== LAYOUT ===== */
.app{display:grid;grid-template-columns:260px minmax(0,1fr);min-height:100vh}

/* ===== SIDEBAR ===== */
.sidebar{position:sticky;top:0;height:100vh;overflow-y:auto;padding:20px 14px;background:var(--nav);color:var(--nav-text);display:flex;flex-direction:column;gap:0;z-index:100}
.sidebar::-webkit-scrollbar{width:4px}
.sidebar::-webkit-scrollbar-thumb{background:rgba(255,255,255,0.12);border-radius:2px}
.brand{padding:4px 8px 16px;border-bottom:1px solid var(--nav-line);margin-bottom:4px}
.brand-kicker{color:var(--primary-light);font-size:11px;font-weight:800;letter-spacing:.08em;text-transform:uppercase}
.brand h1{margin:6px 0 0;font-size:20px;line-height:1.2;color:var(--nav-active);font-weight:700}
.nav-section{padding:18px 10px 4px;color:var(--nav-muted);font-size:10px;font-weight:800;letter-spacing:.08em;text-transform:uppercase}
.nav-btn{width:100%;display:flex;align-items:center;justify-content:space-between;gap:8px;min-height:44px;padding:0 10px;background:transparent;color:var(--nav-text);text-align:left;border:1px solid transparent;border-radius:var(--radius-md);font-weight:500;font-size:14px;transition:all .12s}
.nav-btn:hover{background:rgba(255,255,255,0.04);color:#fff}
.nav-btn.active{background:var(--nav2);border-color:rgba(37,211,102,0.18);color:var(--nav-active)}
.nav-btn small{color:var(--nav-muted);font-size:11px;font-weight:400}
.nav-btn.active small{color:var(--primary-light)}
.sidebar-foot{margin-top:auto;padding:10px 12px;border:1px solid var(--nav-line);border-radius:var(--radius-md);color:var(--nav-muted);font-size:11px;line-height:1.5}

/* ===== MAIN ===== */
.main{min-width:0;padding:24px}
.topbar{display:flex;justify-content:space-between;gap:16px;align-items:flex-start;margin-bottom:20px;flex-wrap:wrap}
.topbar-left{min-width:0}
.topbar-left h2{margin:0;font-size:28px}
.topbar-left p{margin:6px 0 0;color:var(--muted);font-size:14px}
.topbar-right{display:flex;gap:10px;align-items:center;flex-wrap:wrap}
.sync-status{color:var(--muted);font-size:12px;min-width:120px;text-align:right}

/* ===== VIEWS ===== */
.view{display:none}
.view.active{display:block}

/* ===== GRID ===== */
.grid{display:grid;gap:14px}
.grid-4{grid-template-columns:repeat(4,minmax(0,1fr))}
.grid-2{grid-template-columns:minmax(0,1fr) minmax(0,1fr);align-items:start}
.grid-3{grid-template-columns:repeat(3,minmax(0,1fr))}

/* ===== PANEL ===== */
.panel{background:var(--surface);border:1px solid var(--line);border-radius:var(--radius-lg);overflow:hidden;box-shadow:var(--shadow-sm)}
.panel-head{padding:14px 16px;border-bottom:1px solid var(--line-soft);display:flex;align-items:center;justify-content:space-between;gap:12px;flex-wrap:wrap}
.panel-head h3{font-size:16px;font-weight:700}
.panel-sub{color:var(--muted);font-size:12px;margin-top:2px}
.panel-body{padding:16px}

/* ===== METRIC CARD ===== */
.metric{background:var(--surface);border:1px solid var(--line);border-radius:var(--radius-lg);padding:16px;min-height:100px;display:flex;flex-direction:column;justify-content:center}
.metric-label{color:var(--muted);font-size:12px;font-weight:700;text-transform:uppercase;letter-spacing:.04em}
.metric-value{margin-top:8px;font-size:32px;line-height:1;font-weight:800;color:var(--ink)}
.metric-note{margin-top:6px;color:var(--muted);font-size:12px}

/* ===== STATUS BADGES ===== */
.badge{display:inline-flex;align-items:center;gap:6px;min-height:24px;padding:2px 10px;border-radius:var(--radius-pill);font-size:12px;font-weight:700;white-space:nowrap}
.badge::before{content:"";width:7px;height:7px;border-radius:50%;background:currentColor;flex-shrink:0}
.badge-ok{background:var(--success-soft);color:var(--success)}
.badge-warn{background:var(--warning-soft);color:var(--warning)}
.badge-err{background:var(--error-soft);color:var(--error)}
.badge-muted{background:var(--surface-soft);color:var(--muted)}
.badge-info{background:var(--info-soft);color:var(--info)}
.badge-sm{min-height:20px;padding:1px 8px;font-size:11px}
.badge-sm::before{width:6px;height:6px}

/* ===== CHIP ===== */
.chip{display:inline-flex;align-items:center;padding:3px 10px;border:1px solid var(--line);border-radius:var(--radius-pill);background:var(--surface-soft);color:var(--body);font-size:12px;font-weight:500;margin:0 4px 4px 0;white-space:nowrap}

/* ===== CARD ===== */
.card{background:var(--surface);border:1px solid var(--line);border-radius:var(--radius-lg);padding:14px;transition:box-shadow .15s}
.card:hover{box-shadow:var(--shadow-md)}
.card-row{display:flex;justify-content:space-between;align-items:flex-start;gap:10px;flex-wrap:wrap}
.card-title{font-weight:700;font-size:15px}
.card-meta{color:var(--muted);font-size:12px;line-height:1.5}

/* ===== PROGRESS BAR ===== */
.bar{height:8px;background:var(--line-soft);border-radius:var(--radius-pill);overflow:hidden;margin-top:6px}
.bar-fill{display:block;height:100%;border-radius:var(--radius-pill);background:var(--primary);transition:width .3s}
.bar-fill.warn{background:var(--warning)}
.bar-fill.danger{background:var(--error)}

/* ===== TABLE ===== */
table{width:100%;border-collapse:collapse;font-size:13px}
th,td{padding:10px 12px;border-bottom:1px solid var(--line-soft);text-align:left;vertical-align:top}
th{font-weight:700;font-size:12px;color:var(--muted);text-transform:uppercase;letter-spacing:.04em;background:var(--surface-soft);white-space:nowrap}
tr:hover td{background:var(--primary-ghost)}
.table-wrap{max-height:500px;overflow-y:auto}

/* ===== TABS ===== */
.tabs{display:flex;gap:0;border-bottom:2px solid var(--line-soft);margin-bottom:16px}
.tab-btn{padding:10px 20px;background:transparent;color:var(--muted);border:0;border-bottom:2px solid transparent;margin-bottom:-2px;border-radius:0;font-weight:600;font-size:14px;min-height:40px;transition:all .15s}
.tab-btn:hover{color:var(--ink);background:transparent}
.tab-btn.active{color:var(--primary);border-bottom-color:var(--primary);background:transparent}
.tab-panel{display:none}
.tab-panel.active{display:block}

/* ===== CHAT BUBBLES ===== */
.chat-conversation{display:flex;flex-direction:column;gap:6px;padding:12px 0}
.chat-divider{text-align:center;padding:8px 16px}
.chat-divider span{display:inline-block;padding:4px 14px;background:var(--surface-soft);border-radius:var(--radius-pill);color:var(--muted);font-size:12px;font-weight:600}
.chat-bubble{max-width:70%;padding:10px 14px;border-radius:12px;line-height:1.55;font-size:14px;word-break:break-word}
.chat-bubble.inbound{align-self:flex-start;background:var(--chat-in);border-bottom-left-radius:4px;color:var(--ink)}
.chat-bubble.outbound{align-self:flex-end;background:var(--chat-out);border-bottom-right-radius:4px;color:var(--ink)}
.chat-time{font-size:11px;color:var(--muted-soft);margin-top:4px;text-align:right}
.chat-meta{font-size:11px;color:var(--muted);margin-bottom:2px}

/* ===== FORM LAYOUTS ===== */
.form-row{display:flex;gap:10px;align-items:flex-end;flex-wrap:wrap}
.form-row .field{flex:1;min-width:160px}
.form-inline{display:flex;gap:10px;align-items:flex-end;flex-wrap:wrap}
.form-inline .field{min-width:140px}
.card-actions{display:flex;gap:8px;align-items:center;flex-wrap:wrap;margin-top:10px}

/* ===== MODAL ===== */
.modal-overlay{position:fixed;inset:0;background:rgba(0,0,0,0.45);display:flex;align-items:center;justify-content:center;z-index:1000}
.modal-overlay.hidden{display:none}
.modal-box{width:min(480px,92vw);max-height:85vh;overflow-y:auto;padding:24px;border-radius:var(--radius-xl);background:var(--surface);box-shadow:var(--shadow-lg)}
.modal-box.wide{width:min(640px,92vw)}
.modal-box h3{margin:0 0 12px;font-size:18px}
.modal-box p{margin:0 0 16px;color:var(--muted);line-height:1.6;font-size:14px}
.modal-box .field{margin-bottom:12px}
.modal-actions{display:flex;gap:10px;justify-content:flex-end;margin-top:16px}
.qr-box{text-align:center;padding:16px;background:var(--surface-soft);border-radius:var(--radius-lg);border:1px solid var(--line)}
.qr-box img{width:200px;height:200px;border-radius:var(--radius-md);background:#fff}
.qr-status{margin-top:10px;font-size:14px;font-weight:600}

/* ===== TOAST ===== */
.toast-container{position:fixed;top:20px;right:20px;z-index:2000;display:flex;flex-direction:column;gap:8px}
.toast{padding:12px 20px;background:var(--ink);color:#fff;border-radius:var(--radius-md);font-size:13px;font-weight:500;box-shadow:var(--shadow-md);animation:toastIn .25s ease;max-width:360px}
.toast.error{background:var(--error)}
.toast.success{background:var(--success)}
@keyframes toastIn{from{opacity:0;transform:translateX(20px)}to{opacity:1;transform:translateX(0)}}
@keyframes toastOut{from{opacity:1}to{opacity:0;transform:translateX(20px)}}

/* ===== MISC ===== */
.notice{padding:14px;border-radius:var(--radius-md);background:var(--surface-soft);border:1px solid var(--line);color:var(--muted);line-height:1.6;font-size:13px}
.notice.error{color:var(--error);background:var(--error-soft);border-color:#f5c6cb}
.empty-state{text-align:center;padding:40px 20px;color:var(--muted)}
.empty-state .empty-icon{font-size:40px;margin-bottom:12px}
.empty-state .empty-title{font-size:16px;font-weight:700;color:var(--ink);margin-bottom:6px}
.pager{display:flex;align-items:center;gap:10px;justify-content:center;margin-top:16px}
.code-editor{font-family:var(--font-mono);font-size:13px;line-height:1.6;min-height:300px;width:100%;padding:14px;border:1px solid var(--line);border-radius:var(--radius-md);background:var(--surface-soft);resize:vertical}
.live-feed{max-height:500px;overflow-y:auto;display:flex;flex-direction:column;gap:6px}
.live-msg{display:flex;gap:8px;padding:8px 10px;border-radius:var(--radius-md);background:var(--surface);border:1px solid var(--line-soft);animation:fadeIn .3s;font-size:13px}
@keyframes fadeIn{from{opacity:0;transform:translateY(-6px)}to{opacity:1;transform:translateY(0)}}
.flex-between{display:flex;justify-content:space-between;align-items:center;gap:10px;flex-wrap:wrap}
.text-muted{color:var(--muted);font-size:12px}
.text-strong{font-weight:700}
.text-sm{font-size:12px}
.mt-1{margin-top:8px}
.mt-2{margin-top:14px}
.mt-3{margin-top:20px}
.mb-1{margin-bottom:8px}
.mb-2{margin-bottom:14px}
.gap-1{gap:8px}
.gap-2{gap:14px}

/* ===== RESPONSIVE ===== */
@media(max-width:1080px){
  .app{grid-template-columns:1fr}
  .sidebar{position:relative;height:auto;overflow-y:visible;flex-direction:row;flex-wrap:wrap;padding:12px;gap:6px;align-items:center}
  .sidebar .brand,.sidebar .nav-section,.sidebar .sidebar-foot{display:none}
  .sidebar .nav-btn{width:auto;min-height:36px;padding:0 10px;font-size:13px}
  .grid-4{grid-template-columns:repeat(2,minmax(0,1fr))}
  .grid-3{grid-template-columns:repeat(2,minmax(0,1fr))}
}
@media(max-width:720px){
  .main{padding:14px}
  .topbar{flex-direction:column}
  .grid-4,.grid-3,.grid-2{grid-template-columns:1fr}
  .sidebar{gap:4px}
  .sidebar .nav-btn{font-size:11px;padding:0 8px;min-height:32px}
  .form-row{flex-direction:column}
  .form-row .field{min-width:100%}
  .chat-bubble{max-width:85%}
  .modal-box.wide{width:98vw}
}
</style>
</head>
<body>
<div class="toast-container" id="toastContainer"></div>

<!-- QR Modal -->
<div class="modal-overlay hidden" id="qrModal">
  <div class="modal-box">
    <div class="flex-between mb-2"><h3>扫码关联 WhatsApp</h3><button class="btn-ghost btn-sm" onclick="closeQrModal()">&#x2715;</button></div>
    <div class="qr-box">
      <img id="qrImage" src="" alt="QR Code" style="display:none">
      <div id="qrLoading" class="notice">正在生成二维码...</div>
    </div>
    <div class="qr-status" id="qrStatus"></div>
    <p class="text-muted mt-2">WhatsApp 手机端打开：设置 / 已链接的设备 / 链接设备，扫描上方二维码。</p>
  </div>
</div>

<!-- Confirm Modal -->
<div class="modal-overlay hidden" id="confirmModal">
  <div class="modal-box">
    <h3 id="confirmTitle">确认操作</h3>
    <p id="confirmBody"></p>
    <div class="modal-actions">
      <button class="btn-secondary" onclick="closeConfirm()">取消</button>
      <button class="btn-danger" id="confirmOk">确认</button>
    </div>
  </div>
</div>

<!-- Role Edit Modal -->
<div class="modal-overlay hidden" id="roleModal">
  <div class="modal-box">
    <h3 id="roleModalTitle">编辑角色</h3>
    <div class="field"><label class="field-label">角色 ID</label><input id="roleId" type="text" placeholder="support"></div>
    <div class="field"><label class="field-label">角色名称</label><input id="roleName" type="text" placeholder="客服角色"></div>
    <div class="field"><label class="field-label">描述</label><input id="roleDesc" type="text" placeholder="角色描述"></div>
    <div class="field"><label class="field-label">关联知识库（可多选）</label><div id="roleBaseChecks" class="text-muted">加载中...</div></div>
    <div class="modal-actions">
      <button class="btn-secondary" onclick="closeRoleModal()">取消</button>
      <button id="roleSaveBtn" onclick="saveRole()">保存</button>
    </div>
  </div>
</div>

<!-- Create Account Modal -->
<div class="modal-overlay hidden" id="createAccountModal">
  <div class="modal-box">
    <h3>&#x65B0;&#x589E;&#x5BA2;&#x670D;</h3>
    <div class="field"><label class="field-label">&#x5BA2;&#x670D;&#x540D;&#x79F0; <span style="color:var(--error)">*</span></label><input id="newLabel" type="text" placeholder="&#x8BF7;&#x8F93;&#x5165;&#x5BA2;&#x670D;&#x540D;&#x79F0;"></div>
    <div style="display:grid;grid-template-columns:120px 1fr;gap:12px;margin-top:12px">
      <div class="field"><label class="field-label">&#x6BCF;&#x65E5;&#x4E0A;&#x9650; <span style="color:var(--error)">*</span></label><input id="newLimit" type="number" min="1" value="30"></div>
      <div class="field"><label class="field-label">分配角色 <span style="color:var(--error)">*</span></label><div id="newAccountRoles" class="text-muted">&#x52A0;&#x8F7D;&#x4E2D;...</div></div>
    </div>
    <div class="modal-actions" style="margin-top:12px">
      <button class="btn-secondary" onclick="closeCreateAccountModal()">&#x53D6;&#x6D88;</button>
      <button onclick="createAccount()">&#x4FDD;&#x5B58;</button>
    </div>
  </div>
</div>

<!-- Knowledge Base Modal -->
<div class="modal-overlay hidden" id="baseModal">
  <div class="modal-box">
    <h3 id="baseModalTitle">编辑知识库</h3>
    <div class="field"><label class="field-label">名称</label><input id="baseName" type="text" placeholder="知识库名称"></div>
    <div class="field"><label class="field-label">类型</label><select id="baseType"><option value="faq">FAQ</option><option value="product">产品</option><option value="document">文档</option></select></div>
    <div class="field"><label class="field-label">描述</label><input id="baseDesc" type="text" placeholder="描述"></div>
    <div class="modal-actions">
      <button class="btn-secondary" onclick="closeBaseModal()">取消</button>
      <button id="baseSaveBtn" onclick="saveBase()">保存</button>
    </div>
  </div>
</div>

<!-- Account Edit Modal -->
<div class="modal-overlay hidden" id="editAccountModal">
  <div class="modal-box">
    <h3>编辑客服账号</h3>
    <input type="hidden" id="editAccountKey">
    <div class="field"><label class="field-label">客服名称</label><input id="editAccountLabel" type="text" placeholder="客服名称"></div>
    <div class="field"><label class="field-label">每日上限</label><input id="editAccountLimit" type="number" min="0" value="30"></div>
    <div class="field"><label class="field-label">分配角色</label><div id="editAccountRoles"></div></div>
    <div class="modal-actions">
      <button class="btn-secondary" onclick="closeEditAccountModal()">取消</button>
      <button onclick="saveAccountEdit()">保存</button>
    </div>
  </div>
</div>

<div class="app">
  <aside class="sidebar">
    <div class="brand"><div class="brand-kicker">WhatsApp AI Ops</div><h1>&#x667A;&#x80FD;&#x5BA2;&#x670D;&#x540E;&#x53F0;</h1></div>
    <button class="nav-btn active" data-view="overview">&#x1F4CA; &#x603B;&#x89C8;<small>Overview</small></button>
    <button class="nav-btn" data-view="accounts">&#x1F464; &#x5BA2;&#x670D;&#x8D26;&#x53F7;<small>Accounts</small></button>
    <button class="nav-btn" data-view="knowledge">&#x1F9E0; &#x77E5;&#x8BC6;&#x5E93;<small>Knowledge</small></button>
    <button class="nav-btn" data-view="models">&#x1F527; &#x6A21;&#x578B;&#x914D;&#x7F6E;<small>Models</small></button>
    <div class="nav-section">&#x8FD0;&#x8425;&#x6570;&#x636E;</div>
    <button class="nav-btn" data-view="messages">&#x1F4AC; &#x95EE;&#x7B54;&#x5386;&#x53F2;<small>History</small></button>
    <button class="nav-btn" data-view="customers">&#x1F465; &#x5BA2;&#x6237;&#x5217;&#x8868;<small>Customers</small></button>
    <button class="nav-btn" data-view="live">&#x1F4E1; &#x5B9E;&#x65F6;&#x76D1;&#x63A7;<small>Live</small></button>
    <div class="nav-section">&#x7B56;&#x7565;&#x914D;&#x7F6E;</div>
    <button class="nav-btn" data-view="antiban">&#x1F6E1;&#xFE0F; &#x9632;&#x5C01;&#x7B56;&#x7565;<small>Anti-ban</small></button>
    <button class="nav-btn" data-view="settings">&#x2699;&#xFE0F; &#x8BBE;&#x7F6E;<small>Settings</small></button>
    <div class="nav-section">&#x7CFB;&#x7EDF;</div>
    <button class="nav-btn" data-view="audit">&#x1F4CB; &#x64CD;&#x4F5C;&#x65E5;&#x5FD7;<small>Audit</small></button>
    <button class="nav-btn" data-view="alerts">&#x1F514; &#x544A;&#x8B66;&#x901A;&#x77E5;<small>Alerts</small></button>
    <button class="nav-btn" data-view="sandbox">&#x1F9EA; &#x5FEB;&#x6377;&#x6D4B;&#x8BD5;<small>Sandbox</small></button>
    <div class="sidebar-foot">&#x77E5;&#x8BC6;&#x5E93;&#x4FDD;&#x5B58;&#x540E;&#x4F1A;&#x81EA;&#x52A8;&#x6821;&#x9A8C;&#x683C;&#x5F0F;&#x5E76;&#x91CD;&#x65B0;&#x751F;&#x6210; OpenClaw agents&#x3002;&#x5BA2;&#x6237;&#x56DE;&#x590D;&#x5386;&#x53F2;&#x6BCF; 3 &#x79D2;&#x81EA;&#x52A8;&#x5237;&#x65B0;&#x3002;</div>
  </aside>

  <main class="main">
    <div class="topbar">
      <div class="topbar-left"><h2 id="pageTitle">&#x603B;&#x89C8;</h2><p id="pageDesc">&#x67E5;&#x770B;&#x8D26;&#x53F7;&#x53EF;&#x7528;&#x6027;&#x3001;&#x4ECA;&#x65E5;&#x56DE;&#x590D;&#x91CF;&#x548C;&#x77E5;&#x8BC6;&#x5E93;&#x8986;&#x76D6;&#x60C5;&#x51B5;&#x3002;</p></div>
      <div class="topbar-right">
        <button onclick="loadAll()">&#x5237;&#x65B0;&#x5168;&#x90E8;</button>
        <button class="btn-secondary" onclick="logout()">&#x9000;&#x51FA;</button>
        <span class="sync-status" id="syncStatus">&#x7B49;&#x5F85;&#x540C;&#x6B65;</span>
      </div>
    </div>

    <!-- OVERVIEW -->
    <section id="overview" class="view active">
      <div class="grid grid-4 mb-2" id="metrics"></div>
      <div class="panel mt-3">
        <div class="panel-head"><div><h3>&#x8D26;&#x53F7;&#x5065;&#x5EB7;</h3><p class="panel-sub">&#x5728;&#x7EBF;&#x3001;&#x505C;&#x7528;&#x3001;&#x5F02;&#x5E38;&#x8D26;&#x53F7;&#x5206;&#x5E03;&#x3002;</p></div></div>
        <div class="panel-body" id="overviewAccounts"></div>
      </div>
      <div class="grid grid-2 mt-3">
        <div class="panel"><div class="panel-head"><h3>7&#x5929;&#x6D88;&#x606F;&#x8D8B;&#x52BF;</h3></div><div class="panel-body"><div id="volumeChart" style="width:100%;height:200px"></div></div></div>
        <div class="panel"><div class="panel-head"><h3>&#x70ED;&#x95E8;&#x4EA7;&#x54C1; Top 5</h3></div><div class="panel-body"><div id="productChart" style="width:100%;height:200px"></div></div></div>
      </div>
      <div class="grid grid-4 mt-3" id="responseMetrics"></div>
    </section>

    <!-- ACCOUNTS -->
    <section id="accounts" class="view">
      <div class="panel">
        <div class="panel-head"><div><h3>&#x8D26;&#x53F7;&#x6C60;&#x7BA1;&#x7406;</h3><p class="panel-sub">&#x65B0;&#x589E; WhatsApp &#x5BA2;&#x670D;&#x3001;&#x626B;&#x7801;&#x767B;&#x5F55;&#x3001;&#x8BBE;&#x7F6E;&#x6BCF;&#x65E5;&#x56DE;&#x590D;&#x4E0A;&#x9650;&#x548C;分配角色&#x3002;</p></div><div style="display:flex;gap:10px;align-items:center"><span class="badge" id="accountHealth">&#x68C0;&#x67E5;&#x4E2D;</span><button onclick="openCreateAccountModal()">&#x65B0;&#x589E;&#x5BA2;&#x670D;</button></div></div>
        <div class="panel-body">
          <div id="accountsList" class="grid gap-2"><div class="notice">&#x8D26;&#x53F7;&#x72B6;&#x6001;&#x52A0;&#x8F7D;&#x4E2D;...</div></div>
        </div>
      </div>
    </section>

    <!-- KNOWLEDGE -->
    <section id="knowledge" class="view">
      <div class="tabs">
        <button class="tab-btn active" data-ktab="roles">&#x89D2;&#x8272;&#x7BA1;&#x7406;</button>
        <button class="tab-btn" data-ktab="bases">&#x77E5;&#x8BC6;&#x5E93;&#x7BA1;&#x7406;</button>
      </div>
      <div class="tab-panel active" id="ktab-roles">
        <div class="panel"><div class="panel-head"><div><h3>知识角色</h3><p class="panel-sub">&#x7BA1;&#x7406;&#x5BA2;&#x670D;&#x89D2;&#x8272;&#xFF0C;&#x6BCF;&#x4E2A;&#x89D2;&#x8272;&#x53EF;&#x52FE;&#x9009;&#x591A;&#x4E2A;&#x77E5;&#x8BC6;&#x5E93;&#x3002;</p></div><button onclick="openRoleModal()">&#x65B0;&#x589E;&#x89D2;&#x8272;</button></div><div class="panel-body"><div id="roleList" class="grid gap-1"><div class="notice">&#x52A0;&#x8F7D;&#x4E2D;...</div></div></div></div>
      </div>
      <div class="tab-panel" id="ktab-bases">
        <div class="panel"><div class="panel-head"><div><h3>&#x77E5;&#x8BC6;&#x5E93;</h3><p class="panel-sub">&#x7BA1;&#x7406;&#x77E5;&#x8BC6;&#x5E93;&#xFF0C;&#x5305;&#x542B; FAQ&#x3001;&#x4EA7;&#x54C1;&#x3001;&#x6587;&#x6863;&#x7B49;&#x7C7B;&#x578B;&#x3002;&#x70B9;&#x51FB;&#x201C;&#x7BA1;&#x7406;&#x5185;&#x5BB9;&#x201D;&#x4E0A;&#x4F20;&#x77E5;&#x8BC6;&#x6761;&#x76EE;&#x3002;</p></div><button onclick="openBaseModal()">&#x65B0;&#x589E;&#x77E5;&#x8BC6;&#x5E93;</button></div><div class="panel-body"><div id="baseList" class="grid gap-1"><div class="notice">&#x52A0;&#x8F7D;&#x4E2D;...</div></div>
        <div id="baseEntriesSection" style="display:none;margin-top:16px">
          <div class="panel"><div class="panel-head"><div><h3 id="baseEntriesTitle">&#x77E5;&#x8BC6;&#x6761;&#x76EE;</h3><p class="panel-sub">&#x7F16;&#x8F91;&#x77E5;&#x8BC6;&#x5E93;&#x4E2D;&#x7684;&#x6761;&#x76EE;&#x3002;</p></div><button class="btn-ghost btn-sm" onclick="closeBaseEntries()">&#x2715; &#x5173;&#x95ED;</button></div><div class="panel-body">
          <div class="flex-between mb-2"><div class="form-inline"><div class="field"><label class="field-label">&#x95EE;&#x9898;/&#x540D;&#x79F0;</label><input id="newEntryQ" type="text" placeholder="&#x6807;&#x9898;"></div><div class="field"><label class="field-label">&#x56DE;&#x7B54;/&#x5185;&#x5BB9;</label><input id="newEntryA" type="text" placeholder="&#x5185;&#x5BB9;"></div><button onclick="addBaseEntry()">&#x6DFB;&#x52A0;</button></div><div class="form-inline"><div class="field"><label class="field-label">CSV&#x5BFC;&#x5165;</label><input id="csvFile" type="file" accept=".csv" style="padding:6px;height:auto"></div><button class="btn-secondary" onclick="importBaseCsv()">&#x5BFC;&#x5165;CSV</button></div></div>
          <div class="table-wrap"><table><thead><tr><th>&#x6807;&#x9898;</th><th>&#x5185;&#x5BB9;</th><th>&#x5206;&#x7C7B;</th><th>&#x64CD;&#x4F5C;</th></tr></thead><tbody id="entryBody"><tr><td colspan="4" class="text-muted">&#x8BF7;&#x9009;&#x62E9;&#x77E5;&#x8BC6;&#x5E93;</td></tr></tbody></table></div>
        </div></div></div></div></div>
    </section>

    <!-- MODELS -->
    <section id="models" class="view">
      <div class="panel"><div class="panel-head"><div><h3>&#x6A21;&#x578B;&#x914D;&#x7F6E;</h3><p class="panel-sub">&#x7F16;&#x8F91; OpenClaw providers &#x914D;&#x7F6E;&#xFF0C;&#x4FDD;&#x5B58;&#x540E;&#x91CD;&#x542F;&#x751F;&#x6548;&#x3002;</p></div><button onclick="saveModels()">&#x4FDD;&#x5B58;&#x914D;&#x7F6E;</button></div>
      <div class="panel-body">
        <div class="field mb-2"><label class="field-label">&#x9ED8;&#x8BA4;&#x6A21;&#x578B; (fallback)</label><select id="fallbackModel" style="min-width:260px"><option value="">&#x8BF7;&#x9009;&#x62E9;...</option></select></div>
        <div id="providersConfig"><div class="notice">&#x52A0;&#x8F7D;&#x4E2D;...</div></div>
        <span class="text-muted" id="modelStatus"></span>
      </div></div>
    </section>

    <!-- MESSAGES -->
    <section id="messages" class="view">
      <div class="panel"><div class="panel-head"><div><h3>&#x95EE;&#x7B54;&#x5386;&#x53F2;</h3><p class="panel-sub">&#x6309;&#x5BA2;&#x670D;&#x67E5;&#x770B;&#x5BA2;&#x6237;&#x63D0;&#x95EE;&#x548C; OpenClaw &#x56DE;&#x590D;&#x3002;</p></div><span class="badge badge-info" id="messageCount">0 &#x6761;</span></div>
      <div class="panel-body">
        <div class="form-inline mb-2">
          <div class="field"><label class="field-label">&#x5BA2;&#x670D;</label><select id="messageAccount" onchange="changeMessageAccount(this.value)" style="min-width:180px"></select></div>
          <div class="field"><label class="field-label">&#x641C;&#x7D22;</label><input id="messageSearch" type="text" placeholder="&#x5173;&#x952E;&#x8BCD;" onkeydown="if(event.key==='Enter')searchMessages()"></div>
          <button onclick="searchMessages()">&#x641C;&#x7D22;</button>
          <button class="btn-secondary" onclick="clearMessageSearch()">&#x6E05;&#x7A7A;</button>
          <button class="btn-secondary" onclick="exportMessages()">&#x5BFC;&#x51FA;CSV</button>
          <div class="field"><label class="field-label">&#x6BCF;&#x9875;</label><select id="messagePageSize" onchange="changeMessagePageSize(this.value)" style="width:80px"><option value="10">10</option><option value="20" selected>20</option><option value="50">50</option></select></div>
        </div>
        <div id="messagesList"><div class="notice">&#x95EE;&#x7B54;&#x5386;&#x53F2;&#x52A0;&#x8F7D;&#x4E2D;...</div></div>
        <div class="pager"><button class="btn-secondary" onclick="changeMessagePage(-1)">&#x4E0A;&#x4E00;&#x9875;</button><span class="text-muted" id="messagePageInfo">&#x7B2C; 1 / 1 &#x9875;</span><button class="btn-secondary" onclick="changeMessagePage(1)">&#x4E0B;&#x4E00;&#x9875;</button></div>
      </div></div>
    </section>

    <!-- CUSTOMERS -->
    <section id="customers" class="view">
      <div class="panel"><div class="panel-head"><div><h3>&#x5BA2;&#x6237;&#x5217;&#x8868;</h3><p class="panel-sub">&#x4ECE;&#x6D88;&#x606F;&#x8BB0;&#x5F55;&#x805A;&#x5408;&#x5BA2;&#x6237;&#x4FE1;&#x606F;&#xFF0C;&#x6309;&#x6700;&#x8FD1;&#x8054;&#x7CFB;&#x65F6;&#x95F4;&#x6392;&#x5E8F;&#x3002;</p></div><span class="badge badge-info" id="customerCount">0 &#x4F4D;</span></div>
      <div class="panel-body">
        <div class="field mb-2"><label class="field-label">&#x8D26;&#x53F7;</label><select id="customerAccount" onchange="loadCustomers()" style="min-width:200px"><option value="">&#x5168;&#x90E8;</option></select></div>
        <div class="table-wrap"><table><thead><tr><th>&#x5BA2;&#x6237;&#x53F7;&#x7801;</th><th>&#x8D26;&#x53F7;</th><th>&#x5BF9;&#x8BDD;&#x6B21;&#x6570;</th><th>&#x9996;&#x6B21;&#x8054;&#x7CFB;</th><th>&#x6700;&#x8FD1;&#x8054;&#x7CFB;</th><th>&#x6700;&#x8FD1;&#x6D88;&#x606F;</th></tr></thead><tbody id="customerBody"><tr><td colspan="6" class="text-muted">&#x52A0;&#x8F7D;&#x4E2D;...</td></tr></tbody></table></div>
        <div class="pager"><button class="btn-secondary" onclick="changeCustomerPage(-1)">&#x4E0A;&#x4E00;&#x9875;</button><span class="text-muted" id="customerPageInfo">&#x7B2C; 1 / 1 &#x9875;</span><button class="btn-secondary" onclick="changeCustomerPage(1)">&#x4E0B;&#x4E00;&#x9875;</button></div>
      </div></div>
    </section>

    <!-- LIVE -->
    <section id="live" class="view">
      <div class="panel"><div class="panel-head"><div><h3>&#x5B9E;&#x65F6;&#x6D88;&#x606F;&#x76D1;&#x63A7;</h3><p class="panel-sub">&#x81EA;&#x52A8;&#x5237;&#x65B0;&#x6700;&#x65B0;&#x6D88;&#x606F;&#xFF0C;2&#x79D2;&#x8F6E;&#x8BE2;&#x3002;</p></div><div style="display:flex;gap:10px;align-items:center"><select id="liveAccount" onchange="resetLiveFeed()" style="min-width:140px"><option value="">&#x5168;&#x90E8;</option></select><button class="btn-secondary" onclick="toggleLiveAutoScroll()" id="liveScrollBtn">&#x81EA;&#x52A8;&#x6EDA;&#x52A8;: &#x5F00;</button></div></div>
      <div class="panel-body"><div class="live-feed" id="liveFeed"><div class="notice">&#x7B49;&#x5F85;&#x65B0;&#x6D88;&#x606F;...</div></div></div></div>
    </section>

    <!-- ANTIBAN -->
    <section id="antiban" class="view">
      <div class="grid grid-2">
        <div class="panel"><div class="panel-head"><h3>&#x56DE;&#x590D;&#x5EF6;&#x8FDF;</h3><p class="panel-sub">&#x6A21;&#x62DF;&#x4EBA;&#x5DE5;&#x6253;&#x5B57;&#xFF0C;&#x968F;&#x673A;&#x5EF6;&#x8FDF;&#x56DE;&#x590D;&#x3002;</p></div>
        <div class="panel-body"><div class="field"><label class="field-label">&#x6700;&#x5C0F;&#x5EF6;&#x8FDF;(ms)</label><input id="abDelayMin" type="number" min="0" step="500"></div><div class="field"><label class="field-label">&#x6700;&#x5927;&#x5EF6;&#x8FDF;(ms)</label><input id="abDelayMax" type="number" min="0" step="500"></div></div></div>
        <div class="panel"><div class="panel-head"><h3>&#x9891;&#x7387;&#x9650;&#x5236;</h3><p class="panel-sub">&#x9632;&#x6B62;&#x5355;&#x53F7;&#x53D1;&#x9001;&#x8FC7;&#x591A;&#x6D88;&#x606F;&#x88AB;&#x68C0;&#x6D4B;&#x3002;</p></div>
        <div class="panel-body"><div class="field"><label class="field-label">&#x6BCF;&#x5C0F;&#x65F6;&#x4E0A;&#x9650;</label><input id="abRateHour" type="number" min="0"></div><div class="field"><label class="field-label">&#x6BCF;&#x5929;&#x4E0A;&#x9650;</label><input id="abRateDay" type="number" min="0"></div></div></div>
      </div>
      <div class="grid grid-2 mt-3">
        <div class="panel"><div class="panel-head"><h3>&#x65B0;&#x53F7;&#x9884;&#x70ED;</h3><p class="panel-sub">&#x65B0;&#x8D26;&#x53F7;&#x9010;&#x6B65;&#x653E;&#x5F00;&#x6D88;&#x606F;&#x53D1;&#x9001;&#x3002;</p></div>
        <div class="panel-body"><div class="field"><label><input id="abWarmupEnabled" type="checkbox"> &#x542F;&#x7528;&#x9884;&#x70ED;</label></div><div class="field"><label class="field-label">&#x9884;&#x70ED;&#x65F6;&#x957F;(&#x5C0F;&#x65F6;)</label><input id="abWarmupHours" type="number" min="1" max="168"></div></div></div>
        <div class="panel"><div class="panel-head"><h3>&#x4F1A;&#x8BDD;&#x4FDD;&#x6E29;</h3><p class="panel-sub">&#x5B9A;&#x671F;&#x5FC3;&#x8DF3;&#x4FDD;&#x6301; WebSocket &#x8FDE;&#x63A5;&#x3002;</p></div>
        <div class="panel-body"><div class="field"><label><input id="abKeepalive" type="checkbox"> &#x542F;&#x7528;&#x5FC3;&#x8DF3;</label></div><div class="field"><label class="field-label">&#x5FC3;&#x8DF3;&#x95F4;&#x9694;(&#x5206;&#x949F;)</label><input id="abKeepaliveInterval" type="number" min="1" max="30"></div></div></div>
      </div>
      <div class="mt-3"><button onclick="saveAntiBan()">&#x4FDD;&#x5B58;&#x7B56;&#x7565;</button><span class="text-muted" id="antibanStatus" style="margin-left:12px"></span></div>
    </section>

    <!-- SETTINGS -->
    <section id="settings" class="view">
      <div class="grid grid-2">
        <div class="panel"><div class="panel-head"><h3>&#x5DE5;&#x4F5C;&#x65F6;&#x95F4;</h3><p class="panel-sub">&#x8BBE;&#x7F6E;&#x5BA2;&#x670D;&#x5728;&#x7EBF;&#x65F6;&#x95F4;&#xFF0C;&#x975E;&#x5DE5;&#x4F5C;&#x65F6;&#x95F4;&#x81EA;&#x52A8;&#x56DE;&#x590D;&#x4E0D;&#x540C;&#x5185;&#x5BB9;&#x3002;</p></div>
        <div class="panel-body"><div class="field"><label class="field-label">&#x5F00;&#x59CB;&#x65F6;&#x95F4;</label><input id="setWorkStart" type="text" placeholder="09:00"></div><div class="field"><label class="field-label">&#x7ED3;&#x675F;&#x65F6;&#x95F4;</label><input id="setWorkEnd" type="text" placeholder="18:00"></div><div class="field"><label class="field-label">&#x65F6;&#x533A;</label><input id="setTimezone" type="text" placeholder="Asia/Shanghai"></div></div></div>
        <div class="panel"><div class="panel-head"><h3>&#x81EA;&#x52A8;&#x56DE;&#x590D;&#x6A21;&#x677F;</h3><p class="panel-sub">&#x975E;&#x5DE5;&#x4F5C;&#x65F6;&#x95F4;&#x548C;&#x9996;&#x6B21;&#x95EE;&#x5019;&#x7684;&#x81EA;&#x52A8;&#x56DE;&#x590D;&#x3002;</p></div>
        <div class="panel-body"><div class="field"><label class="field-label">&#x975E;&#x5DE5;&#x4F5C;&#x65F6;&#x95F4;&#x56DE;&#x590D;</label><textarea id="setOutOfHours" style="min-height:80px"></textarea></div><div class="field"><label class="field-label">&#x9996;&#x6B21;&#x95EE;&#x5019;</label><textarea id="setGreeting" style="min-height:60px"></textarea></div><div class="field"><label class="field-label">&#x9ED8;&#x8BA4;&#x6BCF;&#x65E5;&#x9650;&#x989D;</label><input id="setDefaultLimit" type="number" min="0"></div></div></div>
      </div>
      <div class="mt-3"><button onclick="saveSettings()">&#x4FDD;&#x5B58;&#x8BBE;&#x7F6E;</button><span class="text-muted" id="settingsStatus" style="margin-left:12px"></span></div>
    </section>

    <!-- AUDIT -->
    <section id="audit" class="view">
      <div class="panel"><div class="panel-head"><div><h3>&#x64CD;&#x4F5C;&#x65E5;&#x5FD7;</h3><p class="panel-sub">&#x8BB0;&#x5F55;&#x6240;&#x6709;&#x914D;&#x7F6E;&#x53D8;&#x66F4;&#x64CD;&#x4F5C;&#xFF0C;&#x4FBF;&#x4E8E;&#x5BA1;&#x8BA1;&#x8FFD;&#x8E2A;&#x3002;</p></div><span class="badge badge-info" id="auditCount">0 &#x6761;</span></div>
      <div class="panel-body">
        <div class="table-wrap"><table><thead><tr><th>&#x65F6;&#x95F4;</th><th>&#x64CD;&#x4F5C;</th><th>&#x8BE6;&#x60C5;</th><th>IP</th></tr></thead><tbody id="auditBody"><tr><td colspan="4" class="text-muted">&#x52A0;&#x8F7D;&#x4E2D;...</td></tr></tbody></table></div>
        <div class="pager"><button class="btn-secondary" onclick="changeAuditPage(-1)">&#x4E0A;&#x4E00;&#x9875;</button><span class="text-muted" id="auditPageInfo">&#x7B2C; 1 / 1 &#x9875;</span><button class="btn-secondary" onclick="changeAuditPage(1)">&#x4E0B;&#x4E00;&#x9875;</button></div>
      </div></div>
    </section>

    <!-- ALERTS -->
    <section id="alerts" class="view">
      <div class="panel"><div class="panel-head"><div><h3>&#x544A;&#x8B66;&#x901A;&#x77E5;</h3><p class="panel-sub">&#x7CFB;&#x7EDF;&#x81EA;&#x52A8;&#x68C0;&#x6D4B;&#x8D26;&#x53F7;&#x5F02;&#x5E38;&#x5E76;&#x751F;&#x6210;&#x544A;&#x8B66;&#x3002;</p></div><div style="display:flex;gap:10px"><span class="badge badge-err" id="alertUnread">0 &#x672A;&#x8BFB;</span><button class="btn-secondary btn-sm" onclick="markAllAlertsRead()">&#x5168;&#x90E8;&#x5DF2;&#x8BFB;</button></div></div>
      <div class="panel-body"><div id="alertsList" class="grid gap-1"><div class="notice">&#x52A0;&#x8F7D;&#x4E2D;...</div></div></div></div>
    </section>

    <!-- SANDBOX -->
    <section id="sandbox" class="view">
      <div class="grid grid-2">
        <div class="panel"><div class="panel-head"><h3>&#x5FEB;&#x6377;&#x6D4B;&#x8BD5;</h3><p class="panel-sub">&#x6A21;&#x62DF;&#x5BA2;&#x6237;&#x53D1;&#x9001;&#x6D88;&#x606F;&#xFF0C;&#x67E5;&#x770B;&#x5BA2;&#x670D;&#x56DE;&#x590D;&#x3002;</p></div>
        <div class="panel-body">
          <div class="field"><label class="field-label">&#x9009;&#x62E9;&#x8D26;&#x53F7;</label><select id="sandboxAccount" style="min-width:200px" onchange="loadSandboxHistory()"><option value="">&#x8BF7;&#x9009;&#x62E9;...</option></select></div>
          <div class="field"><label class="field-label">&#x6D4B;&#x8BD5;&#x6D88;&#x606F;</label><textarea id="sandboxMessage" style="min-height:80px" placeholder="&#x8F93;&#x5165;&#x5BA2;&#x6237;&#x95EE;&#x9898;..."></textarea></div>
          <button onclick="sendSandboxMessage()" id="sandboxSendBtn">&#x53D1;&#x9001;&#x6D4B;&#x8BD5;</button>
          <div id="sandboxReply" class="mt-2"></div>
        </div></div>
        <div class="panel"><div class="panel-head"><h3>&#x6D4B;&#x8BD5;&#x5386;&#x53F2;</h3><p class="panel-sub">&#x6700;&#x8FD1;20&#x6761;&#x6D4B;&#x8BD5;&#x8BB0;&#x5F55;&#x3002;</p></div>
        <div class="panel-body"><div id="sandboxHistory" class="grid gap-1"><div class="notice">&#x9009;&#x62E9;&#x8D26;&#x53F7;&#x540E;&#x52A0;&#x8F7D;&#x5386;&#x53F2;...</div></div></div></div>
      </div>
    </section>
  </main>
</div>

<script>
/* ===== STATE ===== */
const S={
  accounts:{}, roles:[], bases:[], entries:[],
  msgs:{rows:[],page:1,pageSize:20,total:0,totalPages:1,accountKey:'',query:''},
  qr:{key:null,poll:null},
  auditPage:1, auditTotal:1,
  custPage:1, custTotal:1,
  liveSince:'', liveAuto:true, livePoll:null,
  activeAlerts:0, errorRate:0
};

const PAGES={
  overview:['总览','查看账号可用性、今日回复量和知识库覆盖情况。'],
  accounts:['客服账号','新增 WhatsApp 客服、扫码登录、设置每日回复上限和分配角色。'],
  knowledge:['知识库','管理知识角色和知识库，在知识库中直接上传内容。'],
  models:['模型配置','编辑 OpenClaw providers 配置，保存后重启生效。'],
  messages:['问答历史','查看客户每一次询问和客服每一次回复。'],
  customers:['客户列表','聚合客户对话记录，查看客户互动历史。'],
  live:['实时监控','实时监控客服状态和消息流量，2秒自动刷新。'],
  antiban:['防封策略','配置回复延迟、频率限制、新号预热等防封策略。'],
  settings:['设置','配置工作时间、自动回复模板和全局默认值。'],
  audit:['操作日志','配置变更审计追踪。'],
  alerts:['告警通知','系统自动告警：账号断连、限额到达等。'],
  sandbox:['快捷测试','模拟客户发送消息，快速验证客服回复效果。']
};

/* ===== HELPERS ===== */
function esc(v){return String(v??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]))}
function fmtTime(v){return v?new Date(v).toLocaleString():'-'}
function fmtDate(v){return v?new Date(v).toLocaleDateString():'-'}
function $(id){return document.getElementById(id)}
function setSync(t){$('syncStatus').textContent=t}

async function fetchJson(url,timeoutMs=8000){
  const c=new AbortController();const t=setTimeout(()=>c.abort(),timeoutMs);
  try{const r=await fetch(url,{signal:c.signal});const x=await r.text();if(!r.ok)throw new Error(x||('HTTP '+r.status));return JSON.parse(x)}finally{clearTimeout(t)}
}
async function postJson(url,body){
  const r=await fetch(url,{method:'POST',headers:{'content-type':'application/json'},body:JSON.stringify(body)});
  const x=await r.text();if(!r.ok)throw new Error(x);return x?JSON.parse(x):{}
}
async function putJson(url,body){
  const r=await fetch(url,{method:'PUT',headers:{'content-type':'application/json'},body:JSON.stringify(body)});
  const x=await r.text();if(!r.ok)throw new Error(x);return x?JSON.parse(x):{}
}
async function delJson(url){
  const r=await fetch(url,{method:'DELETE'});
  if(!r.ok){const x=await r.text();throw new Error(x)}return r.status!==204?r.json():{}
}
async function logout(){await postJson('/admin-api/auth/logout',{});location.href='/login'}

/* ===== TOAST ===== */
function showToast(msg,type){
  const t=document.createElement('div');t.className='toast'+(type?' '+type:'');
  t.textContent=msg;$('toastContainer').appendChild(t);
  setTimeout(()=>{t.style.animation='toastOut .25s ease forwards';setTimeout(()=>t.remove(),250)},3000)
}

/* ===== CONFIRM MODAL ===== */
let confirmCb=null;
function showConfirm(title,body,cb){
  $('confirmTitle').textContent=title;$('confirmBody').textContent=body;
  confirmCb=cb;$('confirmModal').classList.remove('hidden')
}
function closeConfirm(){$('confirmModal').classList.add('hidden');confirmCb=null}
$('confirmOk').addEventListener('click',()=>{if(confirmCb)confirmCb();closeConfirm()})
$('confirmModal').addEventListener('click',function(e){if(e.target===this)closeConfirm()})
document.addEventListener('keydown',function(e){if(e.key==='Escape'){closeConfirm();closeQrModal();closeRoleModal();closeBaseModal();closeEditAccountModal();closeCreateAccountModal()}})

/* ===== EVENT DELEGATION ===== */
document.addEventListener('click',function(e){
  const btn=e.target.closest('[data-action]');
  if(!btn)return;
  const action=btn.dataset.action;
  const id=btn.dataset.id;
  const extra=btn.dataset.extra;
  if(action==='startQr')startQr(id);
  else if(action==='openEditAccount')openEditAccount(id);
  else if(action==='deleteAccount')deleteAccount(id);
  else if(action==='toggleAccount')toggleAccount(id,extra!=='false');
  else if(action==='openRoleModal')openRoleModal(id);
  else if(action==='deleteRole')deleteRole(id);
  else if(action==='openBaseModal')openBaseModal(id);
  else if(action==='deleteBase')deleteBase(id);
  else if(action==='deleteEntry')deleteEntry(id);
  else if(action==='markAlertRead')markAlertRead(id);
});

/* ===== NAVIGATION ===== */
function showView(id){
  if(!PAGES[id])id='overview';
  document.querySelectorAll('.view').forEach(v=>v.classList.toggle('active',v.id===id));
  document.querySelectorAll('.nav-btn').forEach(b=>b.classList.toggle('active',b.dataset.view===id));
  $('pageTitle').textContent=PAGES[id][0];$('pageDesc').textContent=PAGES[id][1];
  if(location.hash.slice(1)!==id)history.replaceState(null,'','#'+id);
  if(id==='settings')loadSettings().catch(()=>{});
  if(id==='audit'){S.auditPage=1;loadAudit().catch(()=>{})}
  if(id==='customers'){S.custPage=1;loadCustomers().catch(()=>{})}
  if(id==='overview')loadStats().catch(()=>{});
  if(id==='antiban')loadAntiBan().catch(()=>{});
  if(id==='alerts')loadAlerts().catch(()=>{});
  if(id==='models')loadModels().catch(()=>{});
  if(id==='knowledge'){loadRoles().catch(()=>{});loadBases().catch(()=>{})}
  if(id==='live'){
    S.liveSince=new Date().toISOString();resetLiveFeed();
    if(S.livePoll)clearInterval(S.livePoll);
    S.livePoll=setInterval(()=>loadLive().catch(()=>{}),2000);loadLive().catch(()=>{})
  }else if(S.livePoll){clearInterval(S.livePoll);S.livePoll=null}
  if(id==='sandbox'){
    const sel=$('sandboxAccount');if(sel&&!sel.options.length)Object.entries(S.accounts).forEach(([k,a])=>sel.innerHTML+='<option value="'+esc(k)+'">'+esc(a.label||k)+'</option>');
    loadSandboxHistory().catch(()=>{})
  }
}
document.querySelectorAll('.nav-btn').forEach(b=>b.addEventListener('click',()=>showView(b.dataset.view)));
showView(location.hash.slice(1)||'overview');

/* ===== QR MODAL ===== */
function showQrModal(imgUrl,status,key){
  const img=$('qrImage');const load=$('qrLoading');const st=$('qrStatus');
  if(imgUrl){img.src=imgUrl;img.style.display='block';load.style.display='none'}else{img.style.display='none';load.style.display='block'}
  st.textContent=status||'等待扫码...';st.className='qr-status';
  if(status==='linked')st.style.color='var(--success)';else if(status==='failed')st.style.color='var(--error)';else st.style.color='var(--warning)';
  $('qrModal').classList.remove('hidden')
}
function closeQrModal(){
  $('qrModal').classList.add('hidden');
  if(S.qr.poll){clearInterval(S.qr.poll);S.qr.poll=null}S.qr.key=null
}
async function pollQrStatus(key){
  const poll=setInterval(async()=>{
    try{
      const r=await fetchJson('/api/accounts/'+encodeURIComponent(key)+'/qr-status');
      const img=$('qrImage');const load=$('qrLoading');const st=$('qrStatus');
      if(r.qrDataUrl&&img.style.display==='none'){img.src=r.qrDataUrl;img.style.display='block';load.style.display='none'}
      st.textContent=r.status==='linked'?'已关联成功':r.status==='failed'?'关联失败: '+(r.message||''):r.message||'等待扫码...';
      st.style.color=r.status==='linked'?'var(--success)':r.status==='failed'?'var(--error)':'var(--warning)';
      if(r.status==='linked'||r.status==='failed'){
        clearInterval(poll);S.qr.poll=null;
        if(r.status==='linked'){showToast('扫码成功，客服已关联','success');setTimeout(()=>closeQrModal(),1500)}
        loadAccounts()
      }
    }catch(e){console.error(e)}
  },2500);
  S.qr.poll=poll;S.qr.key=key
}

/* ===== ACCOUNTS ===== */
async function loadAccounts(){
  try{const d=await fetchJson('/api/accounts');const arr=d.accounts||[];S.accounts={};arr.forEach(a=>{S.accounts[a.id||a.accountKey]=a});renderAccounts();renderMetrics();renderOverviewAccounts();renderMessageAccountOptions();renderLiveAccountOptions();renderNewAccountRoles();setSync('账号 '+new Date().toLocaleTimeString())}catch(e){console.error(e)}
}
function renderAccounts(){
  const entries=Object.entries(S.accounts);
  if(!entries.length){$('accountsList').innerHTML='<div class="empty-state"><div class="empty-icon">&#x1F464;</div><div class="empty-title">还没有配置账号</div><p class="text-muted">点击上方新增表单创建第一个 WhatsApp 客服账号。</p></div>';return}
  const h=$('accountHealth');const bad=entries.filter(([,a])=>!a.live||!a.live.connected||!a.live.healthy).length;
  h.className='badge '+(bad?'badge-err':'badge-ok');h.textContent=bad?bad+' 个异常':'全部正常';
  $('accountsList').innerHTML=entries.map(([k,a])=>renderAccountCard(k,a)).join('')
}
function renderAccountCard(key,a){
  const ok=a.live&&a.live.connected&&a.live.healthy;
  const linked=a.live&&a.live.linked;
  let statusBadge='',statusText='';
  if(ok){statusBadge='badge-ok';statusText='在线'}
  else if(!a.enabled){statusBadge='badge-warn';statusText='已停用'}
  else if(linked){statusBadge='badge-warn';statusText='重连中'}
  else{statusBadge='badge-err';statusText='异常'}
  if(!ok&&!linked&&a.enabled===false){statusBadge='badge-warn';statusText='已停用'}
  if(!ok&&!linked&&a.enabled!==false&&!a.live){statusBadge='badge-muted';statusText='待关联'}

  const used=Number(a.usedToday||0),limit=Number(a.dailyLimit||0);
  const pct=limit?Math.min(Math.round(used/limit*100),100):0;
  const tone=pct>=90?'danger':pct>=70?'warn':'';
  const roles=(a.allowedProducts||[]).map(p=>'<span class="chip">'+esc(p)+'</span>').join('')||'<span class="chip">未绑定知识</span>';
  const phone=a.displayPhone||'未关联号码';

  return '<div class="card">'+
    '<div class="card-row"><div><div class="card-title">'+esc(a.label||key)+'</div><div class="card-meta">'+esc(key)+' &middot; '+esc(phone)+'</div></div><span class="badge '+statusBadge+'">'+statusText+'</span></div>'+
    '<div class="mt-1">'+roles+'</div>'+
    '<div class="card-meta mt-1">今日回复 '+used+' / '+(limit||'不限')+' <span style="float:right">'+pct+'%</span></div>'+
    '<div class="bar"><span class="bar-fill '+tone+'" style="width:'+pct+'%"></span></div>'+
    '<div class="card-actions">'+
      '<button class="btn-secondary btn-sm" data-action="startQr" data-id="'+esc(key)+'">&#x1F4F7; 扫码登录</button>'+
      '<button class="btn-ghost btn-sm" data-action="openEditAccount" data-id="'+esc(key)+'">&#x270F;&#xFE0F; 编辑</button>'+
      '<button class="btn-ghost btn-sm" data-action="toggleAccount" data-id="'+esc(key)+'" data-extra="'+(!a.enabled)+'">'+(a.enabled?'&#x23F9;&#xFE0F; 停用':'&#x25B6;&#xFE0F; 启用')+'</button>'+
      '<button class="btn-ghost btn-sm" style="color:var(--error)" data-action="deleteAccount" data-id="'+esc(key)+'">&#x1F5D1;&#xFE0F; 删除</button>'+
    '</div></div>'
}

function openCreateAccountModal(){
  $('newLabel').value='';$('newLimit').value=30;
  renderNewAccountRoles();$('createAccountModal').classList.remove('hidden')
}
function closeCreateAccountModal(){$('createAccountModal').classList.add('hidden')}
$('createAccountModal').addEventListener('click',function(e){if(e.target===this)closeCreateAccountModal()});

async function createAccount(){
  const label=$('newLabel').value.trim();
  if(!label){showToast('请输入客服名称','error');return}
  const dailyLimit=Number($('newLimit').value);
  if(!dailyLimit||dailyLimit<1){showToast('每日上限至少为1','error');return}
  const roles=[...document.querySelectorAll('[data-role]:checked')].map(el=>el.value);
  if(!roles.length){showToast('请至少勾选一个角色','error');return}
  try{
    await postJson('/api/accounts',{label,dailyLimit,roles});
    await loadAccounts();
    showToast('客服已创建，请点击扫码登录关联 WhatsApp','success');
    closeCreateAccountModal()
  }catch(e){showToast('创建失败: '+(e.message||e),'error')}
}
async function startQr(key){
  try{const r=await postJson('/api/accounts/'+encodeURIComponent(key)+'/qr',{});showQrModal(r.qrDataUrl||'',r.status||'等待扫码...',key);pollQrStatus(key)}catch(e){showToast('生成二维码失败: '+(e.message||e),'error')}
}
async function toggleAccount(key,enabled){
  try{await putJson('/api/accounts/'+encodeURIComponent(key)+'/toggle',{enabled});await loadAccounts();showToast(enabled?'账号已启用':'账号已停用','success')}catch(e){showToast('操作失败: '+(e.message||e),'error')}
}
async function deleteAccount(key){
  const a=S.accounts[key];const label=a?a.label||key:key;
  showConfirm('确认删除','确定要删除 "'+label+'" 吗？此操作不可撤销。',async()=>{
    try{await delJson('/api/accounts/'+encodeURIComponent(key));await loadAccounts();showToast('已删除 '+label,'success')}catch(e){showToast('删除失败: '+(e.message||e),'error')}
  })
}

/* ===== ACCOUNT EDIT MODAL ===== */
function openEditAccount(key){
  const a=S.accounts[key];if(!a)return;
  $('editAccountKey').value=key;$('editAccountLabel').value=a.label||'';
  $('editAccountLimit').value=a.dailyLimit||30;
  const assigned=new Set(a.allowedProducts||[]);
  $('editAccountRoles').innerHTML=S.roles.map(r=>'<label class="chip"><input type="checkbox" data-edit-role value="'+esc(r.id)+'" '+(assigned.has(r.id)?'checked':'')+'>'+esc(r.name)+'</label>').join('')||'<span class="text-muted">暂无可绑定角色</span>';
  $('editAccountModal').classList.remove('hidden')
}
function closeEditAccountModal(){$('editAccountModal').classList.add('hidden')}
async function saveAccountEdit(){
  const key=$('editAccountKey').value;
  const label=$('editAccountLabel').value.trim();
  const dailyLimit=Number($('editAccountLimit').value||30);
  const roles=[...document.querySelectorAll('[data-edit-role]:checked')].map(el=>el.value);
  try{await putJson('/api/accounts/'+encodeURIComponent(key),{label,dailyLimit,roles});await loadAccounts();showToast('账号已更新','success');closeEditAccountModal()}catch(e){showToast('更新失败: '+(e.message||e),'error')}
}
$('editAccountModal').addEventListener('click',function(e){if(e.target===this)closeEditAccountModal()})

/* ===== KNOWLEDGE - ROLES ===== */
document.querySelectorAll('.tab-btn').forEach(b=>b.addEventListener('click',function(){
  document.querySelectorAll('.tab-btn').forEach(x=>x.classList.remove('active'));
  this.classList.add('active');
  document.querySelectorAll('.tab-panel').forEach(x=>x.classList.remove('active'));
  $('ktab-'+this.dataset.ktab).classList.add('active');
  if(this.dataset.ktab==='bases'){closeBaseEntries();}
}));

async function loadRoles(){
  try{const d=await fetchJson('/api/knowledge/roles');S.roles=d.roles||[];renderRoles();renderNewAccountRoles()}catch(e){console.error(e)}
}
function renderRoles(){
  if(!S.roles.length){$('roleList').innerHTML='<div class="empty-state"><div class="empty-icon">&#x1F9E0;</div><div class="empty-title">暂无知识角色</div><p class="text-muted">点击"新增角色"创建第一个知识角色。</p></div>';return}
  $('roleList').innerHTML=S.roles.map(r=>'<div class="card"><div class="card-row"><div><div class="card-title">'+esc(r.name)+'</div><div class="card-meta">'+esc(r.id)+' &middot; '+esc(r.description||'')+'</div></div><span class="chip">'+(r.bases||[]).length+' 知识库</span></div><div class="card-actions"><button class="btn-ghost btn-sm" data-action="openRoleModal" data-id="'+esc(r.id)+'">&#x270F;&#xFE0F; 编辑</button><button class="btn-ghost btn-sm" style="color:var(--error)" data-action="deleteRole" data-id="'+esc(r.id)+'">&#x1F5D1;&#xFE0F; 删除</button></div></div>').join('')
}
function openRoleModal(id){
  if(!S.bases.length) loadBases().catch(()=>{});
  const checksBox=$('roleBaseChecks');
  checksBox.innerHTML=S.bases.length?S.bases.map(b=>'<label class="chip"><input type="checkbox" data-role-base value="'+esc(b.id)+'"> '+esc(b.name)+'</label>').join(''):'<span class="text-muted">暂无知识库</span>';
  if(id){
    const r=S.roles.find(x=>x.id===id);if(!r)return;
    $('roleModalTitle').textContent='编辑角色';$('roleId').value=r.id;$('roleId').disabled=true;$('roleName').value=r.name;$('roleDesc').value=r.description||'';$('roleSaveBtn').textContent='更新';
    const associatedIds=new Set((r.bases||[]).map(b=>b.id));
    checksBox.querySelectorAll('input').forEach(cb=>{cb.checked=associatedIds.has(cb.value)});
  }else{
    $('roleModalTitle').textContent='新增角色';$('roleId').value='';$('roleId').disabled=false;$('roleName').value='';$('roleDesc').value='';$('roleSaveBtn').textContent='创建';
  }
  $('roleModal').classList.remove('hidden')
}
function closeRoleModal(){$('roleModal').classList.add('hidden')}
$('roleModal').addEventListener('click',function(e){if(e.target===this)closeRoleModal()})
async function saveRole(){
  const id=$('roleId').value.trim();const name=$('roleName').value.trim();const description=$('roleDesc').value.trim();
  if(!id||!name){showToast('请填写角色ID和名称','error');return}
  const baseIds=[...document.querySelectorAll('[data-role-base]:checked')].map(el=>el.value);
  try{
    if($('roleId').disabled){await putJson('/api/knowledge/roles/'+encodeURIComponent(id),{name,description,baseIds});showToast('角色已更新','success')}
    else{await postJson('/api/knowledge/roles',{id,name,description,baseIds});showToast('角色已创建','success')}
    closeRoleModal();await loadRoles()
  }catch(e){showToast('保存失败: '+(e.message||e),'error')}
}
async function deleteRole(id){
  showConfirm('确认删除','确定要删除角色 "'+id+'" 吗？',async()=>{
    try{await delJson('/api/knowledge/roles/'+encodeURIComponent(id));await loadRoles();showToast('角色已删除','success')}catch(e){showToast('删除失败: '+(e.message||e),'error')}
  })
}

/* ===== KNOWLEDGE - BASES ===== */
async function loadBases(){
  try{const d=await fetchJson('/api/knowledge/bases');S.bases=d.bases||[];renderBases()}catch(e){console.error(e)}
}
function renderBases(){
  if(!S.bases.length){$('baseList').innerHTML='<div class="empty-state"><div class="empty-icon">&#x1F4DA;</div><div class="empty-title">暂无知识库</div><p class="text-muted">点击"新增知识库"创建第一个。</p></div>';$('baseEntriesSection').style.display='none';return}
  $('baseList').innerHTML=S.bases.map(b=>'<div class="card"><div class="card-row"><div><div class="card-title">'+esc(b.name)+'</div><div class="card-meta">类型: '+esc(b.type||'faq')+' &middot; 角色: '+(b.roles||[]).map(rid=>esc(rid)).join(', ')+' &middot; 条目: '+(b.entryCount||0)+'</div></div></div><div class="card-actions"><button class="btn-secondary btn-sm" onclick="openBaseEntries(\\''+esc(b.id)+'\\')">&#x1F4DD; 管理内容</button><button class="btn-ghost btn-sm" data-action="openBaseModal" data-id="'+esc(b.id)+'">&#x270F;&#xFE0F; 编辑</button><button class="btn-ghost btn-sm" style="color:var(--error)" data-action="deleteBase" data-id="'+esc(b.id)+'">&#x1F5D1;&#xFE0F; 删除</button></div></div>').join('')
}
function openBaseModal(id){
  if(id){const b=S.bases.find(x=>x.id===id);if(!b)return;$('baseModalTitle').textContent='编辑知识库';$('baseName').value=b.name;$('baseType').value=b.type||'faq';$('baseDesc').value=b.description||'';$('baseSaveBtn').textContent='更新';$('baseSaveBtn').dataset.baseId=id}else{$('baseModalTitle').textContent='新增知识库';$('baseName').value='';$('baseType').value='faq';$('baseDesc').value='';$('baseSaveBtn').textContent='创建';delete $('baseSaveBtn').dataset.baseId}
  $('baseModal').classList.remove('hidden')
}
function closeBaseModal(){$('baseModal').classList.add('hidden')}
$('baseModal').addEventListener('click',function(e){if(e.target===this)closeBaseModal()})
async function saveBase(){
  const name=$('baseName').value.trim();const type=$('baseType').value;const description=$('baseDesc').value.trim();
  if(!name){showToast('请填写知识库名称','error');return}
  try{
    const existingId=$('baseSaveBtn').dataset.baseId;
    let savedId=existingId;
    if(existingId){
      await putJson('/api/knowledge/bases/'+encodeURIComponent(existingId),{name,type,description});
    }else{
      const id=name.replace(/[^a-z0-9\\u4e00-\\u9fff-]/gi,'-').toLowerCase().replace(/-+/g,'-').replace(/^-|-$/g,'');
      const r=await postJson('/api/knowledge/bases',{id,name,type,description});
      savedId=r.id||id;
    }
    showToast('知识库已保存','success');closeBaseModal();await loadBases();
    // 闭环：自动打开内容管理
    setTimeout(()=>{openBaseEntries(savedId)},300);
  }catch(e){showToast('保存失败: '+(e.message||e),'error')}
}
async function deleteBase(id){
  const b=S.bases.find(x=>x.id===id);const name=b?b.name:id;
  showConfirm('确认删除','确定要删除知识库 "'+name+'" 吗？所有条目将被一并删除。',async()=>{
    try{await delJson('/api/knowledge/bases/'+encodeURIComponent(id));await loadBases();showToast('知识库已删除','success')}catch(e){showToast('删除失败: '+(e.message||e),'error')}
  })
}

/* ===== KNOWLEDGE - ENTRIES (inline in bases tab) ===== */
let S_activeBaseId=null;
function openBaseEntries(baseId){
  S_activeBaseId=baseId;
  const b=S.bases.find(x=>x.id===baseId);
  $('baseEntriesTitle').textContent='知识条目 — '+(b?b.name:baseId);
  $('baseEntriesSection').style.display='block';
  $('baseEntriesSection').scrollIntoView({behavior:'smooth'});
  loadBaseEntries();
}
function closeBaseEntries(){S_activeBaseId=null;$('baseEntriesSection').style.display='none'}
async function loadBaseEntries(){
  if(!S_activeBaseId){$('entryBody').innerHTML='<tr><td colspan="4" class="text-muted">请先选择知识库</td></tr>';return}
  try{const d=await fetchJson('/api/knowledge/bases/'+encodeURIComponent(S_activeBaseId)+'/entries');S.entries=d.entries||[];renderBaseEntries()}catch(e){$('entryBody').innerHTML='<tr><td colspan="4" class="text-muted">加载失败</td></tr>'}
}
function renderBaseEntries(){
  const rows=S.entries;if(!rows.length){$('entryBody').innerHTML='<tr><td colspan="4" class="text-muted">暂无条目，请添加或导入CSV</td></tr>';return}
  $('entryBody').innerHTML=rows.map((e,i)=>'<tr><td>'+esc(e.title||'')+'</td><td>'+esc((e.content||'').slice(0,100))+'</td><td>'+esc(e.category_id||'')+'</td><td><button class="btn-ghost btn-sm" style="color:var(--error)" onclick="deleteBaseEntry(\\''+esc(e.id)+'\\')">&#x1F5D1;&#xFE0F;</button></td></tr>').join('')
}
async function addBaseEntry(){
  if(!S_activeBaseId){showToast('请先选择知识库','error');return}
  const q=$('newEntryQ').value.trim();const a=$('newEntryA').value.trim();
  if(!q||!a){showToast('请填写标题和内容','error');return}
  try{await postJson('/api/knowledge/bases/'+encodeURIComponent(S_activeBaseId)+'/entries',{title:q,content:a});$('newEntryQ').value='';$('newEntryA').value='';await loadBaseEntries();await loadBases();showToast('条目已添加','success')}catch(e){showToast('添加失败: '+(e.message||e),'error')}
}
async function deleteBaseEntry(id){
  try{await delJson('/api/knowledge/entries/'+encodeURIComponent(id));await loadBaseEntries();await loadBases();showToast('条目已删除','success')}catch(e){showToast('删除失败: '+(e.message||e),'error')}
}
async function importBaseCsv(){
  if(!S_activeBaseId){showToast('请先选择知识库','error');return}
  const file=$('csvFile').files[0];if(!file){showToast('请选择CSV文件','error');return}
  try{
    const text=await file.text();await postJson('/api/knowledge/bases/'+encodeURIComponent(S_activeBaseId)+'/import-csv',{csv:text});
    $('csvFile').value='';await loadBaseEntries();await loadBases();showToast('CSV导入成功','success')
  }catch(e){showToast('导入失败: '+(e.message||e),'error')}
}

/* ===== MESSAGES ===== */
async function loadMessages(){
  if(!S.msgs.accountKey&&Object.keys(S.accounts).length){const e=Object.entries(S.accounts).filter(([,a])=>a.live&&a.live.connected);S.msgs.accountKey=(e[0]||Object.entries(S.accounts)[0])?.[0]||''}
  if(!S.msgs.accountKey){renderMessages();return}
  try{
    const d=await fetchJson('/api/messages?accountKey='+encodeURIComponent(S.msgs.accountKey)+'&query='+encodeURIComponent(S.msgs.query)+'&page='+S.msgs.page+'&pageSize='+S.msgs.pageSize);
    S.msgs.rows=d.conversations||[];S.msgs.total=d.total||0;S.msgs.totalPages=d.totalPages||1;renderMessages();setSync('消息 '+new Date().toLocaleTimeString())
  }catch(e){console.error(e)}
}
function renderMessages(){
  renderMessageAccountOptions();
  $('messageCount').textContent=S.msgs.total+' 条';
  $('messagePageInfo').textContent='第 '+S.msgs.page+' / '+S.msgs.totalPages+' 页';
  const conversations=S.msgs.rows;
  if(!conversations.length){$('messagesList').innerHTML='<div class="notice">暂无问答记录。</div>';return}
  $('messagesList').innerHTML=conversations.map(g=>'<div class="chat-conversation"><div class="chat-divider"><span>'+esc(g.customer||'未知客户')+'</span></div>'+
    (g.messages||[]).map(m=>{
      const isIn=m.direction==='inbound';
      return '<div class="chat-bubble '+(isIn?'inbound':'outbound')+'"><div class="chat-text">'+esc(m.text||'')+'</div><div class="chat-time">'+fmtTime(m.at)+'</div></div>';
    }).join('')+'</div>').join('')
}
function renderMessageAccountOptions(){
  const sel=$('messageAccount');const entries=Object.entries(S.accounts);
  sel.innerHTML=entries.map(([k,a])=>'<option value="'+esc(k)+'" '+(k===S.msgs.accountKey?'selected':'')+'>'+esc(a.label||k)+'</option>').join('')
}
function changeMessageAccount(key){S.msgs.accountKey=key;S.msgs.page=1;loadMessages().catch(()=>{})}
function searchMessages(){S.msgs.query=$('messageSearch').value.trim();S.msgs.page=1;loadMessages().catch(()=>{})}
function clearMessageSearch(){$('messageSearch').value='';S.msgs.query='';S.msgs.page=1;loadMessages().catch(()=>{})}
function changeMessagePageSize(v){S.msgs.pageSize=Number(v)||20;S.msgs.page=1;loadMessages().catch(()=>{})}
function changeMessagePage(d){const n=Math.min(Math.max(1,S.msgs.page+d),S.msgs.totalPages||1);if(n===S.msgs.page)return;S.msgs.page=n;loadMessages().catch(()=>{})}
async function exportMessages(){
  try{const r=await fetch('/admin-api/export/conversations',{method:'POST',headers:{'content-type':'application/json'},body:JSON.stringify({accountKey:S.msgs.accountKey,query:S.msgs.query})});if(!r.ok)throw new Error(await r.text());const blob=await r.blob();const a=document.createElement('a');a.href=URL.createObjectURL(blob);a.download='messages.csv';a.click();URL.revokeObjectURL(a.href);showToast('导出成功','success')}catch(e){showToast('导出失败: '+(e.message||e),'error')}
}

/* ===== CUSTOMERS ===== */
async function loadCustomers(){
  try{
    const sel=$('customerAccount');if(sel&&!sel.options.length)Object.entries(S.accounts).forEach(([k,a])=>sel.innerHTML+='<option value="'+esc(k)+'">'+esc(a.label||k)+'</option>');
    const ak=sel?sel.value:'';
    const d=await fetchJson('/api/customers?accountKey='+encodeURIComponent(ak)+'&page='+S.custPage+'&pageSize=20');
    $('customerBody').innerHTML=d.customers?.length?d.customers.map(c=>'<tr><td class="text-strong">'+esc(c.customer)+'</td><td>'+esc(c.account_key)+'</td><td>'+c.conversation_count+'</td><td class="text-muted">'+fmtDate(c.first_contact)+'</td><td class="text-muted">'+fmtDate(c.last_contact)+'</td><td class="text-muted">'+esc((c.lastMessagePreview||'').slice(0,50))+'</td></tr>').join(''):'<tr><td colspan="6" class="text-muted">暂无客户数据</td></tr>';
    $('customerCount').textContent=(d.total||0)+' 位';$('customerPageInfo').textContent='第 '+d.page+' / '+d.totalPages+' 页';S.custTotal=d.totalPages||1
  }catch(e){console.error(e)}
}
function changeCustomerPage(d){const n=Math.min(Math.max(1,S.custPage+d),S.custTotal||1);if(n===S.custPage)return;S.custPage=n;loadCustomers().catch(()=>{})}

/* ===== LIVE ===== */
function renderLiveAccountOptions(){
  const sel=$('liveAccount');if(!sel)return;const cur=sel.value;
  sel.innerHTML='<option value="">全部</option>'+Object.entries(S.accounts).map(([k,a])=>'<option value="'+esc(k)+'">'+esc(a.label||k)+'</option>').join('');
  if(cur)sel.value=cur
}
async function loadLive(){
  try{
    const ak=$('liveAccount')?.value||'';const d=await fetchJson('/admin-api/messages/live?since='+encodeURIComponent(S.liveSince)+'&accountKey='+encodeURIComponent(ak));
    if(d.messages?.length){
      const feed=$('liveFeed');const wasAtBottom=feed.scrollHeight-feed.scrollTop-feed.clientHeight<50;
      const existing=feed.querySelectorAll('.live-msg').length;
      const html=d.messages.map(m=>'<div class="live-msg"><span class="badge badge-sm '+(m.direction==='inbound'?'badge-info':'badge-ok')+'" style="flex-shrink:0">'+(m.direction==='inbound'?'客户':'客服')+'</span><div style="min-width:0"><div class="text-sm" style="display:flex;justify-content:space-between"><span>'+esc(m.customer||'')+'</span><span class="text-muted">'+new Date(m.at).toLocaleTimeString()+'</span></div><div style="font-size:13px;word-break:break-word">'+esc(m.text||'')+'</div></div></div>').join('');
      if(!existing)feed.innerHTML=html;else feed.insertAdjacentHTML('beforeend',html);
      if(S.liveAuto&&wasAtBottom)feed.scrollTop=feed.scrollHeight
    }
    S.liveSince=d.now||new Date().toISOString()
  }catch(e){console.error(e)}
}
function resetLiveFeed(){S.liveSince='';$('liveFeed').innerHTML='<div class="notice">等待新消息...</div>'}
function toggleLiveAutoScroll(){S.liveAuto=!S.liveAuto;$('liveScrollBtn').textContent='自动滚动: '+(S.liveAuto?'开':'关')}

/* ===== MODELS ===== */
async function loadModels(){
  try{const d=await fetchJson('/api/models');renderModels(d)}catch(e){$('providersConfig').innerHTML='<div class="notice error">加载失败: '+esc(e.message||e)+'</div>'}
}
function renderModels(d){
  const providers=d.providers||{};const allModels=[];
  let html='';
  Object.entries(providers).forEach(([name,p])=>{
    const models=p.models||[];
    models.forEach(m=>allModels.push(m));
    html+='<div class="card mb-2"><div class="card-row"><div class="card-title">'+esc(name)+'</div><span class="chip">'+models.length+' 模型</span></div><div class="card-meta">'+esc(p.baseURL||p.apiBase||'')+'</div><div class="mt-1">'+models.map(m=>'<span class="chip">'+esc(typeof m==='string'?m:m.id||m.name)+'</span>').join('')+'</div>'+
      '<div class="card-actions"><input type="text" placeholder="API Key" value="'+esc(p.apiKey||'')+'" data-provider="'+esc(name)+'" data-field="apiKey" style="min-width:200px;height:30px;font-size:12px">'+
      '<input type="text" placeholder="Base URL" value="'+esc(p.baseURL||p.apiBase||'')+'" data-provider="'+esc(name)+'" data-field="baseURL" style="min-width:260px;height:30px;font-size:12px"></div></div>'
  });
  $('providersConfig').innerHTML=html||'<div class="notice">暂无 provider 配置</div>';
  $('fallbackModel').innerHTML='<option value="">请选择...</option>'+allModels.map(m=>'<option value="'+esc(typeof m==='string'?m:m.id||m.name)+'" '+(d.fallback===m||d.fallback===m.id?'selected':'')+'>'+esc(typeof m==='string'?m:m.id||m.name||m)+'</option>').join('')
}
async function saveModels(){
  try{
    const providers={};
    document.querySelectorAll('[data-provider]').forEach(el=>{
      const name=el.dataset.provider;const field=el.dataset.field;
      if(!providers[name])providers[name]={};
      providers[name][field]=el.value
    });
    const fallback=$('fallbackModel').value;
    await postJson('/api/models',{providers,fallback});
    $('modelStatus').textContent='已保存，重启后生效';showToast('模型配置已保存','success');
    setTimeout(()=>$('modelStatus').textContent='',3000)
  }catch(e){showToast('保存失败: '+(e.message||e),'error')}
}

/* ===== ANTIBAN ===== */
async function loadAntiBan(){
  try{const d=await fetchJson('/api/antiban');const c=d.config||d;
    $('abDelayMin').value=c.replyDelay?.min||0;$('abDelayMax').value=c.replyDelay?.max||0;
    $('abRateHour').value=c.rateLimit?.maxPerHour||0;$('abRateDay').value=c.rateLimit?.maxPerDay||0;
    $('abWarmupEnabled').checked=c.warmup?.enabled||false;$('abWarmupHours').value=c.warmup?.durationHours||0;
    $('abKeepalive').checked=c.sessionKeepalive?.enabled||false;$('abKeepaliveInterval').value=c.sessionKeepalive?.intervalMinutes||0
  }catch(e){console.error(e)}
}
async function saveAntiBan(){
  try{
    const c={replyDelay:{min:Number($('abDelayMin').value),max:Number($('abDelayMax').value)},rateLimit:{maxPerHour:Number($('abRateHour').value),maxPerDay:Number($('abRateDay').value)},warmup:{enabled:$('abWarmupEnabled').checked,durationHours:Number($('abWarmupHours').value)},sessionKeepalive:{enabled:$('abKeepalive').checked,intervalMinutes:Number($('abKeepaliveInterval').value)}};
    await postJson('/api/antiban',{config:c});$('antibanStatus').textContent='已保存';showToast('防封策略已保存','success');setTimeout(()=>$('antibanStatus').textContent='',3000)
  }catch(e){showToast('保存失败: '+(e.message||e),'error')}
}

/* ===== SETTINGS ===== */
async function loadSettings(){
  try{const d=await fetchJson('/api/settings');const s=d.settings||d||{};
    $('setWorkStart').value=s.workingHours?.start||'09:00';$('setWorkEnd').value=s.workingHours?.end||'18:00';$('setTimezone').value=s.workingHours?.timezone||'Asia/Shanghai';
    $('setOutOfHours').value=s.autoReply?.outOfHours||'';$('setGreeting').value=s.autoReply?.greeting||'';$('setDefaultLimit').value=s.globalDefaults?.dailyLimit||30
  }catch(e){console.error(e)}
}
async function saveSettings(){
  const settings={workingHours:{start:$('setWorkStart').value,end:$('setWorkEnd').value,timezone:$('setTimezone').value},autoReply:{outOfHours:$('setOutOfHours').value,greeting:$('setGreeting').value},globalDefaults:{dailyLimit:Number($('setDefaultLimit').value)||30}};
  try{await postJson('/api/settings',{settings});$('settingsStatus').textContent='已保存';showToast('设置已保存','success');setTimeout(()=>$('settingsStatus').textContent='',3000)}catch(e){showToast('保存失败: '+(e.message||e),'error')}
}

/* ===== AUDIT ===== */
async function loadAudit(){
  try{const d=await fetchJson('/admin-api/audit-log?page='+S.auditPage+'&pageSize=30');
    const names={'account.create':'创建账号','account.delete':'删除账号','account.toggle':'启停账号','account.update':'更新账号','knowledge.role.create':'创建角色','knowledge.role.delete':'删除角色','knowledge.base.create':'创建知识库','knowledge.base.delete':'删除知识库','knowledge.entry.create':'添加条目','models.save':'更新模型','settings.save':'修改设置','antiban.save':'更新防封','sandbox.test':'测试消息'};
    $('auditBody').innerHTML=d.logs?.length?d.logs.map(l=>{const tag=l.action?.includes('delete')?'badge-err':l.action?.includes('create')?'badge-ok':l.action?.includes('toggle')?'badge-warn':'';return '<tr><td class="text-muted">'+fmtTime(l.at)+'</td><td><span class="badge badge-sm '+tag+'">'+esc(names[l.action]||l.action)+'</span></td><td>'+esc(l.detail)+'</td><td class="text-muted">'+esc(l.ip)+'</td></tr>'}).join(''):'<tr><td colspan="4" class="text-muted">暂无操作记录</td></tr>';
    $('auditCount').textContent=(d.total||0)+' 条';$('auditPageInfo').textContent='第 '+d.page+' / '+d.totalPages+' 页';S.auditTotal=d.totalPages||1
  }catch(e){console.error(e)}
}
function changeAuditPage(d){const n=Math.min(Math.max(1,S.auditPage+d),S.auditTotal||1);if(n===S.auditPage)return;S.auditPage=n;loadAudit().catch(()=>{})}

/* ===== ALERTS ===== */
async function loadAlerts(){
  try{const d=await fetchJson('/admin-api/alerts');$('alertUnread').textContent=(d.unread||0)+' 未读';
    $('alertsList').innerHTML=d.alerts?.length?d.alerts.map(a=>{const lvl=a.level==='error'?'badge-err':a.level==='warn'?'badge-warn':'';return '<div class="card" style="'+(a.read?'':'border-left:3px solid var(--'+(a.level==='error'?'error':'warning')+')')+'"><div class="card-row"><div><span class="badge badge-sm '+lvl+'">'+esc(a.title)+'</span><span class="text-muted" style="margin-left:8px">'+esc(a.accountKey||'')+'</span></div><span class="text-muted">'+fmtTime(a.at)+'</span></div><p class="text-muted mt-1">'+esc(a.body)+(a.read?'':' <button class="btn-ghost btn-sm" data-action="markAlertRead" data-id="'+esc(a.id)+'">标记已读</button>')+'</p></div>'}).join(''):'<div class="notice">暂无告警，系统运行正常。</div>';
    const unread=d.unread||0;const badge=document.querySelector('[data-view="alerts"] small');if(badge&&unread>0)badge.textContent=unread+' · Alerts'
  }catch(e){console.error(e)}
}
async function markAlertRead(id){try{await postJson('/admin-api/alerts/mark-read',{ids:[id]});loadAlerts().catch(()=>{})}catch(e){console.error(e)}}
async function markAllAlertsRead(){try{await postJson('/admin-api/alerts/mark-read',{all:true});loadAlerts().catch(()=>{})}catch(e){console.error(e)}}

/* ===== SANDBOX ===== */
async function sendSandboxMessage(){
  const ak=$('sandboxAccount').value;const msg=$('sandboxMessage').value.trim();
  if(!ak||!msg){showToast('请选择账号并输入消息','error');return}
  const btn=$('sandboxSendBtn');btn.disabled=true;btn.textContent='发送中...';$('sandboxReply').innerHTML='<div class="notice">等待回复...</div>';
  try{const d=await postJson('/admin-api/sandbox/test',{accountKey:ak,message:msg});$('sandboxReply').innerHTML='<div class="card" style="background:var(--chat-out);border-color:var(--primary-soft)"><div class="card-title" style="color:var(--primary)">客服回复</div><div style="margin-top:8px;line-height:1.6">'+esc(d.reply)+'</div></div>';$('sandboxMessage').value='';loadSandboxHistory()}catch(e){$('sandboxReply').innerHTML='<div class="notice error">'+esc(e.message||e)+'</div>'}finally{btn.disabled=false;btn.textContent='发送测试'}
}
async function loadSandboxHistory(){
  const ak=$('sandboxAccount').value;if(!ak)return;
  try{const d=await fetchJson('/admin-api/sandbox/history?accountKey='+encodeURIComponent(ak));$('sandboxHistory').innerHTML=d.messages?.length?d.messages.slice().reverse().map(m=>'<div class="card"><div class="text-strong" style="color:var(--info)">测试消息</div><div class="text-muted">'+fmtTime(m.at)+'</div><div class="mt-1">'+esc(m.message)+'</div><div class="text-strong mt-1" style="color:var(--primary)">客服回复</div><div class="mt-1">'+esc(m.reply)+'</div></div>').join(''):'<div class="notice">暂无测试记录</div>'}catch(e){console.error(e)}
}

/* ===== OVERVIEW ===== */
function renderMetrics(){
  const a=Object.values(S.accounts);const online=a.filter(x=>x.live&&x.live.connected&&x.live.healthy).length;const used=a.reduce((s,x)=>s+Number(x.usedToday||0),0);const total=a.length;
  $('metrics').innerHTML=[['在线账号',online+'/'+total,total?(online===total?'全部在线':'有离线账号'):'暂无账号'],['今日消息',used,'已发送回复'],['活跃告警',S.activeAlerts||0,(S.activeAlerts?'需要处理':'无告警')],['知识角色',S.roles.length,S.roles.map(r=>r.name).join('、')||'暂无']].map(i=>'<div class="metric"><div class="metric-label">'+esc(i[0])+'</div><div class="metric-value">'+esc(i[1])+'</div><div class="metric-note">'+esc(i[2])+'</div></div>').join('')
}
function renderOverviewAccounts(){
  const entries=Object.entries(S.accounts);
  $('overviewAccounts').innerHTML=entries.length?entries.map(([k,a])=>'<div style="display:flex;justify-content:space-between;align-items:center;padding:10px 0;border-bottom:1px solid var(--line-soft)"><div><div class="text-strong">'+esc(a.label||k)+'</div><div class="text-muted">'+esc(a.displayPhone||k)+'</div></div><span class="badge badge-sm '+(a.live&&a.live.healthy?'badge-ok':'badge-err')+'">'+(a.live&&a.live.healthy?'健康':'异常')+'</span></div>').join(''):'<div class="empty-state"><div class="empty-title">还没有配置账号</div><p class="text-muted">前往"客服账号"页面创建第一个 WhatsApp 客服账号。</p></div>'
}
async function loadStats(){
  try{
    const d=await fetchJson('/admin-api/stats/overview');
    const maxVol=Math.max(1,...(d.volumeTrend||[]).map(v=>v.count));
    const bars=(d.volumeTrend||[]).map((v,i)=>{const h=Math.round(v.count/maxVol*160);const x=60+i*48;return '<rect x="'+x+'" y="'+(180-h)+'" width="32" height="'+h+'" rx="4" fill="var(--primary)" opacity="0.8"><title>'+v.date+': '+v.count+' 条</title></rect><text x="'+(x+16)+'" y="195" text-anchor="middle" font-size="11" fill="var(--muted)">'+v.date+'</text><text x="'+(x+16)+'" y="'+(175-h)+'" text-anchor="middle" font-size="10" fill="var(--ink)">'+v.count+'</text>'}).join('');
    $('volumeChart').innerHTML=bars?'<svg viewBox="0 0 400 210" width="100%" height="200">'+bars+'</svg>':'<div class="notice">暂无趋势数据</div>';
    const maxP=Math.max(1,...(d.topProducts||[]).map(p=>p.count));
    const pBars=(d.topProducts||[]).map((p,i)=>{const w=Math.round(p.count/maxP*250);return '<text x="0" y="'+(25+i*34)+'" font-size="12" fill="var(--ink)">'+esc(p.name)+'</text><rect x="120" y="'+(12+i*34)+'" width="'+w+'" height="20" rx="4" fill="var(--info)" opacity="0.7"><title>'+p.count+' 条</title></rect><text x="'+(125+w)+'" y="'+(27+i*34)+'" font-size="11" fill="var(--muted)">'+p.count+'</text>'}).join('');
    $('productChart').innerHTML=pBars?'<svg viewBox="0 0 400 180" width="100%" height="200">'+pBars+'</svg>':'<div class="notice">暂无产品数据</div>';
    S.activeAlerts=d.activeAlerts||0;S.errorRate=d.accountHealth?.errorRate||0;
    $('responseMetrics').innerHTML=[['总对话数',d.responseMetrics?.totalConversations||0,'累计客户提问'],['转人工次数',d.responseMetrics?.handoffCount||0,'触发转人工的对话'],['热门品类',(d.topProducts||[]).length||0,'有客户询问的品类'],['错误率',(S.errorRate||0)+'%',S.errorRate>0?'存在异常':'运行正常']].map(i=>'<div class="metric"><div class="metric-label">'+esc(i[0])+'</div><div class="metric-value">'+esc(i[1])+'</div><div class="metric-note">'+esc(i[2])+'</div></div>').join('')
  }catch(e){console.error(e)}
}

/* ===== NEW ACCOUNT ROLES ===== */
function renderNewAccountRoles(){
  const box=$('newAccountRoles');if(!box)return;
  box.innerHTML=S.roles.length?S.roles.map(r=>'<label class="chip"><input type="checkbox" data-role value="'+esc(r.id)+'"> '+esc(r.name)+'</label>').join(''):'<span class="text-muted">暂无角色</span>'
}

/* ===== INIT ===== */
async function loadAll(){
  try{await Promise.all([loadAccounts(),loadKnowledgeInit()]);await loadMessages();await loadStats()}catch(e){console.error(e)}
}
async function loadKnowledgeInit(){
  try{const d=await fetchJson('/api/knowledge/roles');S.roles=d.roles||[];renderRoles();renderNewAccountRoles()}catch(e){console.error(e)}
}
loadAll();
setInterval(()=>{loadMessages().catch(()=>{})},3000);
setInterval(()=>{loadAccounts().catch(()=>{})},15000);
</script>
</body>
</html>`;
}

export function loginPage() {
  return `<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>WhatsApp AI Ops - Login</title>
<style>:root{--bg:#f5f1e8;--ink:#16211c;--muted:#6f746d;--panel:#fffdf8;--line:#ded7ca;--green:#0f8f68}*{box-sizing:border-box}body{margin:0;display:flex;align-items:center;justify-content:center;min-height:100vh;background:var(--bg);font-family:Inter,system-ui,sans-serif}
.login-card{width:min(360px,90vw);padding:32px;border-radius:12px;background:var(--panel);border:1px solid var(--line);box-shadow:0 12px 30px rgba(36,31,24,.06)}
.login-card h1{margin:0 0 8px;font-size:22px;color:var(--ink)}.login-card p{margin:0 0 20px;color:var(--muted);font-size:14px}
input{width:100%;height:42px;padding:0 14px;border:1px solid var(--line);border-radius:8px;background:#fff;font-size:14px;margin-bottom:14px}
button{width:100%;height:42px;border:0;border-radius:8px;background:var(--ink);color:#fff;font-size:14px;cursor:pointer}
.error{color:#bb3e35;font-size:13px;margin-top:8px;display:none}
</style></head><body><div class="login-card">
<h1>WhatsApp AI Ops</h1><p>&#x8F93;&#x5165;&#x7BA1;&#x7406; Token &#x767B;&#x5F55;</p>
<input id="token" type="password" placeholder="Admin Token" onkeydown="if(event.key==='Enter')login()">
<button onclick="login()">&#x767B;&#x5F55;</button><div class="error" id="error"></div>
</div>
<script>
async function login(){
  try{
    const r=await fetch('/admin-api/auth/login',{method:'POST',headers:{'content-type':'application/json'},body:JSON.stringify({token:document.getElementById('token').value})});
    if(r.ok){location.href='/';return;}
    document.getElementById('error').textContent='Token 错误';
    document.getElementById('error').style.display='block';
  }catch(e){document.getElementById('error').textContent='网络错误';document.getElementById('error').style.display='block';}
}
</script></body></html>`;
}
