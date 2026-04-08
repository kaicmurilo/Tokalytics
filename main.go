package main

import (
	"embed"
	"encoding/json"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"

	"github.com/getlantern/systray"
	"github.com/kaicmurilo/tokalytics/pkg/polling"
	"github.com/kaicmurilo/tokalytics/pkg/providers"
	"github.com/kaicmurilo/tokalytics/pkg/utils"
)

//go:embed web/static/*
var staticFiles embed.FS

// Version é definida em releases via ldflags (-X main.Version=...).
var Version = "dev"

func main() {
	systray.Run(onReady, onExit)
}

func onReady() {
	systray.SetTitle("⟳")
	systray.SetTooltip("Tokalytics: Claude, Cursor & Gemini Usage")

	registerProviders()

	// Pre-create 25 display slots (disabled, used as data rows in the popup)
	const numSlots = 25
	slots := make([]*systray.MenuItem, numSlots)
	for i := 0; i < numSlots; i++ {
		item := systray.AddMenuItem("  ", "")
		item.Disable()
		slots[i] = item
	}
	polling.SetMenuSlots(slots)

	systray.AddSeparator()
	mOpen := systray.AddMenuItem("📊  Abrir Dashboard", "Abrir dashboard web")
	mRefresh := systray.AddMenuItem("🔄  Atualizar", "Atualizar quotas")
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("✕  Sair", "Sair do aplicativo")

	go startHTTPServer()
	go polling.Start()

	go func() {
		for {
			select {
			case <-mOpen.ClickedCh:
				openBrowser("http://localhost:3456")
			case <-mRefresh.ClickedCh:
				log.Println("Atualizando dados manualmente...")
				polling.TriggerUpdate()
			case <-mQuit.ClickedCh:
				systray.Quit()
			}
		}
	}()
}

func onExit() {
	log.Println("Saindo...")
}

func registerProviders() {
	providers.Registry = nil // reset

	cfg := utils.LoadSettings()

	// Claude cookie: settings file first, then Chrome
	claudeCookie := cfg.ClaudeCookie
	if claudeCookie == "" {
		if val, err := utils.GetChromeCookie("claude.ai", "sessionKey"); err == nil && val != "" {
			claudeCookie = "sessionKey=" + val
		}
	}
	if claudeCookie != "" {
		log.Println("Claude provider registrado.")
		providers.Register(&providers.ClaudeProvider{CookieHeader: claudeCookie})
	} else {
		log.Println("Claude: nenhum cookie configurado. Configure em Settings no dashboard.")
	}

	// Gemini CLI: always register (reads local session files, no auth needed)
	providers.Register(&providers.GeminiProvider{})
	log.Println("Gemini provider registrado.")

	// Cursor cookie: settings file first, then Chrome
	cursorCookie := cfg.CursorCookie
	if cursorCookie == "" {
		if val, err := utils.GetChromeCookie("cursor.com", "WorkosCursorSessionToken"); err == nil && val != "" {
			cursorCookie = "WorkosCursorSessionToken=" + val
		}
	}
	if cursorCookie != "" {
		log.Println("Cursor provider registrado.")
		providers.Register(&providers.CursorProvider{CookieHeader: cursorCookie})
	} else {
		log.Println("Cursor: nenhum cookie configurado.")
	}
}

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

