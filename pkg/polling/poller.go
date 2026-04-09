package polling

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/getlantern/systray"
	"github.com/kaicmurilo/tokalytics/pkg/providers"
)

// MenuSlot is a pre-created systray menu item used as a display row
type MenuSlot struct {
	item    *systray.MenuItem
	visible bool
}

var (
	menuMu     sync.Mutex
	menuSlots  []*systray.MenuItem // pre-created display rows
	slotsReady bool
	// Headless é true quando não há menu bar (ex.: tokalytics -headless); updateTray não chama systray.
	Headless bool
)

// SetMenuSlots must be called from onReady() to register the pre-created items
func SetMenuSlots(slots []*systray.MenuItem) {
	menuMu.Lock()
	defer menuMu.Unlock()
	menuSlots = slots
	slotsReady = true
}

func fmtTok(n int) string {
	if n >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(n)/1e6)
	}
	if n >= 1_000 {
		return fmt.Sprintf("%.0fK", float64(n)/1e3)
	}
	return fmt.Sprintf("%d", n)
}

func countdown(t time.Time) string {
	now := time.Now()
	if t.IsZero() || t.Before(now) {
		return "agora"
	}
	secs := int(t.Sub(now).Seconds())
	totalMins := (secs + 59) / 60
	if totalMins < 1 {
		totalMins = 1
	}
	days := totalMins / (24 * 60)
	hours := (totalMins / 60) % 24
	mins := totalMins % 60
	switch {
	case days > 0 && hours > 0:
		return fmt.Sprintf("%dd %dh", days, hours)
	case days > 0:
		return fmt.Sprintf("%dd", days)
	case hours > 0 && mins > 0:
		return fmt.Sprintf("%dh %dm", hours, mins)
	case hours > 0:
		return fmt.Sprintf("%dh", hours)
	default:
		return fmt.Sprintf("%dm", totalMins)
	}
}

// renderBar returns an ASCII progress bar, e.g. "████░░░░░░ 42%"
func renderBar(pct float64) string {
	const total = 10
	filled := int(pct/10 + 0.5)
	if filled > total {
		filled = total
	}
	if filled < 0 {
		filled = 0
	}
	bar := strings.Repeat("█", filled) + strings.Repeat("░", total-filled)
	return fmt.Sprintf("%s %.0f%%", bar, pct)
}

func statusEmoji(pct float64) string {
	switch {
	case pct >= 90:
		return "🔴"
	case pct >= 70:
		return "🟡"
	default:
		return "🟢"
	}
}

// Start inicia o loop de polling
func Start() {
	log.Println("Iniciando daemon de polling do Tokalytics...")
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	updateTray()

	for range ticker.C {
		updateTray()
	}
}

// TriggerUpdate força uma atualização manual da bandeja
func TriggerUpdate() {
	updateTray()
}

// setSlot updates a pre-created menu slot by index
func setSlot(idx int, title string) {
	menuMu.Lock()
	defer menuMu.Unlock()
	if !slotsReady || idx >= len(menuSlots) {
		return
	}
	menuSlots[idx].SetTitle(title)
}

func updateTray() {
	usages := providers.RefreshUsages()
	if Headless {
		return
	}

	if len(usages) == 0 {
		stats := providers.GetTodayStats()
		if stats.TotalTokens > 0 {
			systray.SetTitle(fmt.Sprintf("↑ %s · $%.2f", fmtTok(stats.TotalTokens), stats.TotalCost))
			systray.SetTooltip("Tokalytics — sem dados de quota online")
		} else {
			systray.SetTitle("⟳")
			systray.SetTooltip("Tokalytics — atualizando...")
		}
		fillSlotsNoData()
		return
	}

	// Build compact tray title: just status emojis for each provider
	providerOrderTitle := []string{"claude", "cursor", "gemini", "codex"}
	var titleParts []string
	for _, id := range providerOrderTitle {
		usage, ok := usages[id]
		if !ok {
			continue
		}
		if len(usage.Windows) > 0 {
			w := usage.Windows[0]
			titleParts = append(titleParts, statusEmoji(w.PctUsed))
		}
	}
	if len(titleParts) == 0 {
		systray.SetTitle("⟳")
	} else {
		systray.SetTitle(strings.Join(titleParts, ""))
	}
	systray.SetTooltip("Clique para ver detalhes de uso")

	fillSlotsWithData(usages)
}

func fillSlotsNoData() {
	menuMu.Lock()
	if !slotsReady {
		menuMu.Unlock()
		return
	}
	menuMu.Unlock()

	for i := 0; i < 25; i++ {
		setSlot(i, "")
	}
	setSlot(0, "  Sem dados — aguardando atualização...")
}

func fillSlotsWithData(usages map[string]*providers.Usage) {
	menuMu.Lock()
	if !slotsReady {
		menuMu.Unlock()
		return
	}
	menuMu.Unlock()

	// Clear all slots first
	for i := 0; i < 25; i++ {
		setSlot(i, "  ")
	}

	slot := 0
	providerOrder := []string{"claude", "cursor", "gemini", "codex"}

	for _, id := range providerOrder {
		usage, ok := usages[id]
		if !ok {
			continue
		}

		// Provider header
		plan := usage.Plan
		if plan == "" {
			plan = "Pro"
		}
		setSlot(slot, fmt.Sprintf("  ── %s · %s ──────────────────", usage.Name, plan))
		slot++

		// Windows
		for _, w := range usage.Windows {
			if slot >= 22 {
				break
			}
			resetStr := ""
			if !w.ResetsAt.IsZero() {
				resetStr = "  · " + countdown(w.ResetsAt)
			}
			label := fmt.Sprintf("  %-8s  %s%s", w.Name, renderBar(w.PctUsed), resetStr)
			setSlot(slot, label)
			slot++
		}

		// Cost line for Claude
		if id == "claude" && (usage.TodayCostUSD > 0 || usage.Last30CostUSD > 0) {
			if slot < 22 {
				setSlot(slot, fmt.Sprintf("  💰 $%.2f hoje  ·  $%.2f / 30d", usage.TodayCostUSD, usage.Last30CostUSD))
				slot++
			}
		}

		// Blank separator
		if slot < 22 {
			setSlot(slot, "  ")
			slot++
		}
	}

	// Fill remaining slots with blanks
	for i := slot; i < 25; i++ {
		setSlot(i, "  ")
	}
}

func barChar(pct float64) string {
	switch {
	case pct >= 90:
		return "🔴"
	case pct >= 70:
		return "🟡"
	default:
		return "🟢"
	}
}
