package main

import (
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/getlantern/systray"
	"github.com/kaicmurilo/tokalytics/pkg/autostart"
	"github.com/kaicmurilo/tokalytics/pkg/instancectl"
	"github.com/kaicmurilo/tokalytics/pkg/polling"
	"github.com/kaicmurilo/tokalytics/pkg/providers"
	"github.com/kaicmurilo/tokalytics/pkg/runstate"
	"github.com/kaicmurilo/tokalytics/pkg/sysmon"
	"github.com/kaicmurilo/tokalytics/pkg/updatecheck"
	"github.com/kaicmurilo/tokalytics/pkg/utils"
)

//go:embed web/static/*
var staticFiles embed.FS

//go:embed assets/icon.png
var iconPNG []byte

// Version é definida em releases via ldflags (-X main.Version=...).
var Version = "dev"

// httpListenPort é a porta efetiva do dashboard (3456+ se a padrão estiver ocupada).
var httpListenPort atomic.Int32

func dashboardPort() int {
	p := int(httpListenPort.Load())
	if p > 0 {
		return p
	}
	return 3456
}

func listenFrom(basePort, maxAttempts int) (net.Listener, int, error) {
	for i := 0; i < maxAttempts; i++ {
		p := basePort + i
		ln, err := net.Listen("tcp", fmt.Sprintf(":%d", p))
		if err == nil {
			return ln, p, nil
		}
	}
	return nil, 0, fmt.Errorf("nenhuma porta TCP livre entre %d e %d", basePort, basePort+maxAttempts-1)
}

func main() {
	stopF := flag.Bool("stop", false, "Encerra a instância em execução (via API local)")
	reloadF := flag.Bool("reload", false, "Pede atualização de dados na instância em execução")
	restartF := flag.Bool("restart", false, "Encerra a instância em execução e inicia de novo em segundo plano (com -dev não encerra; equivale a outro -start em paralelo)")
	statusF := flag.Bool("status", false, "Mostra se há instância rodando, URL, versão da API e PID (runstate)")
	devF := flag.Bool("dev", false, "Desenvolvimento: ignora instância já em execução e sobe outra (porta seguinte se 3456 ocupada)")
	startF := flag.Bool("start", false, "Inicia em segundo plano (sem ocupar o terminal; sem ícone na barra de menu)")
	headlessF := flag.Bool("headless", false, "Uso interno: só HTTP + polling, sem menu bar")
	var showVersion bool
	flag.BoolVar(&showVersion, "version", false, "Mostra a versão deste binário e sai (-v e --v também)")
	flag.BoolVar(&showVersion, "v", false, "Mostra a versão deste binário e sai (atalho de -version)")
	var showHelp bool
	flag.BoolVar(&showHelp, "h", false, "Mostra esta ajuda e sai")
	flag.BoolVar(&showHelp, "help", false, "Mostra esta ajuda e sai")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Uso: tokalytics [opções]\n")
		fmt.Fprintf(os.Stderr, "     tokalytics help\n\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nSem flags: abre o menu bar e o dashboard se ainda não existir instância.\n")
		fmt.Fprintf(os.Stderr, "Use -start para subir em segundo plano e liberar o terminal (-start e --start são equivalentes).\n")
		fmt.Fprintf(os.Stderr, "Use -restart para encerrar e subir de novo; -dev (ou TOKALYTICS_DEV=1) para rodar em paralelo ao app instalado.\n")
	}
	flag.Parse()
	args := flag.Args()
	if len(args) > 0 {
		if len(args) == 1 && strings.EqualFold(args[0], "help") {
			flag.Usage()
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, "argumento inesperado: %q\n\n", args[0])
		flag.Usage()
		os.Exit(2)
	}

	if showHelp {
		flag.Usage()
		os.Exit(0)
	}
	if showVersion {
		fmt.Println(Version)
		return
	}
	if *statusF {
		cmdStatus()
		return
	}
	if *stopF {
		if err := cmdStop(); err != nil {
			log.Fatal(err)
		}
		return
	}
	if *reloadF {
		if err := cmdReload(); err != nil {
			log.Fatal(err)
		}
		return
	}

	skipSingleton := *devF || os.Getenv("TOKALYTICS_DEV") == "1" ||
		strings.EqualFold(os.Getenv("TOKALYTICS_DEV"), "true")

	if *restartF {
		if err := cmdRestart(skipSingleton); err != nil {
			log.Fatal(err)
		}
		return
	}

	if *startF {
		if err := cmdStartBackground(skipSingleton); err != nil {
			log.Fatal(err)
		}
		return
	}

	if *headlessF {
		if !skipSingleton {
			if port, ok := instancectl.FindRunning(); ok {
				fmt.Printf("Tokalytics já está rodando em http://localhost:%d\n", port)
				os.Exit(0)
			}
		}
		runHeadless()
		return
	}

	if !skipSingleton {
		if port, ok := instancectl.FindRunning(); ok {
			fmt.Printf("Tokalytics já está rodando em http://localhost:%d\n", port)
			fmt.Fprintf(os.Stderr, "Dica: npm run dev ou go run . -dev para desenvolvimento em paralelo.\n")
			os.Exit(0)
		}
	}

	systray.Run(onReady, onExit)
}

