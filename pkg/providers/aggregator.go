package providers

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// ─── Output types ────────────────────────────────────────────────────────────

type DailyUsage struct {
	Date                string  `json:"date"`
	InputTokens         int     `json:"inputTokens"`
	OutputTokens        int     `json:"outputTokens"`
	CacheCreationTokens int     `json:"cacheCreationTokens"`
	CacheReadTokens     int     `json:"cacheReadTokens"`
	TotalTokens         int     `json:"totalTokens"`
	Cost                float64 `json:"cost"`
	Sessions            int     `json:"sessions"`
	Queries             int     `json:"queries"`
}

type ModelUsage struct {
	Model               string  `json:"model"`
	InputTokens         int     `json:"inputTokens"`
	OutputTokens        int     `json:"outputTokens"`
	CacheCreationTokens int     `json:"cacheCreationTokens"`
	CacheReadTokens     int     `json:"cacheReadTokens"`
	TotalTokens         int     `json:"totalTokens"`
	Cost                float64 `json:"cost"`
	QueryCount          int     `json:"queryCount"`
}

type TopPrompt struct {
	Prompt              string  `json:"prompt"`
	InputTokens         int     `json:"inputTokens"`
	OutputTokens        int     `json:"outputTokens"`
	CacheCreationTokens int     `json:"cacheCreationTokens"`
	CacheReadTokens     int     `json:"cacheReadTokens"`
	TotalTokens         int     `json:"totalTokens"`
	Cost                float64 `json:"cost"`
	Date                string  `json:"date"`
	SessionID           string  `json:"sessionId"`
	Model               string  `json:"model"`
}

type ProjectBreakdown struct {
	Project        string       `json:"project"`
	InputTokens    int          `json:"inputTokens"`
	OutputTokens   int          `json:"outputTokens"`
	TotalTokens    int          `json:"totalTokens"`
	SessionCount   int          `json:"sessionCount"`
	QueryCount     int          `json:"queryCount"`
	ModelBreakdown []ModelUsage `json:"modelBreakdown"`
	TopPrompts     []TopPrompt  `json:"topPrompts"`
}

type DateRange struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type Totals struct {
	TotalSessions            int        `json:"totalSessions"`
	TotalQueries             int        `json:"totalQueries"`
	TotalTokens              int        `json:"totalTokens"`
	TotalInputTokens         int        `json:"totalInputTokens"`
	TotalOutputTokens        int        `json:"totalOutputTokens"`
	TotalCacheCreationTokens int        `json:"totalCacheCreationTokens"`
	TotalCacheReadTokens     int        `json:"totalCacheReadTokens"`
	TotalCost                float64    `json:"totalCost"`
	TotalSaved               float64    `json:"totalSaved"`
	CacheHitRate             float64    `json:"cacheHitRate"`
	AvgTokensPerQuery        int        `json:"avgTokensPerQuery"`
	AvgTokensPerSession      int        `json:"avgTokensPerSession"`
	DateRange                *DateRange `json:"dateRange"`
}

type Insight struct {
	ID          string `json:"id"`
	Type        string `json:"type"` // "warning", "info", "neutral"
	Title       string `json:"title"`
	Description string `json:"description"`
	Action      string `json:"action,omitempty"`
}

type AggregatedData struct {
	Sessions         []Session          `json:"sessions"`
	DailyUsage       []DailyUsage       `json:"dailyUsage"`
	ModelBreakdown   []ModelUsage       `json:"modelBreakdown"`
	ProjectBreakdown []ProjectBreakdown `json:"projectBreakdown"`
	TopPrompts       []TopPrompt        `json:"topPrompts"`
	Totals           Totals             `json:"totals"`
	Insights         []Insight          `json:"insights"`
}

// ─── Main aggregation ─────────────────────────────────────────────────────────

