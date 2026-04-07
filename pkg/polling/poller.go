package polling

import (
	"fmt"
	"log"
	"time"

	"github.com/getlantern/systray"
	"github.com/writetoaniketparihar-collab/claude-spend/pkg/providers"
)

// Start inicia o loop de polling
func Start() {
	log.Println("Iniciando daemon de polling do CodexBar...")
	ticker := time.NewTicker(5 * time.Minute) // Default refresh cadence: 5m
	defer ticker.Stop()

	// Initial fetch
	updateTray()

	for range ticker.C {
		updateTray()
	}
}

func updateTray() {
	usages := providers.GetUsages()
	
	if len(usages) == 0 {
		systray.SetTitle("SpendBar")
		systray.SetTooltip("Nenhum provedor configurado.")
		return
	}

	// Simplificando: Mostrar o somatório de custos ou primeira quota disponível
	// CodexBar shows dynamic menu items for each provider. 
	// Em Go, nós atualizamos o tooltip com os detalhes.
	var tooltip string
	var totalCost float64

	for id, usage := range usages {
		totalCost += usage.Cost
		tooltip += fmt.Sprintf("[%s] Usado: %.2f / %.2f (Reseta em: %s)\n", 
			id, usage.Used, usage.TotalLimit, usage.SessionReset.Format(time.Kitchen))
		
		// Lógica de notificação (alerta > 90% uso)
		if usage.TotalLimit > 0 && (usage.Used/usage.TotalLimit) > 0.90 {
			// AQUI: disparar notificação nativa OS
			log.Printf("ALERTA: Quota de %s esgotando!", id)
		}
	}

	systray.SetTitle(fmt.Sprintf("$%.2f", totalCost))
	systray.SetTooltip(tooltip)
}