func resolveInstancePort() int {
	rs, err := runstate.Read()
	if err == nil && rs.Port > 0 {
		if p, ok := instancectl.PortFromRunstate(rs.Port); ok {
			return p
		}
	}
	if p, ok := instancectl.FindRunning(); ok {
		return p
	}
	return 0
}

func cmdStop() error {
	port := resolveInstancePort()
	if port == 0 {
		return fmt.Errorf("nenhuma instância Tokalytics em execução foi encontrada")
	}
	if err := instancectl.Shutdown(port); err != nil {
		return err
	}
	fmt.Printf("Encerramento solicitado (http://localhost:%d).\n", port)
	return nil
}

func cmdReload() error {
	port := resolveInstancePort()
	if port == 0 {
		return fmt.Errorf("nenhuma instância Tokalytics em execução foi encontrada")
	}
	if err := instancectl.Reload(port); err != nil {
		return err
	}
	fmt.Println("Atualização de dados solicitada na instância em execução.")
	return nil
}

// waitUntilNoRunningInstance aguarda até não haver resposta Tokalytics na faixa de portas (após shutdown).
func waitUntilNoRunningInstance(maxWait time.Duration) {
	deadline := time.Now().Add(maxWait)
	for time.Now().Before(deadline) {
		if _, ok := instancectl.FindRunning(); !ok {
			return
		}
		time.Sleep(80 * time.Millisecond)
	}
}

func cmdRestart(devMode bool) error {
	if !devMode {
		port := resolveInstancePort()
		if port != 0 {
			if err := instancectl.Shutdown(port); err != nil {
				return fmt.Errorf("reinício: falha ao encerrar instância: %w", err)
			}
			waitUntilNoRunningInstance(20 * time.Second)
		}
	}
	return cmdStartBackground(devMode)
}

func cmdStartBackground(devMode bool) error {
	if !devMode {
		if port, ok := instancectl.FindRunning(); ok {
			fmt.Printf("Tokalytics já está rodando em http://localhost:%d\n", port)
			return nil
		}
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	args := []string{exe}
	if devMode {
		args = append(args, "-dev")
	}
	args = append(args, "-headless")
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdin = nil
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	setDetachChild(cmd)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("falha ao iniciar em segundo plano: %w", err)
	}
	childPID := cmd.Process.Pid
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		rs, err := runstate.Read()
		if err == nil && rs.PID == childPID && rs.Port > 0 {
			if _, ok := instancectl.PortFromRunstate(rs.Port); ok {
				fmt.Printf("Tokalytics iniciado em segundo plano: http://localhost:%d\n", rs.Port)
				fmt.Fprintf(os.Stderr, "Encerre com: tokalytics --stop\n")
				return nil
			}
		}
		time.Sleep(120 * time.Millisecond)
	}
	return fmt.Errorf("timeout: Tokalytics não respondeu a tempo (tente tokalytics -headless para ver o erro)")
}

