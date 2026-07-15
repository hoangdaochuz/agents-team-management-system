/* ============================================================
   AI Agent Ops — shared interactions
   ============================================================ */
(function () {
  "use strict";

  /* ── Agent roster (shared data) ────────────────────────────── */
  const AGENTS = [
    { id: "atlas", initials: "AT", name: "Atlas", role: "Backend engineer", hue: 210 },
    { id: "iris",  initials: "IR", name: "Iris",  role: "Research analyst", hue: 280 },
    { id: "forge", initials: "FO", name: "Forge", role: "Code refactorer",  hue: 150 },
    { id: "echo",  initials: "EC", name: "Echo",  role: "QA & tests",       hue: 32 },
    { id: "nova",  initials: "NO", name: "Nova",  role: "Docs & writer",    hue: 340 },
  ];
  window.AGENTS = AGENTS;

  /* ── Sidebar injection ─────────────────────────────────────── */
  const NAV = [
    { group: "Workspace", items: [
      { id: "dashboard", label: "Dashboard", href: "dashboard.html", icon: ICON("grid") },
      { id: "kanban",    label: "Kanban board", href: "kanban.html", icon: ICON("board") },
      { id: "agents",    label: "Agents", href: "agents.html", icon: ICON("bot") },
    ]},
    { group: "Operations", items: [
      { id: "history",    label: "Run history", href: "history.html", icon: ICON("clock") },
      { id: "settings",   label: "Settings", href: "settings.html", icon: ICON("gear") },
    ]},
  ];

  function ICON(name) {
    const s = 'stroke="currentColor" stroke-width="1.8" fill="none" stroke-linecap="round" stroke-linejoin="round"';
    const map = {
      grid:   `<svg viewBox="0 0 24 24" ${s}><rect x="3" y="3" width="7" height="7" rx="1.5"/><rect x="14" y="3" width="7" height="7" rx="1.5"/><rect x="3" y="14" width="7" height="7" rx="1.5"/><rect x="14" y="14" width="7" height="7" rx="1.5"/></svg>`,
      board:  `<svg viewBox="0 0 24 24" ${s}><rect x="3" y="4" width="5" height="16" rx="1.5"/><rect x="10" y="4" width="5" height="11" rx="1.5"/><rect x="17" y="4" width="4" height="14" rx="1.5"/></svg>`,
      bot:    `<svg viewBox="0 0 24 24" ${s}><rect x="4" y="8" width="16" height="11" rx="3"/><path d="M12 8V4M9 14h.01M15 14h.01M9 19v2M15 19v2"/></svg>`,
      clock:  `<svg viewBox="0 0 24 24" ${s}><circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/></svg>`,
      gear:   `<svg viewBox="0 0 24 24" ${s}><circle cx="12" cy="12" r="3.2"/><path d="M12 2v3M12 19v3M4.2 4.2l2.1 2.1M17.7 17.7l2.1 2.1M2 12h3M19 12h3M4.2 19.8l2.1-2.1M17.7 6.3l2.1-2.1"/></svg>`,
      bell:   `<svg viewBox="0 0 24 24" ${s}><path d="M6 8a6 6 0 1 1 12 0c0 5 2 6 2 6H4s2-1 2-6ZM10 20a2 2 0 0 0 4 0"/></svg>`,
      search: `<svg viewBox="0 0 24 24" ${s}><circle cx="11" cy="11" r="7"/><path d="m20 20-3.2-3.2"/></svg>`,
      plus:   `<svg viewBox="0 0 24 24" ${s}><path d="M12 5v14M5 12h14"/></svg>`,
      pause:  `<svg viewBox="0 0 24 24" ${s}><rect x="6" y="5" width="4" height="14" rx="1"/><rect x="14" y="5" width="4" height="14" rx="1"/></svg>`,
      play:   `<svg viewBox="0 0 24 24" ${s}><path d="M7 4v16l13-8z"/></svg>`,
      check:  `<svg viewBox="0 0 24 24" ${s}><path d="M20 6 9 17l-5-5"/></svg>`,
      send:   `<svg viewBox="0 0 24 24" ${s}><path d="M22 2 11 13M22 2l-7 20-4-9-9-4 20-7Z"/></svg>`,
      stop:   `<svg viewBox="0 0 24 24" ${s}><rect x="6" y="6" width="12" height="12" rx="2"/></svg>`,
      code:   `<svg viewBox="0 0 24 24" ${s}><path d="m8 6-6 6 6 6M16 6l6 6-6 6"/></svg>`,
      file:   `<svg viewBox="0 0 24 24" ${s}><path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8z"/><path d="M14 3v5h5"/></svg>`,
    };
    return map[name] || "";
  }
  window.ICON = ICON;

  function buildSidebar() {
    const page = document.body.dataset.page || "";
    let html = "";
    html += `<div class="brand">
      <div class="brand-mark">◆</div>
      <div><div class="brand-name">Agent Ops</div><div class="brand-sub">Control room</div></div>
    </div>`;
    NAV.forEach((g) => {
      html += `<div>`;
      html += `<div class="nav-group-label">${g.group}</div>`;
      html += `<nav class="nav">`;
      g.items.forEach((it) => {
        const active = it.id === page ? " active" : "";
        html += `<a href="${it.href}" class="${active.trim()}"><span class="nav-ico">${it.icon}</span>${it.label}</a>`;
      });
      html += `</nav></div>`;
    });
    html += `<div class="sidebar-foot">
      <div class="avatar">DA</div>
      <div><div class="who-name">Dang Anh</div><div class="who-role">Workspace owner</div></div>
    </div>`;
    return html;
  }

  function buildTopbar() {
    return `
      <button class="burger" id="burger" aria-label="Menu">${ICON("grid")}</button>
      <div class="search">
        <span style="color:var(--meta)">${ICON("search")}</span>
        <input type="text" placeholder="Search tasks, agents, runs…" />
      </div>
      <div class="topbar-actions">
        <button class="btn btn-ghost btn-sm" onclick="location.href='kanban.html'">${ICON("plus")}<span>New task</span></button>
        <button class="icon-btn" aria-label="Notifications">${ICON("bell")}<span class="dot-badge"></span></button>
      </div>`;
  }

  function initChrome() {
    const sb = document.getElementById("sidebar");
    if (sb) sb.innerHTML = buildSidebar();
    const tb = document.getElementById("topbar");
    if (tb) tb.innerHTML = buildTopbar();
    const burger = document.getElementById("burger");
    if (burger && sb) {
      burger.addEventListener("click", () => sb.classList.toggle("open"));
      document.addEventListener("click", (e) => {
        if (!sb.contains(e.target) && !burger.contains(e.target)) sb.classList.remove("open");
      });
    }
  }

  /* ── Tabs ──────────────────────────────────────────────────── */
  function initTabs() {
    document.querySelectorAll("[data-tabs]").forEach((group) => {
      const tabs = group.querySelectorAll(".tab");
      tabs.forEach((tab) => {
        tab.addEventListener("click", () => {
          tabs.forEach((t) => t.classList.remove("active"));
          tab.classList.add("active");
          const target = tab.getAttribute("data-tab");
          const root = group.closest("[data-tabroot]") || document;
          root.querySelectorAll(":scope > [data-tabpanel], .tab-panel").forEach((p) => p.classList.remove("active"));
          const panel = root.querySelector(`[data-tabpanel="${target}"], #${target}`);
          if (panel) panel.classList.add("active");
        });
      });
    });
  }

  /* ── Segmented controls ────────────────────────────────────── */
  function initSegments() {
    document.querySelectorAll(".seg").forEach((seg) => {
      seg.querySelectorAll("button").forEach((b) => {
        if (b.dataset.segBound) return;
        b.dataset.segBound = "1";
        b.addEventListener("click", () => {
          seg.querySelectorAll("button").forEach((x) => x.classList.remove("active"));
          b.classList.add("active");
        });
      });
    });
  }

  /* ── Switches ──────────────────────────────────────────────── */
  function initSwitches() {
    document.querySelectorAll(".switch").forEach((sw) => {
      sw.setAttribute("role", "switch");
      sw.setAttribute("aria-checked", sw.classList.contains("on") ? "true" : "false");
      sw.addEventListener("click", () => {
        sw.classList.toggle("on");
        sw.setAttribute("aria-checked", sw.classList.contains("on") ? "true" : "false");
      });
    });
  }

  /* ── Toast ─────────────────────────────────────────────────── */
  let toastWrap;
  window.toast = function (msg, opts) {
    opts = opts || {};
    if (!toastWrap) {
      toastWrap = document.createElement("div");
      toastWrap.className = "toast-wrap";
      document.body.appendChild(toastWrap);
    }
    const t = document.createElement("div");
    t.className = "toast";
    const ico = opts.icon ? ICON(opts.icon) : ICON("check");
    t.innerHTML = `<span style="width:16px;height:16px;display:inline-grid;place-items:center">${ico}</span><span>${msg}</span>`;
    toastWrap.appendChild(t);
    setTimeout(() => { t.style.opacity = "0"; t.style.transition = "opacity .2s"; }, 2600);
    setTimeout(() => t.remove(), 2900);
  };

  /* ── Modal ─────────────────────────────────────────────────── */
  window.openModal = function (id) {
    const el = document.getElementById(id);
    if (el) { el.classList.add("open"); document.body.style.overflow = "hidden"; }
  };
  window.closeModal = function (id) {
    const el = document.getElementById(id);
    if (el) { el.classList.remove("open"); document.body.style.overflow = ""; }
  };
  function initModals() {
    document.querySelectorAll(".overlay").forEach((ov) => {
      ov.addEventListener("click", (e) => { if (e.target === ov) ov.classList.remove("open"); });
    });
    document.addEventListener("keydown", (e) => {
      if (e.key === "Escape") document.querySelectorAll(".overlay.open").forEach((ov) => { ov.classList.remove("open"); document.body.style.overflow = ""; });
    });
  }

  /* ── Kanban drag & drop ────────────────────────────────────── */
  function initKanban() {
    const board = document.querySelector("[data-kanban]");
    if (!board) return;
    let dragging = null;
    board.querySelectorAll(".tcard").forEach((card) => {
      card.setAttribute("draggable", "true");
      card.addEventListener("dragstart", () => { dragging = card; setTimeout(() => card.classList.add("dragging"), 0); });
      card.addEventListener("dragend", () => { card.classList.remove("dragging"); reCount(); });
    });
    board.querySelectorAll(".col").forEach((col) => {
      col.addEventListener("dragover", (e) => { e.preventDefault(); col.classList.add("drag-over"); });
      col.addEventListener("dragleave", () => col.classList.remove("drag-over"));
      col.addEventListener("drop", (e) => {
        e.preventDefault();
        col.classList.remove("drag-over");
        const body = col.querySelector(".col-body");
        if (dragging && body) body.appendChild(dragging);
      });
    });
    function reCount() {
      board.querySelectorAll(".col").forEach((col) => {
        const n = col.querySelectorAll(".tcard").length;
        const c = col.querySelector(".col-count");
        if (c) c.textContent = n;
      });
    }
  }

  /* ── Terminal streaming ────────────────────────────────────── */
  window.startTerminal = function (terminalEl, script) {
    if (!terminalEl) return;
    const lines = script || TERMINAL_SCRIPT;
    let i = 0;
    function next() {
      if (i >= lines.length) {
        const done = document.createElement("span");
        done.className = "term-line term-ok";
        done.textContent = "✔ run completed — awaiting review";
        terminalEl.appendChild(done);
        terminalEl.scrollTop = terminalEl.scrollHeight;
        return;
      }
      const ln = lines[i++];
      const el = document.createElement("span");
      el.className = "term-line " + (ln.c || "");
      el.innerHTML = ln.t;
      terminalEl.appendChild(el);
      terminalEl.scrollTop = terminalEl.scrollHeight;
      setTimeout(next, ln.d != null ? ln.d : 380);
    }
    next();
  };

  const TERMINAL_SCRIPT = [
    { t: '<span class="term-prompt">forge@agent-ops</span>:<span class="term-muted">~/tasks/refactor-auth</span>$ plan --task TS-214', c: "" },
    { t: "→ reading repository (412 files indexed)", c: "term-muted" },
    { t: "→ locating auth middleware…  src/middleware/auth.go", c: "term-muted" },
    { t: "✓ identified 3 JWT validation paths", c: "term-ok" },
    { t: '<span class="term-prompt">forge</span> editing src/middleware/auth.go', c: "" },
    { t: "  - validateToken(): split into verifySignature + parseClaims", c: "term-muted" },
    { t: "  - extract refresh-token rotation into helper", c: "term-muted" },
    { t: "  - add context-aware cancellation", c: "term-muted" },
    { t: "✓ patch written (+148 / −72)", c: "term-ok" },
    { t: '<span class="term-prompt">forge</span> running go test ./internal/auth/…', c: "" },
    { t: "ok   internal/auth   0.42s", c: "term-ok" },
    { t: "ok   internal/auth/jwt   0.11s", c: "term-ok" },
    { t: "ok   internal/auth/middleware   0.30s", c: "term-ok" },
    { t: "PASS 38/38", c: "term-ok" },
    { t: '<span class="term-warn">⚠ 2 tests still touch deprecated Parse() — flag for follow-up</span>', c: "term-warn" },
    { t: '<span class="term-prompt">forge</span> drafting summary…', c: "" },
  ];

  /* ── Init ──────────────────────────────────────────────────── */
  document.addEventListener("DOMContentLoaded", () => {
    initChrome();
    initTabs();
    initSegments();
    initSwitches();
    initModals();
    initKanban();
  });
})();
