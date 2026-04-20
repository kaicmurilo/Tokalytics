package providers

import (
	"bufio"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Model pricing per token (Anthropic API equivalent estimates)
// Cache write = 1.25x base input. Cache read = 0.1x base input.
type pricingTier struct {
	input      float64
	output     float64
	cacheWrite float64
	cacheRead  float64
}

var modelPricing = map[string]pricingTier{
	// Opus 4.5, 4.6: $5/MTok in, $25/MTok out
	"opus-4.5": {5 / 1e6, 25 / 1e6, 6.25 / 1e6, 0.50 / 1e6},
	"opus-4.6": {5 / 1e6, 25 / 1e6, 6.25 / 1e6, 0.50 / 1e6},
	// Opus 4.0, 4.1: $15/MTok in, $75/MTok out
	"opus-4.0": {15 / 1e6, 75 / 1e6, 18.75 / 1e6, 1.50 / 1e6},
	"opus-4.1": {15 / 1e6, 75 / 1e6, 18.75 / 1e6, 1.50 / 1e6},
	// Sonnet (3.7, 4, 4.5, 4.6): $3/MTok in, $15/MTok out
	"sonnet": {3 / 1e6, 15 / 1e6, 3.75 / 1e6, 0.30 / 1e6},
	// Haiku 4.5: $1/MTok in, $5/MTok out
	"haiku-4.5": {1 / 1e6, 5 / 1e6, 1.25 / 1e6, 0.10 / 1e6},
	// Haiku 3.5: $0.80/MTok in, $4/MTok out
	"haiku-3.5": {0.80 / 1e6, 4 / 1e6, 1.00 / 1e6, 0.08 / 1e6},
}

var defaultPricing = modelPricing["sonnet"]

func getPricing(model string) pricingTier {
	if model == "" {
		return defaultPricing
	}
	m := strings.ToLower(model)
	if strings.Contains(m, "opus") {
		if strings.Contains(m, "4-6") || strings.Contains(m, "4.6") {
			return modelPricing["opus-4.6"]
		}
		if strings.Contains(m, "4-5") || strings.Contains(m, "4.5") {
			return modelPricing["opus-4.5"]
		}
		if strings.Contains(m, "4-1") || strings.Contains(m, "4.1") {
			return modelPricing["opus-4.1"]
		}
		return modelPricing["opus-4.0"]
	}
	if strings.Contains(m, "sonnet") {
		return modelPricing["sonnet"]
	}
	if strings.Contains(m, "haiku") {
		if strings.Contains(m, "4-5") || strings.Contains(m, "4.5") {
			return modelPricing["haiku-4.5"]
		}
		return modelPricing["haiku-3.5"]
	}
	return defaultPricing
}

// Estruturas correspondentes ao Node.js parser
type Query struct {
	UserPrompt          string   `json:"userPrompt"`
	UserTimestamp       string   `json:"userTimestamp,omitempty"`
	AssistantTimestamp  string   `json:"assistantTimestamp,omitempty"`
	Model               string   `json:"model"`
	InputTokens         int      `json:"inputTokens"`
	OutputTokens        int      `json:"outputTokens"`
	CacheCreationTokens int      `json:"cacheCreationTokens"`
	CacheReadTokens     int      `json:"cacheReadTokens"`
	TotalTokens         int      `json:"totalTokens"`
	Cost                float64  `json:"cost"`
	Tools               []string `json:"tools"`
}

type Session struct {
	SessionID           string  `json:"sessionId"`
	Project             string  `json:"project"`
	Date                string  `json:"date"`
	Timestamp           string  `json:"timestamp,omitempty"`
	FirstPrompt         string  `json:"firstPrompt"`
	Model               string  `json:"model"`
	QueryCount          int     `json:"queryCount"`
	Queries             []Query `json:"queries"`
	InputTokens         int     `json:"inputTokens"`
	OutputTokens        int     `json:"outputTokens"`
	CacheCreationTokens int     `json:"cacheCreationTokens"`
	CacheReadTokens     int     `json:"cacheReadTokens"`
	TotalTokens         int     `json:"totalTokens"`
	Cost                float64 `json:"cost"`
}

// LocalParser lida com a leitura de arquivos .jsonl
func ParseJSONLFile(filePath string) ([]map[string]interface{}, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var lines []map[string]interface{}
	scanner := bufio.NewScanner(file)
	// Claude code logs can be large, use a bigger buffer if needed
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024*10)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var entry map[string]interface{}
		if err := json.Unmarshal(line, &entry); err == nil {
			lines = append(lines, entry)
		}
	}
	return lines, scanner.Err()
}

func getClaudeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude")
}

// ParseClaudeSessions varre ~/.claude e retorna as sessões
func ParseClaudeSessions() []Session {
	dir := getClaudeDir()
	if dir == "" {
		return nil
	}

	var sessions []Session
	projectsDir := filepath.Join(dir, "projects")

	if _, err := os.Stat(projectsDir); os.IsNotExist(err) {
		return nil
	}

	// Walk projects
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		return nil
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		projectPath := filepath.Join(projectsDir, entry.Name())
		files, _ := os.ReadDir(projectPath)

		for _, file := range files {
			if filepath.Ext(file.Name()) != ".jsonl" {
				continue
			}

			filePath := filepath.Join(projectPath, file.Name())
			sessionID := strings.TrimSuffix(file.Name(), ".jsonl")

			rawEntries, err := ParseJSONLFile(filePath)
			if err != nil {
				continue
			}

			info, _ := file.Info()
			dateStr := info.ModTime().Format("2006-01-02T15:04:05Z")

			queries := extractQueries(rawEntries)
			if len(queries) == 0 {
				continue
			}

			// Use first timestamp from entries as the session date
			firstTimestamp := ""
			for _, e := range rawEntries {
				if ts, ok := e["timestamp"].(string); ok && ts != "" {
					firstTimestamp = ts
					break
				}
			}
			if firstTimestamp != "" {
				dateStr = firstTimestamp
			}
			date := dateStr
			if len(date) >= 10 {
				date = date[:10]
			}

			session := Session{
				SessionID:  sessionID,
				Project:    entry.Name(),
				Date:       date,
				Timestamp:  firstTimestamp,
				QueryCount: len(queries),
				Queries:    queries,
			}

			// Aggregate session totals + pick primary model
			modelCounts := map[string]int{}
			for _, q := range queries {
				session.InputTokens += q.InputTokens
				session.OutputTokens += q.OutputTokens
				session.CacheCreationTokens += q.CacheCreationTokens
				session.CacheReadTokens += q.CacheReadTokens
				session.TotalTokens += q.TotalTokens
				session.Cost += q.Cost
				if q.Model != "" && q.Model != "<synthetic>" {
					modelCounts[q.Model]++
				}
				if session.FirstPrompt == "" && q.UserPrompt != "" {
					session.FirstPrompt = q.UserPrompt
					if len(session.FirstPrompt) > 200 {
						session.FirstPrompt = session.FirstPrompt[:200]
					}
				}
			}
			// Primary model = most frequent
			bestCount := 0
			for m, c := range modelCounts {
				if c > bestCount {
					bestCount = c
					session.Model = m
				}
			}
			if session.FirstPrompt == "" {
				session.FirstPrompt = "(sem prompt)"
			}

			sessions = append(sessions, session)
		}
	}

	return sessions
}

func toInt(v interface{}) int {
	if v == nil {
		return 0
	}
	if f, ok := v.(float64); ok {
		return int(f)
	}
	return 0
}

// usageNum lê o primeiro campo presente em usage (ordem de preferência).
func usageNum(usage map[string]interface{}, keys ...string) int {
	for _, k := range keys {
		if _, ok := usage[k]; ok {
			return toInt(usage[k])
		}
	}
	return 0
}

// cacheCreationInputTokens aceita cache_creation_input_tokens plano ou o objeto aninhado
// cache_creation do Claude Code recente (ex.: ephemeral_5m_input_tokens).
func cacheCreationInputTokens(usage map[string]interface{}) int {
	v := usageNum(usage, "cache_creation_input_tokens", "cacheCreationInputTokens")
	if v > 0 {
		return v
	}
	cc, ok := usage["cache_creation"].(map[string]interface{})
	if !ok {
		cc, ok = usage["cacheCreation"].(map[string]interface{})
	}
	if !ok {
		return 0
	}
	sum := 0
	for _, val := range cc {
		switch t := val.(type) {
		case map[string]interface{}:
			sum += usageNum(t, "input_tokens", "inputTokens")
		default:
			sum += toInt(val)
		}
	}
	return sum
}