func runHeadless() {
	registerProviders()
	polling.Headless = true
	go startHTTPServer()
	go polling.Start()

	sigCh := make(chan os.Signal, 1)
	var sigs []os.Signal
	sigs = append(sigs, os.Interrupt)
	if runtime.GOOS != "windows" {
		sigs = append(sigs, syscall.SIGTERM)
	}
	signal.Notify(sigCh, sigs...)
	<-sigCh
	log.Println("Encerrando Tokalytics...")
	runstate.Remove()
	os.Exit(0)
}

func cmdStatus() {
	fmt.Printf("CLI (binário): %s\n", Version)
	port, apiVer, ok := instancectl.RunningInfo()
	if ok {
		fmt.Println("Estado: rodando")
		fmt.Printf("URL: http://127.0.0.1:%d/\n", port)
		if apiVer != "" {
			fmt.Printf("Versão (API): %s\n", apiVer)
		}
		if rs, err := runstate.Read(); err == nil && rs.Port == port && rs.PID > 0 {
			fmt.Printf("PID: %d\n", rs.PID)
		}
		return
	}
	fmt.Printf("Estado: parado (nenhuma resposta nas portas %d–%d)\n", instancectl.PortMin, instancectl.PortMax)
	if rs, err := runstate.Read(); err == nil && (rs.PID > 0 || rs.Port > 0) {
		fmt.Printf("runstate (pode estar obsoleto): pid=%d port=%d\n", rs.PID, rs.Port)
	}
}

func onReady() {
	systray.SetIcon(iconPNG)
	systray.SetTitle("")
	systray.SetTooltip("Tokalytics: Claude, Cursor, Gemini & Codex Usage")

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
				openBrowser(fmt.Sprintf("http://localhost:%d", dashboardPort()))
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
	runstate.Remove()
}

func registerProviders() {
	providers.Registry = nil // reset

	cfg := utils.LoadSettings()

	// Claude: sempre registra — OAuth lê de ~/.claude/.credentials.json (Linux) ou Keychain (macOS).
	// Cookie web é um fallback adicional (usado quando OAuth não está disponível).
	claudeCookie := cfg.ClaudeCookie
	if claudeCookie == "" {
		if val, err := utils.GetChromeCookie("claude.ai", "sessionKey"); err == nil && val != "" {
			claudeCookie = "sessionKey=" + val
		}
	}
	providers.Register(&providers.ClaudeProvider{CookieHeader: claudeCookie})
	if claudeCookie != "" {
		log.Println("Claude provider registrado (cookie + OAuth).")
	} else {
		log.Println("Claude provider registrado (OAuth local).")
	}

	// Gemini CLI: always register (reads local session files, no auth needed)
	providers.Register(&providers.GeminiProvider{})
	log.Println("Gemini provider registrado.")

	// Codex CLI: register when local config/session dir exists
	if _, err := os.Stat(filepath.Join(providersPathHome(), ".codex")); err == nil {
		providers.Register(&providers.CodexProvider{})
		log.Println("Codex provider registrado.")
	} else {
		log.Println("Codex: nenhuma instalação local detectada.")
	}

	// Cursor: settings file → Chrome cookies → Cursor local SQLite (Bearer JWT)
	cursorCookie := cfg.CursorCookie
	if cursorCookie == "" {
		if val, err := utils.GetChromeCookie("cursor.com", "WorkosCursorSessionToken"); err == nil && val != "" {
			cursorCookie = "WorkosCursorSessionToken=" + val
		}
	}
	if cursorCookie != "" {
		log.Println("Cursor provider registrado (cookie).")
		providers.Register(&providers.CursorProvider{CookieHeader: cursorCookie})
	} else if val, err := utils.GetCursorAuthToken(); err == nil && val != "" {
		log.Println("Cursor provider registrado (JWT local).")
		providers.Register(&providers.CursorLocalProvider{BearerToken: val})
	} else {
		log.Println("Cursor: nenhum cookie ou token local encontrado.")
	}
}

