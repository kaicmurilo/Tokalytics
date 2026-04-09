package providers

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type CodexProvider struct{}

func (p *CodexProvider) ID() string   { return "codex" }
func (p *CodexProvider) Name() string { return "Codex" }

func (p *CodexProvider) FetchUsage() (*Usage, error) {
	sessions := ParseCodexSessions()
	meta := latestCodexRateMeta()

	if len(sessions) == 0 && !meta.valid {
		return nil, fmt.Errorf("codex: no local sessions found")
	}

	now := time.Now()
	today := now.Format("2006-01-02")
	cutoff := now.AddDate(0, 0, -30)

	var todayTokens, last30Tokens, todaySessions, totalSessions int
	for _, s := range sessions {
		totalSessions++
		sessionTime := parseSessionTime(s)
		if sessionTime.IsZero() {
			continue
		}
		if sessionTime.Local().Format("2006-01-02") == today {
			todaySessions++
			todayTokens += s.TotalTokens
		}
		if sessionTime.After(cutoff) {
			last30Tokens += s.TotalTokens
		}
	}

	plan := "CLI"
	if meta.plan != "" {
		plan = strings.ToUpper(meta.plan[:1]) + meta.plan[1:]
	}

	u := &Usage{
		ProviderID:   "codex",
		Name:         "Codex",
		Plan:         plan,
		UpdatedAt:    "agora",
		TotalLimit:   100,
		TodayTokens:  todayTokens,
		Last30Tokens: last30Tokens,
	}

	if meta.valid {
		name := "Primary"
		switch meta.windowMinutes {
		case 60:
			name = "Hourly"
		case 24 * 60:
			name = "Daily"
		case 7 * 24 * 60:
			name = "Weekly"
		}
		u.Used = meta.usedPct
		u.Remaining = 100 - meta.usedPct
		u.SessionReset = meta.resetsAt
		u.WeeklyReset = meta.resetsAt
		u.Windows = append(u.Windows, RateWindow{
			Name:     name,
			PctUsed:  meta.usedPct,
			PctLeft:  maxPctLeft(meta.usedPct),
			ResetsAt: meta.resetsAt,
		})
	}

	if todayTokens > 0 || last30Tokens > 0 {
		u.Windows = append(u.Windows, RateWindow{
			Name:    fmt.Sprintf("Hoje  %d sess · %s tok", todaySessions, fmtTokens(todayTokens)),
			PctUsed: 0,
			PctLeft: 100,
		})
		u.Windows = append(u.Windows, RateWindow{
			Name:    fmt.Sprintf("30d   %d sess · %s tok", totalSessions, fmtTokens(last30Tokens)),
			PctUsed: 0,
			PctLeft: 100,
		})
	}

	return u, nil
}

type codexRateMeta struct {
	valid         bool
	usedPct       float64
	windowMinutes int
	resetsAt      time.Time
	plan          string
	timestamp     time.Time
}

func getCodexDir() string {
	if p := strings.TrimSpace(os.Getenv("TOKALYTICS_CODEX_HOME")); p != "" {
		return p
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".codex")
}

func getCodexSessionsDir() string {
	return filepath.Join(getCodexDir(), "sessions")
}

func ParseCodexSessions() []Session {
	base := getCodexSessionsDir()
	if _, err := os.Stat(base); err != nil {
		return nil
	}

	var sessions []Session
	_ = filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() || filepath.Ext(path) != ".jsonl" {
			return nil
		}
		if s := parseCodexSessionFile(path); s != nil {
			sessions = append(sessions, *s)
		}
		return nil
	})

	return sessions
}

