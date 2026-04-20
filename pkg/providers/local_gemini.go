package providers

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kaicmurilo/tokalytics/pkg/utils"
)

// GeminiSession holds aggregated data for one Gemini CLI session
type GeminiSession struct {
	SessionID     string
	Project       string
	Date          string
	StartTime     time.Time
	InputTokens   int
	OutputTokens  int
	CachedTokens  int
	ThoughtTokens int
	TotalTokens   int
	Model         string
	Messages      int
}

type geminiSessionFile struct {
	SessionID   string          `json:"sessionId"`
	StartTime   time.Time       `json:"startTime"`
	LastUpdated time.Time       `json:"lastUpdated"`
	Messages    []geminiMessage `json:"messages"`
}

type geminiMessage struct {
	ID        string    `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Type      string    `json:"type"`
	Tokens    *struct {
		Input    int `json:"input"`
		Output   int `json:"output"`
		Cached   int `json:"cached"`
		Thoughts int `json:"thoughts"`
		Tool     int `json:"tool"`
		Total    int `json:"total"`
	} `json:"tokens"`
	Model string `json:"model"`
}

// ParseGeminiSessions reads all session files from ~/.gemini/tmp/*/chats/session-*.json
func ParseGeminiSessions() []GeminiSession {
	var sessions []GeminiSession
	for _, homeRoot := range utils.DataHomeRoots() {
		base := filepath.Join(homeRoot, ".gemini", "tmp")
		entries, err := os.ReadDir(base)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			project := entry.Name()
			chatsDir := filepath.Join(base, project, "chats")
			files, err := os.ReadDir(chatsDir)
			if err != nil {
				continue
			}
			for _, f := range files {
				if !strings.HasPrefix(f.Name(), "session-") || !strings.HasSuffix(f.Name(), ".json") {
					continue
				}
				s := parseGeminiSessionFile(filepath.Join(chatsDir, f.Name()), project)
				if s != nil {
					sessions = append(sessions, *s)
				}
			}
		}
	}
	return sessions
}

func parseGeminiSessionFile(path, project string) *GeminiSession {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	var sf geminiSessionFile
	if err := json.Unmarshal(data, &sf); err != nil {
		return nil
	}

	s := &GeminiSession{
		SessionID: sf.SessionID,
		Project:   project,
		StartTime: sf.StartTime,
		Date:      sf.StartTime.Format("2006-01-02"),
	}

	for _, msg := range sf.Messages {
		if msg.Type != "gemini" || msg.Tokens == nil {
			continue
		}
		s.InputTokens += msg.Tokens.Input
		s.OutputTokens += msg.Tokens.Output
		s.CachedTokens += msg.Tokens.Cached
		s.ThoughtTokens += msg.Tokens.Thoughts
		s.TotalTokens += msg.Tokens.Total
		s.Messages++
		if msg.Model != "" {
			s.Model = msg.Model
		}
	}

	if s.TotalTokens == 0 && s.Messages == 0 {
		return nil
	}
	return s
}

// GetGeminiTodayStats returns aggregated token counts for today
func GetGeminiTodayStats() GeminiTodayStats {
	today := time.Now().Format("2006-01-02")
	var stats GeminiTodayStats
	for _, s := range ParseGeminiSessions() {
		stats.TotalSessions++
		stats.TotalTokens += s.TotalTokens
		stats.InputTokens += s.InputTokens
		stats.OutputTokens += s.OutputTokens
		if s.Date == today {
			stats.TodaySessions++
			stats.TodayTokens += s.TotalTokens
		}
	}
	return stats
}

type GeminiTodayStats struct {
	TodaySessions int
	TodayTokens   int
	TotalSessions int
	TotalTokens   int
	InputTokens   int
	OutputTokens  int
}
