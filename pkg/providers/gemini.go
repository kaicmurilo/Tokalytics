package providers

import (
	"fmt"
	"time"
)

type GeminiProvider struct{}

func (p *GeminiProvider) ID() string   { return "gemini" }
func (p *GeminiProvider) Name() string { return "Gemini" }

func (p *GeminiProvider) FetchUsage() (*Usage, error) {
	sessions := ParseGeminiSessions()
	if len(sessions) == 0 {
		return nil, fmt.Errorf("gemini: no session data found")
	}

	today := time.Now().Format("2006-01-02")

	var todayTokens, last30Tokens int
	var todaySessions, totalSessions int

	// 30-day cutoff
	cutoff := time.Now().AddDate(0, 0, -30)

	for _, s := range sessions {
		totalSessions++
		if s.Date == today {
			todaySessions++
			todayTokens += s.TotalTokens
		}
		if s.StartTime.After(cutoff) {
			last30Tokens += s.TotalTokens
		}
	}

	u := &Usage{
		ProviderID: "gemini",
		Name:       "Gemini",
		Plan:       "CLI",
		UpdatedAt:  "agora",
		TotalLimit: 100,
	}

	// Show token counts as windows (no % limit — free tier)
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

	u.TodayTokens = todayTokens
	u.Last30Tokens = last30Tokens

	return u, nil
}