func extractQueries(entries []map[string]interface{}) []Query {
	var queries []Query
	var pendingPrompt string
	var pendingTimestamp string

	for _, entry := range entries {
		entryType, _ := entry["type"].(string)
		if entryType == "" {
			entryType, _ = entry["role"].(string)
		}
		entryType = strings.ToLower(strings.TrimSpace(entryType))
		entryTimestamp, _ := entry["timestamp"].(string)
		isMeta, _ := entry["isMeta"].(bool)

		if entryType == "user" && !isMeta {
			if msg, ok := entry["message"].(map[string]interface{}); ok {
				// Claude Code: envelope type=user e message.role=user.
				// Cursor (Linux): envelope role=user sem message.role — não exigir role interno.
				if r, ok := msg["role"].(string); ok && strings.ToLower(r) != "user" {
					continue
				}
				switch content := msg["content"].(type) {
				case string:
					if strings.HasPrefix(content, "<local-command") || strings.HasPrefix(content, "<command-name") {
						continue
					}
					pendingPrompt = content
				case []interface{}:
					var parts []string
					for _, b := range content {
						if block, ok := b.(map[string]interface{}); ok {
							if block["type"] == "text" {
								if t, ok := block["text"].(string); ok {
									parts = append(parts, t)
								}
							}
						}
					}
					pendingPrompt = strings.TrimSpace(strings.Join(parts, "\n"))
				}
				pendingTimestamp = entryTimestamp
			}
		}

		if entryType == "assistant" {
			if msg, ok := entry["message"].(map[string]interface{}); ok {
				usage, _ := msg["usage"].(map[string]interface{})
				if usage == nil {
					usage, _ = entry["usage"].(map[string]interface{})
				}
				model, _ := msg["model"].(string)

				if model == "<synthetic>" {
					continue
				}

				// Extract tool names (Claude tool_use; Cursor pode usar nomes diferentes no futuro)
				var tools []string
				if contentArr, ok := msg["content"].([]interface{}); ok {
					for _, b := range contentArr {
						if block, ok := b.(map[string]interface{}); ok {
							if block["type"] == "tool_use" {
								if name, ok := block["name"].(string); ok {
									tools = append(tools, name)
								}
							}
						}
					}
				}

				if usage == nil {
					// Transcripts do Cursor no Linux costumam não persistir usage por turno;
					// ainda registramos a troca para listar sessões e prompts (tokens = 0).
					if pendingPrompt == "" {
						continue
					}
					if model == "" {
						model = "cursor-transcript"
					}
					queries = append(queries, Query{
						UserPrompt:         pendingPrompt,
						UserTimestamp:      pendingTimestamp,
						AssistantTimestamp: entryTimestamp,
						Model:              model,
						Tools:              tools,
					})
					pendingPrompt = ""
					pendingTimestamp = ""
					continue
				}

				input := usageNum(usage, "input_tokens", "inputTokens")
				output := usageNum(usage, "output_tokens", "outputTokens")
				cacheWrite := cacheCreationInputTokens(usage)
				cacheRead := usageNum(usage, "cache_read_input_tokens", "cacheReadInputTokens")

				pricing := getPricing(model)
				cost := float64(input)*pricing.input +
					float64(cacheWrite)*pricing.cacheWrite +
					float64(cacheRead)*pricing.cacheRead +
					float64(output)*pricing.output

				q := Query{
					UserPrompt:          pendingPrompt,
					UserTimestamp:       pendingTimestamp,
					AssistantTimestamp:  entryTimestamp,
					Model:               model,
					InputTokens:         input,
					OutputTokens:        output,
					CacheCreationTokens: cacheWrite,
					CacheReadTokens:     cacheRead,
					TotalTokens:         input + output + cacheWrite + cacheRead,
					Cost:                cost,
					Tools:               tools,
				}
				queries = append(queries, q)
				pendingPrompt = ""
				pendingTimestamp = ""
			}
		}
	}
	return queries
}

// TodayStats holds a quick summary of today's local usage
type TodayStats struct {
	TotalTokens         int     `json:"totalTokens"`
	TotalCost           float64 `json:"totalCost"`
	Sessions            int     `json:"sessions"`
	InputTokens         int     `json:"inputTokens"`
	OutputTokens        int     `json:"outputTokens"`
	CacheCreationTokens int     `json:"cacheCreationTokens"`
	CacheReadTokens     int     `json:"cacheReadTokens"`
}

