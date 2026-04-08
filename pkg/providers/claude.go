package providers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type ClaudeProvider struct {
	CookieHeader string // fallback: web cookie
}

// lastClaudeUsage caches the last successful OAuth result
var lastClaudeUsage *Usage

func (p *ClaudeProvider) ID() string   { return "claude" }
func (p *ClaudeProvider) Name() string { return "Claude" }

func (p *ClaudeProvider) FetchUsage() (*Usage, error) {
	// 1. Try OAuth first (Claude Code-credentials from Keychain)
	if u, err := fetchClaudeOAuth(); err == nil {
		lastClaudeUsage = u
		saveClaudeCache(u)
		return u, nil
	}

	// 2. Fallback: web cookie approach
	if p.CookieHeader != "" {
		if u, err := fetchClaudeWeb(p.CookieHeader); err == nil {
			lastClaudeUsage = u
			saveClaudeCache(u)
			return u, nil
		}
	}

	// 3. In-memory cache
	if lastClaudeUsage != nil {
		cached := *lastClaudeUsage
		cached.UpdatedAt = "cache"
		return &cached, nil
	}

	// 4. Disk cache (survives restarts)
	if u := loadClaudeCache(); u != nil {
		lastClaudeUsage = u
		u.UpdatedAt = "cache"
		return u, nil
	}

	return nil, fmt.Errorf("claude: no authentication available")
}

func claudeCachePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "tokalytics", "claude_cache.json")
}

func saveClaudeCache(u *Usage) {
	data, err := json.Marshal(u)
	if err != nil {
		return
	}
	os.MkdirAll(filepath.Dir(claudeCachePath()), 0700)
	os.WriteFile(claudeCachePath(), data, 0600)
}

func loadClaudeCache() *Usage {
	data, err := os.ReadFile(claudeCachePath())
	if err != nil {
		return nil
	}
	var u Usage
	if err := json.Unmarshal(data, &u); err != nil {
		return nil
	}
	return &u
}

// fetchClaudeOAuth reads the Claude CLI OAuth token from Keychain and calls the OAuth API.
// This is the same approach used by CodexBar's ClaudeOAuthUsageFetcher.
func fetchClaudeOAuth() (*Usage, error) {
	// Read "Claude Code-credentials" from Keychain
	out, err := exec.Command("security", "find-generic-password", "-s", "Claude Code-credentials", "-w").Output()
	if err != nil || len(out) == 0 {
		return nil, fmt.Errorf("keychain: Claude Code-credentials not found")
	}

	var creds struct {
		ClaudeAiOauth struct {
			AccessToken string `json:"accessToken"`
		} `json:"claudeAiOauth"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(out))), &creds); err != nil {
		return nil, fmt.Errorf("parse credentials: %v", err)
	}
	token := creds.ClaudeAiOauth.AccessToken
	if token == "" {
		return nil, fmt.Errorf("empty access token")
	}

	client := &http.Client{Timeout: 10 * time.Second}
	req, _ := http.NewRequest("GET", "https://api.anthropic.com/api/oauth/usage", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("anthropic-beta", "oauth-2025-04-20")
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("oauth api: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("oauth api status %d", resp.StatusCode)
	}

	var raw struct {
		FiveHour  *usageWindow `json:"five_hour"`
		SevenDay  *usageWindow `json:"seven_day"`
		SevenDayOpus    *usageWindow `json:"seven_day_opus"`
		SevenDaySonnet  *usageWindow `json:"seven_day_sonnet"`
		ExtraUsage *struct {
			IsEnabled    bool    `json:"is_enabled"`
			MonthlyLimit float64 `json:"monthly_limit"` // cents
			UsedCredits  float64 `json:"used_credits"`  // cents
			Utilization  float64 `json:"utilization"`
			Currency     string  `json:"currency"`
		} `json:"extra_usage"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode: %v", err)
	}

	u := &Usage{
		ProviderID: "claude",
		Name:       "Claude",
		Plan:       "Pro",
		UpdatedAt:  "agora",
		TotalLimit: 100,
	}

	// Session (5h)
	if raw.FiveHour != nil {
		w := toRateWindow("Session", raw.FiveHour)
		u.Windows = append(u.Windows, w)
		u.Used = w.PctUsed
		u.Remaining = w.PctLeft
		u.SessionReset = w.ResetsAt
	}

	// Weekly (7-day)
	if raw.SevenDay != nil {
		w := toRateWindow("Weekly", raw.SevenDay)
		u.Windows = append(u.Windows, w)
		u.WeeklyReset = w.ResetsAt
	}

	// Opus-specific weekly
	if raw.SevenDayOpus != nil {
		u.Windows = append(u.Windows, toRateWindow("Weekly (Opus)", raw.SevenDayOpus))
	}

	// Extra usage / spending limit
	if raw.ExtraUsage != nil && raw.ExtraUsage.IsEnabled && raw.ExtraUsage.MonthlyLimit > 0 {
		currency := raw.ExtraUsage.Currency
		if currency == "" {
			currency = "USD"
		}
		sym := "$"
		limitUSD := raw.ExtraUsage.MonthlyLimit / 100
		usedUSD := raw.ExtraUsage.UsedCredits / 100
		u.Windows = append(u.Windows, RateWindow{
			Name:    fmt.Sprintf("Extra usage (%s%.2f / %s%.2f)", sym, usedUSD, sym, limitUSD),
			PctUsed: raw.ExtraUsage.Utilization,
			PctLeft: 100 - raw.ExtraUsage.Utilization,
		})
	}

	return u, nil
}

