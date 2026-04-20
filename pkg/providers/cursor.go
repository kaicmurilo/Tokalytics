package providers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
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
		r.Header.Set("User-Agent", "Mozilla/5.0")
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

// CursorLocalProvider usa o JWT armazenado localmente pelo IDE (sem cookies).
// Chama api2.cursor.sh com Authorization: Bearer — funciona no Linux sem configuração manual.
type CursorLocalProvider struct {
	BearerToken string
}

func (p *CursorLocalProvider) ID() string   { return "cursor" }
func (p *CursorLocalProvider) Name() string { return "Cursor" }

func (p *CursorLocalProvider) FetchUsage() (*Usage, error) {
	if p.BearerToken == "" {
		return nil, fmt.Errorf("no bearer token")
	}

	client := &http.Client{Timeout: 10 * time.Second}
	setHeaders := func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer "+p.BearerToken)
		r.Header.Set("User-Agent", "Mozilla/5.0")
		r.Header.Set("Accept", "application/json")
	}

	// 1. Perfil/plano via full_stripe_profile
	var membership, subStatus string
	req, _ := http.NewRequest("GET", "https://api2.cursor.sh/auth/full_stripe_profile", nil)
	setHeaders(req)
	if resp, err := client.Do(req); err == nil && resp.StatusCode == 200 {
		var profile map[string]interface{}
		if json.NewDecoder(resp.Body).Decode(&profile) == nil {
			if m, ok := profile["individualMembershipType"].(string); ok && m != "" {
				membership = m
			} else if m, ok := profile["membershipType"].(string); ok {
				membership = m
			}
			if s, ok := profile["subscriptionStatus"].(string); ok {
				subStatus = s
			}
		}
		resp.Body.Close()
	}

	// 2. Uso por modelo via auth/usage
	req2, _ := http.NewRequest("GET", "https://api2.cursor.sh/auth/usage", nil)
	setHeaders(req2)
	resp2, err := client.Do(req2)
	if err != nil {
		return nil, fmt.Errorf("cursor usage request: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode == 401 || resp2.StatusCode == 403 {
		return nil, fmt.Errorf("cursor: token expirado ou sem permissão")
	}
	if resp2.StatusCode != 200 {
		return nil, fmt.Errorf("cursor api status %d", resp2.StatusCode)
	}

	var usageData map[string]interface{}
	if err := json.NewDecoder(resp2.Body).Decode(&usageData); err != nil {
		return nil, fmt.Errorf("decode cursor usage: %v", err)
	}

	plan := membership
	if subStatus != "" && subStatus != "active" {
		plan = membership + " (" + subStatus + ")"
	}

	u := &Usage{
		ProviderID: "cursor",
		Name:       "Cursor",
		Plan:       plan,
		UpdatedAt:  "agora",
		TotalLimit: 100,
	}

	// Calcula total de requests do mês e lista por modelo (auth/usage)
	var totalRequests int
	var modelReqs []ModelRequestStat
	for model, raw := range usageData {
		if model == "startOfMonth" {
			continue
		}
		if entry, ok := raw.(map[string]interface{}); ok {
			if n, ok := entry["numRequests"].(float64); ok {
				c := int(n)
				totalRequests += c
				modelReqs = append(modelReqs, ModelRequestStat{Model: model, Requests: c})
			}
		}
	}
	sort.Slice(modelReqs, func(i, j int) bool {
		if modelReqs[i].Requests != modelReqs[j].Requests {
			return modelReqs[i].Requests > modelReqs[j].Requests
		}
		return modelReqs[i].Model < modelReqs[j].Model
	})
	u.ModelRequests = modelReqs

	// Extrai data de início do mês para reset
	var monthStart time.Time
	if s, ok := usageData["startOfMonth"].(string); ok {
		monthStart, _ = time.Parse(time.RFC3339, s)
	}
	var resetAt time.Time
	if !monthStart.IsZero() {
		// Reset aproximado = 30 dias após início do período
		resetAt = monthStart.AddDate(0, 1, 0)
	}

	if totalRequests > 0 || !monthStart.IsZero() {
		u.Windows = []RateWindow{
			{
				Name:     fmt.Sprintf("Requests este mês: %d", totalRequests),
				PctUsed:  0,
				PctLeft:  100,
				ResetsAt: resetAt,
			},
		}
	}

	return u, nil
}