func readClaudeMCPs(home string) []string {
	path := filepath.Join(home, ".claude.json")
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

func readClaudeSkills(home string) []string {
	path := filepath.Join(home, ".claude", "plugins", "installed_plugins.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return []string{}
	}
	var cfg struct {
		Plugins map[string][]struct {
			InstallPath string `json:"installPath"`
		} `json:"plugins"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return []string{}
	}
	seen := map[string]struct{}{}
	for _, installs := range cfg.Plugins {
		for _, inst := range installs {
			skillsDir := filepath.Join(inst.InstallPath, "skills")
			entries, err := os.ReadDir(skillsDir)
			if err != nil {
				continue
			}
			for _, e := range entries {
				if e.IsDir() {
					seen[e.Name()] = struct{}{}
				}
			}
		}
	}
	names := make([]string, 0, len(seen))
	for k := range seen {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

func readCursorSkills(home string) []string {
	dir := filepath.Join(home, ".cursor", "skills")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return []string{}
	}
	names := []string{}
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names
}

func readGeminiSkills(home string) []string {
	dir := filepath.Join(home, ".gemini", "skills")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return []string{}
	}
	names := []string{}
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
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

func startHTTPServer() {
	subFS, err := fs.Sub(staticFiles, "web/static")
	if err != nil {
		log.Fatal("Falha ao carregar estáticos: ", err)
	}

	http.Handle("/", http.FileServer(http.FS(subFS)))

	http.HandleFunc("/api/refresh", func(w http.ResponseWriter, r *http.Request) {
		log.Println("Refresh manual solicitado via API")
		polling.TriggerUpdate()
		w.WriteHeader(http.StatusOK)
	})

	http.HandleFunc("/api/settings", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			cfg := utils.LoadSettings()
			// Mask cookie values for display
			masked := map[string]interface{}{
				"claudeConfigured": cfg.ClaudeCookie != "",
				"cursorConfigured": cfg.CursorCookie != "",
			}
			json.NewEncoder(w).Encode(masked)

		case http.MethodPost:
			var body struct {
				ClaudeCookie string `json:"claudeCookie"`
				CursorCookie string `json:"cursorCookie"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, `{"error":"invalid json"}`, 400)
				return
			}
			cfg := utils.LoadSettings()
			if body.ClaudeCookie != "" {
				cfg.ClaudeCookie = body.ClaudeCookie
			}
			if body.CursorCookie != "" {
				cfg.CursorCookie = body.CursorCookie
			}
			if err := utils.SaveSettings(cfg); err != nil {
				http.Error(w, `{"error":"failed to save"}`, 500)
				return
			}
			// Re-register providers with new cookies
			registerProviders()
			polling.TriggerUpdate()
			json.NewEncoder(w).Encode(map[string]bool{"ok": true})

		case http.MethodDelete:
			var body struct {
				Provider string `json:"provider"`
			}
			json.NewDecoder(r.Body).Decode(&body)
			cfg := utils.LoadSettings()
			if body.Provider == "claude" {
				cfg.ClaudeCookie = ""
			} else if body.Provider == "cursor" {
				cfg.CursorCookie = ""
			} else {
				cfg = utils.Settings{}
			}
			utils.SaveSettings(cfg)
			registerProviders()
			json.NewEncoder(w).Encode(map[string]bool{"ok": true})
		}
	})

	http.HandleFunc("/api/data", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		log.Println("Endpoint /api/data chamado")
		providerUsages := providers.GetUsages()

		// Parse local logs
		claudeSessions := providers.ParseClaudeSessions()
		cursorSessions := providers.ParseCursorSessions()
		allSessions := append(claudeSessions, cursorSessions...)

		// Full aggregation: daily, model, projects, top prompts, insights
		agg := providers.Aggregate(allSessions)
		todayStats := providers.GetTodayStats()

		// Enrich Claude provider with local cost data
		if cp, ok := providerUsages["claude"]; ok {
			cp.TodayCostUSD = todayStats.TotalCost
			cp.TodayTokens = todayStats.TotalTokens
			// 30-day cost from sessions
			var cost30 float64
			var tok30 int
			for _, s := range claudeSessions {
				cost30 += s.Cost
				tok30 += s.TotalTokens
			}
			cp.Last30CostUSD = cost30
			cp.Last30Tokens = tok30
		}

		data := map[string]interface{}{
			"providers":        providerUsages,
			"localSummary":     todayStats,
			"sessions":         agg.Sessions,
			"dailyUsage":       agg.DailyUsage,
			"modelBreakdown":   agg.ModelBreakdown,
			"projectBreakdown": agg.ProjectBreakdown,
			"topPrompts":       agg.TopPrompts,
			"totals":           agg.Totals,
			"insights":         agg.Insights,
			"warnings": []map[string]interface{}{},
		}
		json.NewEncoder(w).Encode(data)
	})

	http.HandleFunc("/api/plugins", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		home, err := os.UserHomeDir()
		if err != nil {
			http.Error(w, `{"error":"could not find home dir"}`, 500)
			return
		}
		result := map[string]interface{}{
			"claude": map[string]interface{}{
				"plugins": readClaudePlugins(home),
				"mcps":    readClaudeMCPs(home),
				"skills":  readClaudeSkills(home),
			},
			"cursor": map[string]interface{}{
				"plugins": []string{},
				"mcps":    readCursorMCPs(home),
				"skills":  readCursorSkills(home),
			},
			"gemini": map[string]interface{}{
				"plugins": readGeminiExtensions(home),
				"mcps":    []string{},
				"skills":  readGeminiSkills(home),
			},
		}
		json.NewEncoder(w).Encode(result)
	})

	log.Println("Dashboard rodando em http://localhost:3456")
	if err := http.ListenAndServe(":3456", nil); err != nil {
		log.Fatal(err)
	}
}

func openBrowser(url string) {
	var err error
	switch runtime.GOOS {
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		err = exec.Command("open", url).Start()
	default:
		err = exec.Command("open", url).Start()
	}
	if err != nil {
		log.Printf("Falha ao abrir navegador: %v", err)
	}
}
