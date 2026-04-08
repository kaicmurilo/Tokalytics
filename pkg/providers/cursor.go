package providers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type CursorProvider struct {
	CookieHeader string
}

func (p *CursorProvider) ID() string   { return "cursor" }
func (p *CursorProvider) Name() string { return "Cursor" }

func (p *CursorProvider) FetchUsage() (*Usage, error) {
	if p.CookieHeader == "" {
		return nil, fmt.Errorf("no cookies")
	}

	client := &http.Client{Timeout: 10 * time.Second}
	setHeaders := func(r *http.Request) {
		r.Header.Set("Cookie", p.CookieHeader)
		r.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")
		r.Header.Set("Accept", "application/json")
	}

	// 1. Get email from /api/auth/me
	email := ""
	req, _ := http.NewRequest("GET", "https://cursor.com/api/auth/me", nil)
	setHeaders(req)
	if resp, err := client.Do(req); err == nil && resp.StatusCode == 200 {
		var me map[string]interface{}
		if json.NewDecoder(resp.Body).Decode(&me) == nil {
			email, _ = me["email"].(string)
		}
		resp.Body.Close()
	}

	// 2. Get usage summary
	req, _ = http.NewRequest("GET", "https://cursor.com/api/usage-summary", nil)
	setHeaders(req)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cursor request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return nil, fmt.Errorf("cursor: sessão expirada")
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("cursor api status %d", resp.StatusCode)
	}

	var summary map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&summary); err != nil {
		return nil, fmt.Errorf("decode cursor: %v", err)
	}

	billingEnd := time.Time{}
	if s, ok := summary["billingCycleEnd"].(string); ok {
		billingEnd, _ = time.Parse(time.RFC3339, s)
	}
	membership, _ := summary["membershipType"].(string)

	u := &Usage{
		ProviderID:   "cursor",
		Name:         "Cursor",
		Email:        email,
		Plan:         membership,
		UpdatedAt:    "agora",
		TotalLimit:   100,
		SessionReset: billingEnd,
		WeeklyReset:  billingEnd,
	}

	indUsage, _ := summary["individualUsage"].(map[string]interface{})
	if indUsage == nil {
		return u, nil
	}
	plan, _ := indUsage["plan"].(map[string]interface{})
	if plan == nil {
		return u, nil
	}

	getF64 := func(m map[string]interface{}, key string) float64 {
		v, _ := m[key].(float64)
		return v
	}

	totalPct := getF64(plan, "totalPercentUsed")
	autoPct := getF64(plan, "autoPercentUsed")
	apiPct := getF64(plan, "apiPercentUsed")

	u.Used = totalPct
	u.Remaining = 100 - totalPct

	// Windows: Total, Auto, API — matching CodexBar exactly
	u.Windows = []RateWindow{
		{Name: "Total", PctUsed: totalPct, PctLeft: 100 - totalPct, ResetsAt: billingEnd},
		{Name: "Auto", PctUsed: autoPct, PctLeft: 100 - autoPct, ResetsAt: billingEnd},
		{Name: "API", PctUsed: apiPct, PctLeft: 100 - apiPct, ResetsAt: billingEnd},
	}

	// On-demand usage (credits beyond plan)
	if onDemand, ok := plan["onDemand"].(map[string]interface{}); ok {
		enabled, _ := onDemand["enabled"].(bool)
		usedCents := getF64(onDemand, "used")
		limitCents := getF64(onDemand, "limit")
		if enabled && limitCents > 0 {
			pct := usedCents / limitCents * 100
			u.Windows = append(u.Windows, RateWindow{
				Name:    fmt.Sprintf("On-demand ($%.2f / $%.2f)", usedCents/100, limitCents/100),
				PctUsed: pct,
				PctLeft: 100 - pct,
				ResetsAt: billingEnd,
			})
		}
	}

	return u, nil
}
