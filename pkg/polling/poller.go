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

// renderBar returns a block progress bar, e.g. "████████░░ 82%"
func renderBar(pct float64) string {
	const total = 10
	filled := int(float64(total)*pct/100 + 0.5)
	if filled > total {
		filled = total
	}
	if filled < 0 {
		filled = 0
	}
	return strings.Repeat("█", filled) + strings.Repeat("░", total-filled) + fmt.Sprintf(" %2.0f%%", pct)
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

	systray.SetTitle("")
	if len(usages) == 0 {
		systray.SetTooltip("Tokalytics — aguardando dados...")
		fillSlotsNoData()
		return
	}

	systray.SetTooltip(buildTooltip(usages))
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
		setSlot(i, "  ")
	}
	setSlot(0, "  ⏳ Aguardando dados...")
}

// worstPct returns the highest PctUsed across all windows (used for header emoji).
func worstPct(windows []providers.RateWindow) float64 {
	worst := 0.0
	for _, w := range windows {
		if w.PctUsed > worst {
			worst = w.PctUsed
		}
	}
	return worst
}

// isInfoWindow detects stat-carrying rows that need no bar (PctUsed=0, full, no reset).
func isInfoWindow(w providers.RateWindow) bool {
	return w.PctUsed == 0 && w.PctLeft >= 99 && w.ResetsAt.IsZero()
}

// buildTooltip composes the hover tooltip from current usages.
func buildTooltip(usages map[string]*providers.Usage) string {
	var parts []string
	if u, ok := usages["claude"]; ok && u.TodayCostUSD > 0 {
		parts = append(parts, fmt.Sprintf("Claude $%.2f hoje", u.TodayCostUSD))
	}
	for _, id := range []string{"cursor", "gemini", "codex"} {
		u, ok := usages[id]
		if !ok {
			continue
		}
		w := worstPct(u.Windows)
		if w > 0 {
			parts = append(parts, fmt.Sprintf("%s %s %.0f%%", u.Name, barChar(w), w))
		}
	}
	if len(parts) == 0 {
		return "Tokalytics — sem dados de quota"
	}
	return "Tokalytics — " + strings.Join(parts, " · ")
}

func fillSlotsWithData(usages map[string]*providers.Usage) {
	menuMu.Lock()
	if !slotsReady {
		menuMu.Unlock()
		return
	}
	menuMu.Unlock()

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
		if slot >= 22 {
			break
		}

		plan := usage.Plan
		if plan == "" {
			plan = "Pro"
		}
		emoji := barChar(worstPct(usage.Windows))
		setSlot(slot, fmt.Sprintf("  %s  %s  ·  %s", emoji, usage.Name, plan))
		slot++

		for _, w := range usage.Windows {
			if slot >= 22 {
				break
			}
			if isInfoWindow(w) {
				setSlot(slot, fmt.Sprintf("  📊 %s", w.Name))
			} else {
				resetStr := ""
				if !w.ResetsAt.IsZero() {
					resetStr = "  ↺ " + countdown(w.ResetsAt)
				}
				setSlot(slot, fmt.Sprintf("  %s %-10s  %s%s", barChar(w.PctUsed), w.Name, renderBar(w.PctUsed), resetStr))
			}
			slot++
		}

		if id == "claude" && (usage.TodayCostUSD > 0 || usage.Last30CostUSD > 0) {
			if slot < 22 {
				setSlot(slot, fmt.Sprintf("  💰 $%.2f hoje  ·  $%.2f/30d", usage.TodayCostUSD, usage.Last30CostUSD))
				slot++
			}
		}

		if slot < 22 {
			setSlot(slot, "  ")
			slot++
		}
	}

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