type usageWindow struct {
	Utilization float64 `json:"utilization"`
	ResetsAt    string  `json:"resets_at"`
}

func toRateWindow(name string, w *usageWindow) RateWindow {
	util := w.Utilization
	// normalize 0-1 → 0-100 if needed
	if util > 0 && util <= 1.0 {
		util *= 100
	}
	pctLeft := 100 - util
	deficit := 0.0
	if pctLeft < 0 {
		deficit = -pctLeft
		pctLeft = 0
	}
	rw := RateWindow{
		Name:    name,
		PctUsed: util,
		PctLeft: pctLeft,
		Deficit: deficit,
	}
	if w.ResetsAt != "" {
		if t, err := time.Parse(time.RFC3339, w.ResetsAt); err == nil {
			rw.ResetsAt = t
		}
	}
	return rw
}

// fetchClaudeWeb is the fallback using browser session cookie.
func fetchClaudeWeb(cookieHeader string) (*Usage, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	setH := func(r *http.Request) {
		r.Header.Set("Cookie", cookieHeader)
		r.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")
		r.Header.Set("Accept", "application/json")
	}

	req, _ := http.NewRequest("GET", "https://claude.ai/api/organizations", nil)
	setH(req)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("org: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("org status %d", resp.StatusCode)
	}

	var orgs []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&orgs); err != nil || len(orgs) == 0 {
		return nil, fmt.Errorf("no orgs")
	}
	orgID, _ := orgs[0]["uuid"].(string)

	req2, _ := http.NewRequest("GET", fmt.Sprintf("https://claude.ai/api/organizations/%s/usage", orgID), nil)
	setH(req2)
	resp2, err := client.Do(req2)
	if err != nil {
		return nil, fmt.Errorf("usage: %v", err)
	}
	defer resp2.Body.Close()

	var raw map[string]interface{}
	json.NewDecoder(resp2.Body).Decode(&raw)

	u := &Usage{ProviderID: "claude", Name: "Claude", TotalLimit: 100, UpdatedAt: "agora"}
	if w := parseWebWindow(raw, "five_hour", "Session"); w != nil {
		u.Windows = append(u.Windows, *w)
		u.Used = w.PctUsed; u.Remaining = w.PctLeft; u.SessionReset = w.ResetsAt
	}
	if w := parseWebWindow(raw, "seven_day", "Weekly"); w != nil {
		u.Windows = append(u.Windows, *w)
		u.WeeklyReset = w.ResetsAt
	}
	return u, nil
}

func parseWebWindow(raw map[string]interface{}, key, label string) *RateWindow {
	data, ok := raw[key].(map[string]interface{})
	if !ok {
		return nil
	}
	util := 0.0
	if v, ok := data["utilization"].(float64); ok {
		util = v
		if util <= 1.0 { util *= 100 }
	}
	pctLeft := 100 - util
	w := &RateWindow{Name: label, PctUsed: util, PctLeft: pctLeft}
	if s, ok := data["resets_at"].(string); ok {
		if t, err := time.Parse(time.RFC3339, s); err == nil { w.ResetsAt = t }
	}
	return w
}