// GetTodayStats agrega sessões locais (Claude Code + Cursor) do dia corrente.
func GetTodayStats() TodayStats {
	today := time.Now().Format("2006-01-02")
	var stats TodayStats
	for _, s := range ParseClaudeSessions() {
		if s.Date != today {
			continue
		}
		stats.Sessions++
		stats.TotalTokens += s.TotalTokens
		stats.TotalCost += s.Cost
		stats.InputTokens += s.InputTokens
		stats.OutputTokens += s.OutputTokens
		stats.CacheCreationTokens += s.CacheCreationTokens
		stats.CacheReadTokens += s.CacheReadTokens
	}
	for _, s := range ParseCursorSessions() {
		if s.Date != today {
			continue
		}
		stats.Sessions++
		stats.TotalTokens += s.TotalTokens
		stats.TotalCost += s.Cost
		stats.InputTokens += s.InputTokens
		stats.OutputTokens += s.OutputTokens
		stats.CacheCreationTokens += s.CacheCreationTokens
		stats.CacheReadTokens += s.CacheReadTokens
	}
	for _, s := range ParseCodexSessions() {
		if s.Date != today {
			continue
		}
		stats.Sessions++
		stats.TotalTokens += s.TotalTokens
		stats.TotalCost += s.Cost
		stats.InputTokens += s.InputTokens
		stats.OutputTokens += s.OutputTokens
		stats.CacheCreationTokens += s.CacheCreationTokens
		stats.CacheReadTokens += s.CacheReadTokens
	}
	return stats
}

func getCursorDir() string {
	if p := strings.TrimSpace(os.Getenv("TOKALYTICS_CURSOR_HOME")); p != "" {
		return p
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cursor")
}

// cursorTranscriptSessionSlug gera um identificador estável a partir do caminho relativo
// a .../agent-transcripts/. Preserva o formato legado cursor-<uuid> para <uuid>/<uuid>.jsonl.
func cursorTranscriptSessionSlug(relPath string) string {
	rel := filepath.ToSlash(relPath)
	relKey := strings.TrimSuffix(rel, ".jsonl")
	parts := strings.Split(relKey, "/")
	if len(parts) == 2 && parts[0] == parts[1] {
		return parts[0]
	}
	return strings.ReplaceAll(relKey, "/", "-")
}

func ParseCursorSessions() []Session {
	dir := getCursorDir()
	projectsDir := filepath.Join(dir, "projects")

	var sessions []Session

	if _, err := os.Stat(projectsDir); os.IsNotExist(err) {
		return nil
	}

	entries, _ := os.ReadDir(projectsDir)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		transcriptsDir := filepath.Join(projectsDir, entry.Name(), "agent-transcripts")
		if _, err := os.Stat(transcriptsDir); os.IsNotExist(err) {
			continue
		}

		_ = filepath.WalkDir(transcriptsDir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			if !strings.EqualFold(filepath.Ext(d.Name()), ".jsonl") {
				return nil
			}
			rel, err := filepath.Rel(transcriptsDir, path)
			if err != nil {
				return nil
			}
			sessionKey := cursorTranscriptSessionSlug(rel)

			rawEntries, err := ParseJSONLFile(path)
			if err != nil {
				return nil
			}

			info, _ := d.Info()
			dateStr := ""
			if info != nil {
				dateStr = info.ModTime().Format("2006-01-02T15:04:05Z")
			}

			queries := extractQueries(rawEntries)
			if len(queries) == 0 {
				return nil
			}

			firstTimestamp := ""
			for _, e := range rawEntries {
				if ts, ok := e["timestamp"].(string); ok && ts != "" {
					firstTimestamp = ts
					break
				}
			}
			if firstTimestamp != "" {
				dateStr = firstTimestamp
			}
			date := dateStr
			if len(date) >= 10 {
				date = date[:10]
			}

			sess := Session{
				SessionID:  "cursor-" + sessionKey,
				Project:    entry.Name(),
				Date:       date,
				Timestamp:  firstTimestamp,
				QueryCount: len(queries),
				Queries:    queries,
			}
			if len(queries) > 0 {
				sess.FirstPrompt = queries[0].UserPrompt
			}
			modelCounts := map[string]int{}
			for _, q := range queries {
				sess.InputTokens += q.InputTokens
				sess.OutputTokens += q.OutputTokens
				sess.CacheCreationTokens += q.CacheCreationTokens
				sess.CacheReadTokens += q.CacheReadTokens
				sess.TotalTokens += q.TotalTokens
				sess.Cost += q.Cost
				if q.Model != "" && q.Model != "<synthetic>" {
					modelCounts[q.Model]++
				}
			}
			bestCount := 0
			for m, c := range modelCounts {
				if c > bestCount {
					bestCount = c
					sess.Model = m
				}
			}
			sessions = append(sessions, sess)
			return nil
		})
	}
	return sessions
}
