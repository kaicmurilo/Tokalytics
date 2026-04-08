// Package sysmon coleta uso de CPU e RAM do host e de processos ligados a Cursor, Claude Code e Gemini CLI.
package sysmon

import (
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/process"
)

// HostSnapshot é o agregado da máquina.
type HostSnapshot struct {
	CPUPercent float64 `json:"cpuPercent"`
	MemUsed    uint64  `json:"memUsedBytes"`
	MemTotal   uint64  `json:"memTotalBytes"`
	MemPercent float64 `json:"memPercent"`
}

// ModelSlice agrupa processos dentro de uma ferramenta por “modelo” inferido (ou rótulo genérico).
type ModelSlice struct {
	Key          string  `json:"key"`
	CPUPercent   float64 `json:"cpuPercent"`
	RSSBytes     uint64  `json:"rssBytes"`
	ProcessCount int     `json:"processCount"`
}

// ToolBucket é uma família de app (Cursor, Claude Code, Gemini).
type ToolBucket struct {
	ID           string       `json:"id"`
	Label        string       `json:"label"`
	CPUPercent   float64      `json:"cpuPercent"`
	RSSBytes     uint64       `json:"rssBytes"`
	ProcessCount int          `json:"processCount"`
	Models       []ModelSlice `json:"models"`
}

// Snapshot é a resposta de /api/system-live.
type Snapshot struct {
	Host  HostSnapshot `json:"host"`
	Tools []ToolBucket `json:"tools"`
	Note  string       `json:"note"`
}

var warmupOnce sync.Once

// Warmup dispara uma leitura inicial de CPUPercent nos processos (primeira chamada costuma retornar 0).
func Warmup() {
	warmupOnce.Do(func() {
		procs, err := process.Processes()
		if err != nil {
			return
		}
		for _, p := range procs {
			_, _ = p.CPUPercent()
		}
		time.Sleep(120 * time.Millisecond)
	})
}

func modelHintFromCmdline(cmd string) string {
	s := strings.ToLower(cmd)
	// Ordem: mais específico primeiro
	checks := []struct{ needle, label string }{
		{"claude-opus", "Claude Opus"},
		{"claude-sonnet", "Claude Sonnet"},
		{"claude-haiku", "Claude Haiku"},
		{"gpt-5", "GPT-5"},
		{"gpt-4", "GPT-4"},
		{"o3-", "o3"},
		{"gemini-2.5", "Gemini 2.5"},
		{"gemini-2", "Gemini 2.x"},
		{"gemini-1.5", "Gemini 1.5"},
		{"gemini-pro", "Gemini Pro"},
		{"gemini", "Gemini"},
	}
	for _, c := range checks {
		if strings.Contains(s, c.needle) {
			return c.label
		}
	}
	return ""
}

// isGeminiCLI detecta o binário do Gemini CLI: npm/nvm global costuma ser node .../bin/gemini (sem "google" no path).
func isGeminiCLI(name, cmdline string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	c := strings.ToLower(cmdline)
	base := strings.TrimSuffix(n, ".exe")
	if base == "gemini" {
		return true
	}
	if strings.Contains(c, "gemini-cli") || strings.Contains(c, "@google/gemini-cli") {
		return true
	}
	if strings.Contains(c, "/bin/gemini") || strings.Contains(c, "\\gemini.exe") {
		return true
	}
	// NVM / global: "node .../node/v22/.../bin/gemini"
	if (base == "node" || base == "nodejs") && strings.Contains(c, "bin/gemini") {
		return true
	}
	return false
}

// isClaudeCodeCLI cobre: npm/npx (node + @anthropic-ai/claude-code), Homebrew Cask (binário "claude" ou path .../claude-code/...), e shim node .../bin/claude.
func isClaudeCodeCLI(name, cmdline string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	c := strings.ToLower(cmdline)
	base := strings.TrimSuffix(n, ".exe")

	if strings.Contains(c, "claude-code") || strings.Contains(c, "@anthropic-ai/claude-code") {
		return true
	}
	if strings.Contains(c, "anthropic") && strings.Contains(c, "claude") {
		return true
	}
	// Homebrew Cask: .../Caskroom/claude-code/<ver>/claude
	if strings.Contains(c, "caskroom/claude-code") || strings.Contains(c, `caskroom\claude-code`) {
		return true
	}
	// Binário global (brew link → /opt/homebrew/bin/claude — sem "claude-code" no argv)
	if base == "claude" {
		return true
	}
	if (base == "node" || base == "nodejs" || base == "bun") && strings.Contains(c, "bin/claude") {
		return true
	}
	return false
}