func parseCodexSessionFile(path string) *Session {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()

	var (
		sessionID      string
		project        string
		sessionTS      string
		firstPrompt    string
		pendingPrompts []struct {
			text string
			ts   string
		}
		queries []Query
	)

	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024*10)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var entry map[string]interface{}
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}

		entryType, _ := entry["type"].(string)
		entryTS, _ := entry["timestamp"].(string)

		switch entryType {
		case "session_meta":
			payload, _ := entry["payload"].(map[string]interface{})
			if payload == nil {
				continue
			}
			if id, _ := payload["id"].(string); id != "" {
				sessionID = id
			}
			if ts, _ := payload["timestamp"].(string); ts != "" {
				sessionTS = ts
			}
			if cwd, _ := payload["cwd"].(string); cwd != "" {
				project = cwd
			}
		case "event_msg":
			payload, _ := entry["payload"].(map[string]interface{})
			if payload == nil {
				continue
			}
			payloadType, _ := payload["type"].(string)
			switch payloadType {
			case "user_message":
				msg, _ := payload["message"].(string)
				msg = strings.TrimSpace(msg)
				if msg == "" {
					continue
				}
				if firstPrompt == "" {
					firstPrompt = msg
				}
				pendingPrompts = append(pendingPrompts, struct {
					text string
					ts   string
				}{text: msg, ts: entryTS})
			case "token_count":
				info, _ := payload["info"].(map[string]interface{})
				if info == nil {
					continue
				}
				lastUsage, _ := info["last_token_usage"].(map[string]interface{})
				if lastUsage == nil {
					continue
				}

				prompt := ""
				promptTS := sessionTS
				if len(pendingPrompts) > 0 {
					prompt = pendingPrompts[0].text
					promptTS = pendingPrompts[0].ts
					pendingPrompts = pendingPrompts[1:]
				}

				input := toInt(lastUsage["input_tokens"])
				cached := toInt(lastUsage["cached_input_tokens"])
				output := toInt(lastUsage["output_tokens"]) + toInt(lastUsage["reasoning_output_tokens"])
				total := toInt(lastUsage["total_tokens"])
				if total == 0 {
					total = input + cached + output
				}

				queries = append(queries, Query{
					UserPrompt:         prompt,
					UserTimestamp:      promptTS,
					AssistantTimestamp: entryTS,
					Model:              "codex",
					InputTokens:        input,
					CacheReadTokens:    cached,
					OutputTokens:       output,
					TotalTokens:        total,
				})
			}
		}
	}

	if len(queries) == 0 {
		return nil
	}

	date := ""
	if sessionTS != "" {
		date = sessionTS
	} else {
		date = filepath.Base(path)
	}
	if len(date) >= 10 {
		date = date[:10]
	}

	if project == "" {
		project = filepath.Base(filepath.Dir(path))
	}
	if firstPrompt == "" {
		firstPrompt = "(sem prompt)"
	}

	s := &Session{
		SessionID:   sessionID,
		Project:     project,
		Date:        date,
		Timestamp:   sessionTS,
		FirstPrompt: truncatePrompt(firstPrompt, 200),
		Model:       "codex",
		QueryCount:  len(queries),
		Queries:     queries,
	}

	if s.SessionID == "" {
		s.SessionID = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}

	for _, q := range queries {
		s.InputTokens += q.InputTokens
		s.OutputTokens += q.OutputTokens
		s.CacheCreationTokens += q.CacheCreationTokens
		s.CacheReadTokens += q.CacheReadTokens
		s.TotalTokens += q.TotalTokens
	}

	return s
}

func latestCodexRateMeta() codexRateMeta {
	base := getCodexSessionsDir()
	if _, err := os.Stat(base); err != nil {
		return codexRateMeta{}
	}

	var latest codexRateMeta
	_ = filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() || filepath.Ext(path) != ".jsonl" {
			return nil
		}
		meta := latestCodexRateMetaFromFile(path)
		if meta.valid && meta.timestamp.After(latest.timestamp) {
			latest = meta
		}
		return nil
	})
	return latest
}

func latestCodexRateMetaFromFile(path string) codexRateMeta {
	file, err := os.Open(path)
	if err != nil {
		return codexRateMeta{}
	}
	defer file.Close()

	var latest codexRateMeta
	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024*10)

	for scanner.Scan() {
		var entry map[string]interface{}
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}
		if entry["type"] != "event_msg" {
			continue
		}
		payload, _ := entry["payload"].(map[string]interface{})
		if payload == nil || payload["type"] != "token_count" {
			continue
		}
		rateLimits, _ := payload["rate_limits"].(map[string]interface{})
		if rateLimits == nil {
			continue
		}
		primary, _ := rateLimits["primary"].(map[string]interface{})
		if primary == nil {
			continue
		}

		ts, _ := entry["timestamp"].(string)
		parsedTS, _ := time.Parse(time.RFC3339Nano, ts)
		plan, _ := rateLimits["plan_type"].(string)
		resetsAtUnix := int64(toInt(primary["resets_at"]))

		latest = codexRateMeta{
			valid:         true,
			usedPct:       toFloat(primary["used_percent"]),
			windowMinutes: toInt(primary["window_minutes"]),
			resetsAt:      time.Unix(resetsAtUnix, 0),
			plan:          strings.TrimSpace(plan),
			timestamp:     parsedTS,
		}
	}

	return latest
}

func truncatePrompt(s string, limit int) string {
	s = strings.TrimSpace(s)
	if len(s) <= limit {
		return s
	}
	return s[:limit]
}

func parseSessionTime(s Session) time.Time {
	if s.Timestamp != "" {
		if t, err := time.Parse(time.RFC3339Nano, s.Timestamp); err == nil {
			return t
		}
		if t, err := time.Parse(time.RFC3339, s.Timestamp); err == nil {
			return t
		}
	}
	if len(s.Date) == len("2006-01-02") {
		if t, err := time.Parse("2006-01-02", s.Date); err == nil {
			return t
		}
	}
	return time.Time{}
}

func maxPctLeft(used float64) float64 {
	if used >= 100 {
		return 0
	}
	if used <= 0 {
		return 100
	}
	return 100 - used
}

func toFloat(v interface{}) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case int:
		return float64(t)
	case int64:
		return float64(t)
	default:
		return 0
	}
}
