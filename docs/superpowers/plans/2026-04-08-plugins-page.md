# Plugins Page Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a "Plugins" tab to the dashboard listing configured plugins/extensions for Claude, Cursor, and Gemini.

**Architecture:** New `/api/plugins` endpoint in `main.go` reads 3 config files from the user's home directory and returns a structured JSON response. The frontend renders a new tab page with one card per tool.

**Tech Stack:** Go (backend endpoint), HTML/JS (frontend tab + rendering), no new dependencies.

---

### Task 1: Backend — `/api/plugins` endpoint

**Files:**
- Modify: `main.go` (add handler inside `startHTTPServer`, before the last `log.Println`)

- [ ] **Step 1: Add the `/api/plugins` handler in `main.go`**

Insert before `log.Println("Dashboard rodando em http://localhost:3456")`:

```go
http.HandleFunc("/api/plugins", func(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")

    home, err := os.UserHomeDir()
    if err != nil {
        http.Error(w, `{"error":"could not find home dir"}`, 500)
        return
    }

    result := map[string]interface{}{
        "claude": readClaudePlugins(home),
        "cursor": readCursorMCPs(home),
        "gemini": readGeminiExtensions(home),
    }
    json.NewEncoder(w).Encode(result)
})
```

- [ ] **Step 2: Add the three helper functions in `main.go`** (before `startHTTPServer`)

```go
func readClaudePlugins(home string) []string {
    path := filepath.Join(home, ".claude", "settings.json")
    data, err := os.ReadFile(path)
    if err != nil {
        return []string{}
    }
    var cfg struct {
        EnabledPlugins map[string]interface{} `json:"enabledPlugins"`
    }
    if err := json.Unmarshal(data, &cfg); err != nil {
        return []string{}
    }
    names := make([]string, 0, len(cfg.EnabledPlugins))
    for k := range cfg.EnabledPlugins {
        names = append(names, k)
    }
    sort.Strings(names)
    return names
}

func readCursorMCPs(home string) []string {
    path := filepath.Join(home, ".cursor", "mcp.json")
    data, err := os.ReadFile(path)
    if err != nil {
        return []string{}
    }
    var cfg struct {
        McpServers map[string]interface{} `json:"mcpServers"`
    }
    if err := json.Unmarshal(data, &cfg); err != nil {
        return []string{}
    }
    names := make([]string, 0, len(cfg.McpServers))
    for k := range cfg.McpServers {
        names = append(names, k)
    }
    sort.Strings(names)
    return names
}

func readGeminiExtensions(home string) []string {
    path := filepath.Join(home, ".gemini", "extensions", "extension-enablement.json")
    data, err := os.ReadFile(path)
    if err != nil {
        return []string{}
    }
    var cfg map[string]interface{}
    if err := json.Unmarshal(data, &cfg); err != nil {
        return []string{}
    }
    names := make([]string, 0, len(cfg))
    for k := range cfg {
        names = append(names, k)
    }
    sort.Strings(names)
    return names
}
```

- [ ] **Step 3: Ensure imports include `path/filepath` and `sort` in `main.go`**

Check existing imports at top of `main.go` and add `"path/filepath"` and `"sort"` if missing.

- [ ] **Step 4: Build and verify**

```bash
cd /Users/kaicmurilo/Documents/DEV/estudos/claude-spend
go build ./...
```

Expected: no errors.

- [ ] **Step 5: Test endpoint manually**

```bash
curl http://localhost:3456/api/plugins
```

Expected JSON similar to:
```json
{
  "claude": ["claude-mem@thedotmack", "gemini@robbyt-claude-skills", "lumen@ory", "superpowers@superpowers-marketplace", "ui-ux-pro-max@ui-ux-pro-max-skill"],
  "cursor": ["Playwright", "claude-mem", "context7"],
  "gemini": ["superpowers"]
}
```

- [ ] **Step 6: Commit**

```bash
git add main.go
git commit -m "feat: add /api/plugins endpoint reading Claude/Cursor/Gemini config files"
```

---

### Task 2: Frontend — Plugins tab

**Files:**
- Modify: `web/static/index.html`

- [ ] **Step 1: Add nav button** after the Sessões button (line ~817):

```html
<button type="button" class="app-nav-link" data-page="plugins" id="nav-plugins">Plugins</button>
```

- [ ] **Step 2: Add page div** after the `page-sessions` closing tag and before the drilldown div:

```html
<!-- Página: plugins -->
<div id="page-plugins" class="app-page">
  <div class="sessions-section animate delay-1">
    <div class="section-header">
      <div class="section-icon" style="background:linear-gradient(135deg,#EDE9FE,#DDD6FE)">
        <svg viewBox="0 0 24 24" fill="none" stroke="#7C3AED" stroke-width="2.5" stroke-linecap="round"><path d="M9 3H5a2 2 0 0 0-2 2v4m6-6h10a2 2 0 0 1 2 2v4M9 3v18m0 0h10a2 2 0 0 0 2-2v-4M9 21H5a2 2 0 0 1-2-2v-4m0 0h18"/></svg>
      </div>
      <div class="section-title">Plugins Configurados</div>
    </div>
    <div id="pluginsContent" style="display:flex;gap:16px;flex-wrap:wrap;margin-top:4px;"></div>
  </div>
</div>
```

- [ ] **Step 3: Add CSS for plugin badges** in the `<style>` block:

```css
.plugin-tool-card { background:var(--white); border:1px solid var(--border); border-radius:12px; padding:20px; min-width:220px; flex:1; }
.plugin-tool-title { font-size:13px; font-weight:600; color:var(--text-secondary); text-transform:uppercase; letter-spacing:0.05em; margin-bottom:12px; }
.plugin-badge { display:inline-block; background:#F5F3FF; border:1px solid #DDD6FE; color:#6D28D9; border-radius:6px; padding:4px 10px; font-size:12px; font-weight:500; margin:3px; }
.plugin-empty { color:var(--text-secondary); font-size:13px; font-style:italic; }
```

- [ ] **Step 4: Add JS function `loadPlugins()`** and call it when the plugins tab is activated.

Find the existing tab-switching JS (look for `data-page` handler or `showPage` function) and add:

```js
async function loadPlugins() {
  const el = document.getElementById('pluginsContent');
  if (!el) return;
  try {
    const res = await fetch('/api/plugins');
    const data = await res.json();
    const tools = [
      { key: 'claude', label: 'Claude Code' },
      { key: 'cursor', label: 'Cursor' },
      { key: 'gemini', label: 'Gemini CLI' },
    ];
    el.innerHTML = tools.map(t => {
      const plugins = data[t.key] || [];
      const badges = plugins.length
        ? plugins.map(p => `<span class="plugin-badge">${p}</span>`).join('')
        : '<span class="plugin-empty">Nenhum plugin detectado</span>';
      return `<div class="plugin-tool-card"><div class="plugin-tool-title">${t.label}</div>${badges}</div>`;
    }).join('');
  } catch(e) {
    document.getElementById('pluginsContent').innerHTML = '<span class="plugin-empty">Erro ao carregar plugins</span>';
  }
}
```

Find where the tab click handler calls functions (e.g. `if (page === 'charts') renderCharts()`) and add:
```js
if (page === 'plugins') loadPlugins();
```

- [ ] **Step 5: Commit**

```bash
git add web/static/index.html
git commit -m "feat: add Plugins tab to dashboard"
```