func classifyProcess(name string, cmdline string) (toolID string) {
	n := strings.ToLower(strings.TrimSpace(name))
	c := strings.ToLower(cmdline)

	if strings.Contains(n, "cursor") || strings.Contains(c, "cursor.app") || strings.Contains(c, "\\cursor\\") {
		return "cursor"
	}
	if isGeminiCLI(name, cmdline) {
		return "gemini"
	}
	if isClaudeCodeCLI(name, cmdline) {
		return "claude"
	}
	if n == "node" || n == "node.exe" {
		if strings.Contains(c, "cursor") {
			return "cursor"
		}
	}
	return ""
}

// Collect mede host e processos classificados. Chame periodicamente; a primeira leitura de CPU por processo pode ser baixa.
func Collect() Snapshot {
	Warmup()

	note := "CPU e RAM são medidos no seu Mac/PC. O modelo de LLM só aparece quando o processo expõe esse nome na linha de comando; muitos processos aparecem como “Outros (ferramenta)”."
	snap := Snapshot{
		Note: note,
		Tools: []ToolBucket{
			{ID: "cursor", Label: "Cursor"},
			{ID: "claude", Label: "Claude Code"},
			{ID: "gemini", Label: "Gemini CLI"},
		},
	}

	if v, err := mem.VirtualMemory(); err == nil {
		snap.Host.MemTotal = v.Total
		snap.Host.MemUsed = v.Used
		if v.Total > 0 {
			snap.Host.MemPercent = float64(v.Used) / float64(v.Total) * 100
		}
	}

	if pct, err := cpu.Percent(280*time.Millisecond, false); err == nil && len(pct) > 0 {
		snap.Host.CPUPercent = pct[0]
	}

	toolModelMap := map[string]map[string]*ModelSlice{
		"cursor": {},
		"claude": {},
		"gemini": {},
	}

	procs, err := process.Processes()
	if err != nil {
		return snap
	}

	for _, p := range procs {
		name, err := p.Name()
		if err != nil {
			continue
		}
		cmdline := ""
		if cmd, err := p.Cmdline(); err == nil {
			cmdline = cmd
		}
		tool := classifyProcess(name, cmdline)
		if tool == "" {
			continue
		}

		cpuP, _ := p.CPUPercent()
		if cpuP < 0 {
			cpuP = 0
		}
		memInfo, err := p.MemoryInfo()
		var rss uint64
		if err == nil && memInfo != nil {
			rss = memInfo.RSS
		}

		hint := modelHintFromCmdline(cmdline)
		if hint == "" {
			hint = "Outros (ferramenta)"
		}

		mm := toolModelMap[tool]
		if mm[hint] == nil {
			mm[hint] = &ModelSlice{Key: hint}
		}
		m := mm[hint]
		m.CPUPercent += cpuP
		m.RSSBytes += rss
		m.ProcessCount++
	}

	for i := range snap.Tools {
		tid := snap.Tools[i].ID
		mm := toolModelMap[tid]
		var models []ModelSlice
		var sumCPU float64
		var sumRSS uint64
		var count int
		for _, m := range mm {
			models = append(models, *m)
			sumCPU += m.CPUPercent
			sumRSS += m.RSSBytes
			count += m.ProcessCount
		}
		sort.Slice(models, func(i, j int) bool { return models[i].RSSBytes > models[j].RSSBytes })
		snap.Tools[i].Models = models
		snap.Tools[i].CPUPercent = sumCPU
		snap.Tools[i].RSSBytes = sumRSS
		snap.Tools[i].ProcessCount = count
	}

	return snap
}