func Aggregate(sessions []Session) AggregatedData {
	// Sort sessions by total tokens desc
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].TotalTokens > sessions[j].TotalTokens
	})

	dailyMap := map[string]*DailyUsage{}
	modelMap := map[string]*ModelUsage{}
	projectMap := map[string]*struct {
		pb      ProjectBreakdown
		modelMp map[string]*ModelUsage
		prompts []TopPrompt
	}{}
	var allPrompts []TopPrompt

	for i := range sessions {
		s := &sessions[i]

		// Daily
		if s.Date != "" && s.Date != "unknown" {
			d := s.Date
			if len(d) > 10 {
				d = d[:10]
			}
			if dailyMap[d] == nil {
				dailyMap[d] = &DailyUsage{Date: d}
			}
			dailyMap[d].InputTokens += s.InputTokens
			dailyMap[d].OutputTokens += s.OutputTokens
			dailyMap[d].CacheCreationTokens += s.CacheCreationTokens
			dailyMap[d].CacheReadTokens += s.CacheReadTokens
			dailyMap[d].TotalTokens += s.TotalTokens
			dailyMap[d].Cost += s.Cost
			dailyMap[d].Sessions++
			dailyMap[d].Queries += s.QueryCount
		}

		// Model breakdown
		for _, q := range s.Queries {
			if q.Model == "" || q.Model == "<synthetic>" || q.Model == "unknown" {
				continue
			}
			if modelMap[q.Model] == nil {
				modelMap[q.Model] = &ModelUsage{Model: q.Model}
			}
			mu := modelMap[q.Model]
			mu.InputTokens += q.InputTokens
			mu.OutputTokens += q.OutputTokens
			mu.CacheCreationTokens += q.CacheCreationTokens
			mu.CacheReadTokens += q.CacheReadTokens
			mu.TotalTokens += q.TotalTokens
			mu.Cost += q.Cost
			mu.QueryCount++
		}

		// Project breakdown
		proj := s.Project
		if proj == "" {
			proj = "unknown"
		}
		if projectMap[proj] == nil {
			projectMap[proj] = &struct {
				pb      ProjectBreakdown
				modelMp map[string]*ModelUsage
				prompts []TopPrompt
			}{
				pb:      ProjectBreakdown{Project: proj},
				modelMp: map[string]*ModelUsage{},
			}
		}
		pe := projectMap[proj]
		pe.pb.InputTokens += s.InputTokens
		pe.pb.OutputTokens += s.OutputTokens
		pe.pb.TotalTokens += s.TotalTokens
		pe.pb.SessionCount++
		pe.pb.QueryCount += s.QueryCount

		for _, q := range s.Queries {
			if q.Model != "" && q.Model != "<synthetic>" {
				if pe.modelMp[q.Model] == nil {
					pe.modelMp[q.Model] = &ModelUsage{Model: q.Model}
				}
				pm := pe.modelMp[q.Model]
				pm.InputTokens += q.InputTokens
				pm.OutputTokens += q.OutputTokens
				pm.TotalTokens += q.TotalTokens
				pm.QueryCount++
			}
		}

		// Top prompts (group consecutive queries under same user prompt)
		type promptAcc struct {
			prompt              string
			inputTokens         int
			outputTokens        int
			cacheCreationTokens int
			cacheReadTokens     int
			cost                float64
		}
		var cur *promptAcc
		flush := func() {
			if cur != nil && cur.prompt != "" {
				total := cur.inputTokens + cur.outputTokens + cur.cacheCreationTokens + cur.cacheReadTokens
				tp := TopPrompt{
					Prompt:              cur.prompt,
					InputTokens:         cur.inputTokens,
					OutputTokens:        cur.outputTokens,
					CacheCreationTokens: cur.cacheCreationTokens,
					CacheReadTokens:     cur.cacheReadTokens,
					TotalTokens:         total,
					Cost:                cur.cost,
					Date:                s.Date,
					SessionID:           s.SessionID,
					Model:               s.Model,
				}
				allPrompts = append(allPrompts, tp)
				pe.prompts = append(pe.prompts, tp)
			}
		}
		for _, q := range s.Queries {
			p := q.UserPrompt
			if p != "" && (cur == nil || p != cur.prompt) {
				flush()
				txt := p
				if len(txt) > 300 {
					txt = txt[:300]
				}
				cur = &promptAcc{prompt: txt}
			}
			if cur != nil {
				cur.inputTokens += q.InputTokens
				cur.outputTokens += q.OutputTokens
				cur.cacheCreationTokens += q.CacheCreationTokens
				cur.cacheReadTokens += q.CacheReadTokens
				cur.cost += q.Cost
			}
		}
		flush()
	}

	// Build slices
	dailyUsage := make([]DailyUsage, 0, len(dailyMap))
	for _, v := range dailyMap {
		dailyUsage = append(dailyUsage, *v)
	}
	sort.Slice(dailyUsage, func(i, j int) bool {
		return dailyUsage[i].Date < dailyUsage[j].Date
	})

	modelBreakdown := make([]ModelUsage, 0, len(modelMap))
	for _, v := range modelMap {
		modelBreakdown = append(modelBreakdown, *v)
	}
	sort.Slice(modelBreakdown, func(i, j int) bool {
		return modelBreakdown[i].TotalTokens > modelBreakdown[j].TotalTokens
	})

	projectBreakdown := make([]ProjectBreakdown, 0, len(projectMap))
	for _, pe := range projectMap {
		// sort model breakdown inside project
		mbs := make([]ModelUsage, 0, len(pe.modelMp))
		for _, v := range pe.modelMp {
			mbs = append(mbs, *v)
		}
		sort.Slice(mbs, func(i, j int) bool { return mbs[i].TotalTokens > mbs[j].TotalTokens })
		pe.pb.ModelBreakdown = mbs

		// top 10 prompts for project
		sort.Slice(pe.prompts, func(i, j int) bool { return pe.prompts[i].TotalTokens > pe.prompts[j].TotalTokens })
		if len(pe.prompts) > 10 {
			pe.prompts = pe.prompts[:10]
		}
		pe.pb.TopPrompts = pe.prompts
		projectBreakdown = append(projectBreakdown, pe.pb)
	}
	sort.Slice(projectBreakdown, func(i, j int) bool {
		return projectBreakdown[i].TotalTokens > projectBreakdown[j].TotalTokens
	})

	// Top 20 prompts globally
	sort.Slice(allPrompts, func(i, j int) bool {
		return allPrompts[i].TotalTokens > allPrompts[j].TotalTokens
	})
	topPrompts := allPrompts
	if len(topPrompts) > 20 {
		topPrompts = topPrompts[:20]
	}

	// Totals
	totals := Totals{TotalSessions: len(sessions)}
	for _, s := range sessions {
		totals.TotalQueries += s.QueryCount
		totals.TotalTokens += s.TotalTokens
		totals.TotalInputTokens += s.InputTokens
		totals.TotalOutputTokens += s.OutputTokens
		totals.TotalCacheCreationTokens += s.CacheCreationTokens
		totals.TotalCacheReadTokens += s.CacheReadTokens
		totals.TotalCost += s.Cost
	}

	// Cache savings: reads at full input price minus what they actually cost
	avgIn := defaultPricing.input
	avgRead := defaultPricing.cacheRead
	totals.TotalSaved = float64(totals.TotalCacheReadTokens) * (avgIn - avgRead)

	totalAllInput := totals.TotalInputTokens + totals.TotalCacheCreationTokens + totals.TotalCacheReadTokens
	if totalAllInput > 0 {
		totals.CacheHitRate = float64(totals.TotalCacheReadTokens) / float64(totalAllInput)
	}
	if totals.TotalQueries > 0 {
		totals.AvgTokensPerQuery = totals.TotalTokens / totals.TotalQueries
	}
	if totals.TotalSessions > 0 {
		totals.AvgTokensPerSession = totals.TotalTokens / totals.TotalSessions
	}
	if len(dailyUsage) > 0 {
		totals.DateRange = &DateRange{
			From: dailyUsage[0].Date,
			To:   dailyUsage[len(dailyUsage)-1].Date,
		}
	}

	insights := generateInsights(sessions, allPrompts, totals)
	if insights == nil {
		insights = []Insight{}
	}
	if sessions == nil {
		sessions = []Session{}
	}
	if topPrompts == nil {
		topPrompts = []TopPrompt{}
	}

	return AggregatedData{
		Sessions:         sessions,
		DailyUsage:       dailyUsage,
		ModelBreakdown:   modelBreakdown,
		ProjectBreakdown: projectBreakdown,
		TopPrompts:       topPrompts,
		Totals:           totals,
		Insights:         insights,
	}
}