func providersPathHome() string {
	home, _ := os.UserHomeDir()
	return home
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

func readCodexSkills(home string) []string {
	root := filepath.Join(home, ".codex", "skills")
	entries, err := os.ReadDir(root)
	if err != nil {
		return []string{}
	}
	var names []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if strings.HasPrefix(entry.Name(), ".") {
			subEntries, err := os.ReadDir(filepath.Join(root, entry.Name()))
			if err != nil {
				continue
			}
			for _, sub := range subEntries {
				if sub.IsDir() {
					names = append(names, sub.Name())
				}
			}
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	return names
}

func readCodexPlugins(home string) []string {
	path := filepath.Join(home, ".codex", "config.toml")
	data, err := os.ReadFile(path)
	if err != nil {
		return []string{}
	}
	re := regexp.MustCompile(`(?m)^\[plugins\."([^"]+)"\]`)
	matches := re.FindAllStringSubmatch(string(data), -1)
	names := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) == 2 {
			names = append(names, match[1])
		}
	}
	sort.Strings(names)
	return names
}

func readCodexMCPs(home string) []string {
	path := filepath.Join(home, ".codex", "config.toml")
	data, err := os.ReadFile(path)
	if err != nil {
		return []string{}
	}
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?m)^\[mcp_servers\.([^\]."]+)\]`),
		regexp.MustCompile(`(?m)^\[mcp_servers\."([^"]+)"\]`),
	}
	seen := map[string]struct{}{}
	for _, re := range patterns {
		for _, match := range re.FindAllStringSubmatch(string(data), -1) {
			if len(match) == 2 {
				seen[match[1]] = struct{}{}
			}
		}
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
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

	http.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"service": instancectl.ServiceName,
			"version": Version,
		})
	})

	http.HandleFunc("/api/shutdown", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !instancectl.LoopbackRequest(r) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		go func() {
			time.Sleep(80 * time.Millisecond)
			if polling.Headless {
				runstate.Remove()
				os.Exit(0)
			}
			systray.Quit()
		}()
	})

	http.HandleFunc("/api/refresh", func(w http.ResponseWriter, r *http.Request) {
		log.Println("Refresh manual solicitado via API")
		polling.TriggerUpdate()
		w.WriteHeader(http.StatusOK)
	})

	http.HandleFunc("/api/update-check", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		res := updatecheck.Check(Version)
		_ = json.NewEncoder(w).Encode(res)
	})

	http.HandleFunc("/api/settings", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			cfg := utils.LoadSettings()
			masked := map[string]interface{}{
				"claudeConfigured":   cfg.ClaudeCookie != "",
				"cursorConfigured":   cfg.CursorCookie != "",
				"launchAtLogin":      cfg.LaunchAtLogin,
				"autostartSupported": autostart.Supported(),
				"httpPort":           dashboardPort(),
			}
			json.NewEncoder(w).Encode(masked)

		case http.MethodPost:
			var body struct {
				ClaudeCookie  string `json:"claudeCookie"`
				CursorCookie  string `json:"cursorCookie"`
				LaunchAtLogin *bool  `json:"launchAtLogin"`
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
			if body.LaunchAtLogin != nil {
				if err := autostart.SetEnabled(*body.LaunchAtLogin); err != nil {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusInternalServerError)
					json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
					return
				}
				cfg.LaunchAtLogin = *body.LaunchAtLogin
			}
			if err := utils.SaveSettings(cfg); err != nil {
				http.Error(w, `{"error":"failed to save"}`, 500)
				return
			}
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
				_ = autostart.SetEnabled(false)
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
		codexSessions := providers.ParseCodexSessions()
		allSessions := append(claudeSessions, cursorSessions...)
		allSessions = append(allSessions, codexSessions...)

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
			"warnings":         []map[string]interface{}{},
		}
		json.NewEncoder(w).Encode(data)
	})

	http.HandleFunc("/api/system-live", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(sysmon.Collect())
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
		if _, err := os.Stat(filepath.Join(home, ".codex")); err == nil {
			result["codex"] = map[string]interface{}{
				"plugins": readCodexPlugins(home),
				"mcps":    readCodexMCPs(home),
				"skills":  readCodexSkills(home),
			}
		}
		json.NewEncoder(w).Encode(result)
	})

	ln, port, err := listenFrom(3456, 100)
	if err != nil {
		log.Fatal(err)
	}
	httpListenPort.Store(int32(port))
	if err := runstate.Write(os.Getpid(), port); err != nil {
		log.Printf("runstate: %v", err)
	}
	log.Printf("Dashboard rodando em http://localhost:%d\n", port)
	if err := http.Serve(ln, nil); err != nil {
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
