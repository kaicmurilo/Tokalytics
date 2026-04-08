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
	// even if no sessions, we might have API usage
	
	now := time.Now()
	today := now.Format("2006-01-02")

	var todayTokens, last30Tokens int
	var todaySessions, totalSessions int

	// 30-day cutoff
	cutoff := now.AddDate(0, 0, -30)

	for _, s := range sessions {
		totalSessions++
		// Use local time for session date
		sessionDate := s.StartTime.Local().Format("2006-01-02")
		if sessionDate == today {
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

	// 1. Get real API quotas
	apiWindows, plan, email, _ := fetchGeminiAPIUsage()
	if plan != "" {
		u.Plan = plan
	}
	if email != "" {
		u.Email = email
	}
	u.Windows = append(u.Windows, apiWindows...)

	// 2. Add local usage stats
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