// ─── Insights ─────────────────────────────────────────────────────────────────

func fmtTokens(n int) string {
	if n >= 1_000_000_000 {
		return fmt.Sprintf("%.1fB", float64(n)/1e9)
	}
	if n >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(n)/1e6)
	}
	if n >= 1000 {
		return fmt.Sprintf("%.1fK", float64(n)/1e3)
	}
	return fmt.Sprintf("%d", n)
}

func generateInsights(sessions []Session, allPrompts []TopPrompt, totals Totals) []Insight {
	var insights []Insight

	// 1. Short vague messages that cost a lot
	var shortExpensive []TopPrompt
	for _, p := range allPrompts {
		if len(strings.TrimSpace(p.Prompt)) < 30 && p.TotalTokens > 100_000 {
			shortExpensive = append(shortExpensive, p)
		}
	}
	if len(shortExpensive) > 0 {
		totalWasted := 0
		seenPrompts := map[string]bool{}
		var examples []string
		for _, p := range shortExpensive {
			totalWasted += p.TotalTokens
			t := strings.TrimSpace(p.Prompt)
			if !seenPrompts[t] && len(examples) < 4 {
				seenPrompts[t] = true
				examples = append(examples, `"`+t+`"`)
			}
		}
		insights = append(insights, Insight{
			ID:          "vague-prompts",
			Type:        "warning",
			Title:       "Mensagens curtas e vagas estão custando mais",
			Description: fmt.Sprintf("%d vezes você enviou uma mensagem curta como %s — e cada mensagem queimou pelo menos 100K tokens. No total: %s tokens.", len(shortExpensive), strings.Join(examples, ", "), fmtTokens(totalWasted)),
			Action:      `Seja específico. Em vez de "Sim", diga "Sim, atualize a página de login e rode os testes."`,
		})
	}

	// 2. Context growth — conversations getting more expensive over time
	var longSessions []Session
	for _, s := range sessions {
		if s.QueryCount > 50 {
			longSessions = append(longSessions, s)
		}
	}
	if len(longSessions) > 0 {
		type growthEntry struct {
			session Session
			ratio   float64
		}
		var growthData []growthEntry
		for _, s := range longSessions {
			if len(s.Queries) < 10 {
				continue
			}
			var first5, last5 float64
			for _, q := range s.Queries[:5] {
				first5 += float64(q.TotalTokens)
			}
			first5 /= 5
			tail := s.Queries[len(s.Queries)-5:]
			for _, q := range tail {
				last5 += float64(q.TotalTokens)
			}
			last5 /= float64(len(tail))
			if first5 > 0 && last5/first5 > 2 {
				growthData = append(growthData, growthEntry{s, last5 / first5})
			}
		}
		if len(growthData) > 0 {
			var avgGrowth float64
			for _, g := range growthData {
				avgGrowth += g.ratio
			}
			avgGrowth /= float64(len(growthData))
			sort.Slice(growthData, func(i, j int) bool { return growthData[i].ratio > growthData[j].ratio })
			worst := growthData[0]
			insights = append(insights, Insight{
				ID:          "context-growth",
				Type:        "warning",
				Title:       "Quanto mais longa a conversa, mais cara fica cada mensagem",
				Description: fmt.Sprintf("Em %d conversas, as mensagens do final custam %.1fx mais que as do início. Sua conversa mais longa (%q) ficou %.1fx mais cara.", len(growthData), avgGrowth, truncate(worst.session.FirstPrompt, 50), worst.ratio),
				Action:      "Inicie uma nova conversa ao mudar de tarefa. Se precisar de contexto anterior, cole um resumo curto na primeira mensagem.",
			})
		}
	}

	// 3. Marathon sessions
	longCount := 0
	var longTokens int
	var turnCounts []int
	for _, s := range sessions {
		turnCounts = append(turnCounts, s.QueryCount)
		if s.QueryCount > 200 {
			longCount++
			longTokens += s.TotalTokens
		}
	}
	sort.Ints(turnCounts)
	medianTurns := 0
	if len(turnCounts) > 0 {
		medianTurns = turnCounts[len(turnCounts)/2]
	}
	if longCount >= 3 {
		longPct := 0
		if totals.TotalTokens > 0 {
			longPct = longTokens * 100 / totals.TotalTokens
		}
		insights = append(insights, Insight{
			ID:          "marathon-sessions",
			Type:        "info",
			Title:       fmt.Sprintf("Apenas %d conversas longas usaram %d%% de todos os tokens", longCount, longPct),
			Description: fmt.Sprintf("Você tem %d conversas com mais de 200 mensagens. Juntas consumiram %s tokens — %d%% do total. Sua conversa típica tem ~%d mensagens.", longCount, fmtTokens(longTokens), longPct, medianTurns),
			Action:      "Tente manter uma conversa por tarefa. Quando uma conversa começar a misturar tópicos, é hora de começar uma nova.",
		})
	}

	// 4. Input-heavy usage
	if totals.TotalTokens > 0 {
		outputPct := float64(totals.TotalOutputTokens) / float64(totals.TotalTokens) * 100
		if outputPct < 2 {
			insights = append(insights, Insight{
				ID:          "input-heavy",
				Type:        "info",
				Title:       fmt.Sprintf("%.1f%% dos seus tokens são o Claude escrevendo de fato", outputPct),
				Description: fmt.Sprintf("De %s tokens totais, apenas %s são respostas do Claude. Os outros %.1f%% são o Claude relendo o histórico da conversa, arquivos e contexto.", fmtTokens(totals.TotalTokens), fmtTokens(totals.TotalOutputTokens), 100-outputPct),
				Action:      "Conversas mais curtas têm mais impacto do que pedir respostas mais curtas.",
			})
		}
	}

	// 5. Day-of-week pattern
	if len(sessions) >= 10 {
		dayNames := []string{"Domingo", "Segunda", "Terça", "Quarta", "Quinta", "Sexta", "Sábado"}
		type dayStats struct {
			tokens   int
			sessions int
		}
		dayMap := map[int]*dayStats{}
		for _, s := range sessions {
			if s.Timestamp == "" {
				continue
			}
			t, err := time.Parse(time.RFC3339, s.Timestamp)
			if err != nil {
				continue
			}
			d := int(t.Weekday())
			if dayMap[d] == nil {
				dayMap[d] = &dayStats{}
			}
			dayMap[d].tokens += s.TotalTokens
			dayMap[d].sessions++
		}
		type dayEntry struct {
			name string
			avg  float64
		}
		var days []dayEntry
		for d, v := range dayMap {
			if v.sessions > 0 {
				days = append(days, dayEntry{dayNames[d], float64(v.tokens) / float64(v.sessions)})
			}
		}
		if len(days) >= 3 {
			sort.Slice(days, func(i, j int) bool { return days[i].avg > days[j].avg })
			busiest := days[0]
			quietest := days[len(days)-1]
			insights = append(insights, Insight{
				ID:          "day-pattern",
				Type:        "neutral",
				Title:       fmt.Sprintf("Você usa mais o Claude nas %ss", busiest.name),
				Description: fmt.Sprintf("Suas conversas de %s têm em média %s tokens, contra %s nas %ss.", busiest.name, fmtTokens(int(busiest.avg)), fmtTokens(int(quietest.avg)), quietest.name),
			})
		}
	}

	// 6. Model mismatch — Opus for simple tasks
	var simpleOpus []Session
	for _, s := range sessions {
		if strings.Contains(strings.ToLower(s.Model), "opus") && s.QueryCount < 10 && s.TotalTokens < 200_000 {
			simpleOpus = append(simpleOpus, s)
		}
	}
	if len(simpleOpus) >= 3 {
		wastedTokens := 0
		var examples []string
		for _, s := range simpleOpus {
			wastedTokens += s.TotalTokens
			if len(examples) < 3 {
				examples = append(examples, `"`+truncate(s.FirstPrompt, 40)+`"`)
			}
		}
		insights = append(insights, Insight{
			ID:          "model-mismatch",
			Type:        "warning",
			Title:       fmt.Sprintf("%d conversas simples usaram Opus desnecessariamente", len(simpleOpus)),
			Description: fmt.Sprintf("Essas conversas tinham menos de 10 mensagens e usaram %s tokens no Opus: %s.", fmtTokens(wastedTokens), strings.Join(examples, ", ")),
			Action:      "Use /model para trocar para Sonnet ou Haiku em tarefas simples. Reserve o Opus para mudanças complexas e decisões de arquitetura.",
		})
	}

	// 7. Tool-heavy conversations
	if len(sessions) >= 5 {
		var toolHeavy []Session
		for _, s := range sessions {
			userMsgs := 0
			for _, q := range s.Queries {
				if q.UserPrompt != "" {
					userMsgs++
				}
			}
			toolCalls := s.QueryCount - userMsgs
			if userMsgs > 0 && toolCalls > userMsgs*3 {
				toolHeavy = append(toolHeavy, s)
			}
		}
		if len(toolHeavy) >= 3 {
			totalToolTokens := 0
			var totalRatio float64
			for _, s := range toolHeavy {
				totalToolTokens += s.TotalTokens
				userMsgs := 0
				for _, q := range s.Queries {
					if q.UserPrompt != "" {
						userMsgs++
					}
				}
				if userMsgs > 0 {
					totalRatio += float64(s.QueryCount-userMsgs) / float64(userMsgs)
				}
			}
			avgRatio := totalRatio / float64(len(toolHeavy))
			insights = append(insights, Insight{
				ID:          "tool-heavy",
				Type:        "info",
				Title:       fmt.Sprintf("%d conversas tiveram %.0fx mais chamadas de ferramenta que mensagens", len(toolHeavy), avgRatio),
				Description: fmt.Sprintf("Nessas conversas, o Claude fez ~%.0f chamadas de ferramenta por mensagem enviada. Cada chamada relê toda a conversa. Juntas usaram %s tokens.", avgRatio, fmtTokens(totalToolTokens)),
				Action:      `Aponte o Claude para arquivos e linhas específicas. "Corrija o bug em src/auth.go linha 42" gera menos chamadas de ferramenta que "corrija o bug de login".`,
			})
		}
	}

	// 8. Project dominance
	if len(sessions) >= 5 {
		projTokens := map[string]int{}
		for _, s := range sessions {
			proj := s.Project
			if proj == "" {
				proj = "unknown"
			}
			projTokens[proj] += s.TotalTokens
		}
		type projEntry struct {
			name   string
			tokens int
		}
		var sorted []projEntry
		for k, v := range projTokens {
			sorted = append(sorted, projEntry{k, v})
		}
		sort.Slice(sorted, func(i, j int) bool { return sorted[i].tokens > sorted[j].tokens })
		if len(sorted) >= 2 && totals.TotalTokens > 0 {
			top := sorted[0]
			pct := top.tokens * 100 / totals.TotalTokens
			if pct >= 60 {
				projName := top.name
				insights = append(insights, Insight{
					ID:          "project-dominance",
					Type:        "info",
					Title:       fmt.Sprintf("%d%% dos seus tokens foram para um projeto: %s", pct, projName),
					Description: fmt.Sprintf("O projeto %q usou %s tokens de %s totais (%d%%). O próximo mais próximo usou %s tokens.", projName, fmtTokens(top.tokens), fmtTokens(totals.TotalTokens), pct, fmtTokens(sorted[1].tokens)),
					Action:      "Não é necessariamente um problema, mas vale saber. Conversas longas neste projeto poderiam ser divididas em sessões menores.",
				})
			}
		}
	}

	// 9. Conversation efficiency
	if len(sessions) >= 10 {
		var shortAvgSum, longAvgSum float64
		var shortCount, longCount2 int
		for _, s := range sessions {
			if s.QueryCount >= 3 && s.QueryCount <= 15 {
				shortAvgSum += float64(s.TotalTokens) / float64(s.QueryCount)
				shortCount++
			} else if s.QueryCount > 80 {
				longAvgSum += float64(s.TotalTokens) / float64(s.QueryCount)
				longCount2++
			}
		}
		if shortCount >= 3 && longCount2 >= 2 {
			shortAvg := shortAvgSum / float64(shortCount)
			longAvg := longAvgSum / float64(longCount2)
			if shortAvg > 0 {
				ratio := longAvg / shortAvg
				if ratio >= 2 {
					insights = append(insights, Insight{
						ID:          "conversation-efficiency",
						Type:        "warning",
						Title:       fmt.Sprintf("Cada mensagem custa %.1fx mais em conversas longas", ratio),
						Description: fmt.Sprintf("Em conversas curtas (até 15 mensagens), cada mensagem custa ~%s tokens. Em longas (80+ mensagens), custa ~%s tokens — %.1fx mais.", fmtTokens(int(shortAvg)), fmtTokens(int(longAvg)), ratio),
						Action:      "Inicie novas conversas com mais frequência. Um fluxo de 5 conversas custa muito menos que uma maratona de 500 mensagens.",
					})
				}
			}
		}
	}

	// 10. Heavy context on first message
	if len(sessions) >= 5 {
		var heavyStarts []Session
		for _, s := range sessions {
			if len(s.Queries) > 0 && s.Queries[0].InputTokens > 50_000 {
				heavyStarts = append(heavyStarts, s)
			}
		}
		if len(heavyStarts) >= 5 {
			var sumStart int
			for _, s := range heavyStarts {
				sumStart += s.Queries[0].InputTokens
			}
			avgStart := sumStart / len(heavyStarts)
			insights = append(insights, Insight{
				ID:          "heavy-context",
				Type:        "info",
				Title:       fmt.Sprintf("%d conversas iniciaram com %s+ tokens de contexto", len(heavyStarts), fmtTokens(avgStart)),
				Description: fmt.Sprintf("Antes da sua primeira mensagem, o Claude lê CLAUDE.md, arquivos do projeto e contexto do sistema. Em %d conversas, esse contexto inicial foi em média %s tokens.", len(heavyStarts), fmtTokens(avgStart)),
				Action:      "Mantenha seus arquivos CLAUDE.md concisos. Remova seções que raramente usa. Contexto menor se multiplica em economia em cada mensagem.",
			})
		}
	}

	// 11. Cache efficiency
	if totals.TotalCacheReadTokens > 0 {
		hitRate := totals.CacheHitRate * 100
		withoutCaching := totals.TotalCost + totals.TotalSaved
		insights = append(insights, Insight{
			ID:          "cache-savings",
			Type:        "info",
			Title:       fmt.Sprintf("O cache te economizou aproximadamente $%.2f", totals.TotalSaved),
			Description: fmt.Sprintf("Sua taxa de acerto de cache é %.1f%%. Sem cache, sua estimativa seria $%.2f em vez de $%.2f. Leituras de cache ocorrem quando o Claude reutiliza partes da conversa que não mudaram.", hitRate, withoutCaching, totals.TotalCost),
		})
	}

	return insights
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
