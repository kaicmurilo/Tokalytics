package providers

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
)

// Estruturas correspondentes ao Node.js parser
type Query struct {
	UserPrompt          string   `json:"userPrompt"`
	Model               string   `json:"model"`
	InputTokens         int      `json:"inputTokens"`
	OutputTokens        int      `json:"outputTokens"`
	CacheCreationTokens int      `json:"cacheCreationTokens"`
	CacheReadTokens     int      `json:"cacheReadTokens"`
	TotalTokens         int      `json:"totalTokens"`
	Cost                float64  `json:"cost"`
	Tools               []string `json:"tools"`
}

type Session struct {
	SessionID   string  `json:"sessionId"`
	Project     string  `json:"project"`
	Date        string  `json:"date"`
	FirstPrompt string  `json:"firstPrompt"`
	Model       string  `json:"model"`
	QueryCount  int     `json:"queryCount"`
	Queries     []Query `json:"queries"`
	TotalTokens int     `json:"totalTokens"`
	Cost        float64 `json:"cost"`
}

// LocalParser lida com a leitura de arquivos .jsonl
func ParseJSONLFile(filePath string) ([]map[string]interface{}, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var lines []map[string]interface{}
	scanner := bufio.NewScanner(file)
	// Claude code logs can be large, use a bigger buffer if needed
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024*10)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var entry map[string]interface{}
		if err := json.Unmarshal(line, &entry); err == nil {
			lines = append(lines, entry)
		}
	}
	return lines, scanner.Err()
}

func getClaudeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude")
}

// ParseClaudeSessions varre ~/.claude e retorna as sessões (simplificado para o MVP)
func ParseClaudeSessions() []Session {
	dir := getClaudeDir()
	if dir == "" {
		return nil
	}

	var sessions []Session
	projectsDir := filepath.Join(dir, "projects")
	
	// A leitura recursiva simplificada aqui
	// (A implementação completa vai varrer projectsDir como no Node.js)
	
	return sessions
}
