package providers

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	quotaEndpoint          = "https://cloudcode-pa.googleapis.com/v1internal:retrieveUserQuota"
	loadCodeAssistEndpoint = "https://cloudcode-pa.googleapis.com/v1internal:loadCodeAssist"
	tokenRefreshEndpoint   = "https://oauth2.googleapis.com/token"
	geminiOAuthCredsPath   = ".gemini/oauth_creds.json"
)

type OAuthCreds struct {
	AccessToken  string  `json:"access_token"`
	IDToken      string  `json:"id_token"`
	RefreshToken string  `json:"refresh_token"`
	ExpiryDate   float64 `json:"expiry_date"` // In milliseconds
}

type QuotaBucket struct {
	RemainingFraction float64 `json:"remainingFraction"`
	ResetTime         string  `json:"resetTime"`
	ModelID           string  `json:"modelId"`
}

type QuotaResponse struct {
	Buckets []QuotaBucket `json:"buckets"`
}

type CodeAssistResponse struct {
	CloudaicompanionProject any `json:"cloudaicompanionProject"`
	CurrentTier             struct {
		ID string `json:"id"`
	} `json:"currentTier"`
}

// discoverGeminiSecrets attempts to find the OAuth client ID and secret from the installed gemini-cli
func discoverGeminiSecrets() (string, string, error) {
	geminiPath, err := exec.LookPath("gemini")
	if err != nil {
		return "", "", fmt.Errorf("gemini binary not found: %v", err)
	}

	realPath, err := filepath.EvalSymlinks(geminiPath)
	if err != nil {
		realPath = geminiPath
	}

	// The bundle files are usually in ../lib/node_modules/@google/gemini-cli/bundle/
	// or similar depending on the installation method (npm, nvm, homebrew).
	// We search for the pattern in all .js files in the bundle directory.
	baseDir := filepath.Dir(filepath.Dir(realPath))
	bundleDir := filepath.Join(baseDir, "lib", "node_modules", "@google", "gemini-cli", "bundle")

	// Fallback for some installations where the structure is different
	if _, err := os.Stat(bundleDir); os.IsNotExist(err) {
		bundleDir = filepath.Join(filepath.Dir(realPath), "bundle")
	}

	idRegex := regexp.MustCompile(`OAUTH_CLIENT_ID\s*=\s*["']([^"']+)["']`)
	secretRegex := regexp.MustCompile(`OAUTH_CLIENT_SECRET\s*=\s*["']([^"']+)["']`)

	var clientID, clientSecret string

	err = filepath.Walk(bundleDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".js") {
			return nil
		}
		if clientID != "" && clientSecret != "" {
			return filepath.SkipDir
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		if clientID == "" {
			if m := idRegex.FindSubmatch(content); m != nil {
				clientID = string(m[1])
			}
		}
		if clientSecret == "" {
			if m := secretRegex.FindSubmatch(content); m != nil {
				clientSecret = string(m[1])
			}
		}
		return nil
	})

	if clientID == "" || clientSecret == "" {
		return "", "", fmt.Errorf("could not discover gemini secrets from bundle at %s", bundleDir)
	}

	return clientID, clientSecret, nil
}

func getGeminiAccountInfo() (string, string, error) {
	home, _ := os.UserHomeDir()
	path := filepath.Join(home, geminiOAuthCredsPath)

	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", err
	}

	var creds OAuthCreds
	if err := json.Unmarshal(data, &creds); err != nil {
		return "", "", err
	}

	email := extractEmailFromToken(creds.IDToken)

	// Check if expired
	expiry := time.Unix(int64(creds.ExpiryDate/1000), 0)
	accessToken := creds.AccessToken
	if time.Now().After(expiry) {
		// Needs refresh
		var err error
		accessToken, err = refreshGeminiToken(creds.RefreshToken, path)
		if err != nil {
			return email, "", err
		}
	}

	return email, accessToken, nil
}

func extractEmailFromToken(idToken string) string {
	parts := strings.Split(idToken, ".")
	if len(parts) < 2 {
		return ""
	}

	payload := parts[1]
	// Base64URL decode
	payload = strings.ReplaceAll(payload, "-", "+")
	payload = strings.ReplaceAll(payload, "_", "/")
	switch len(payload) % 4 {
	case 2:
		payload += "=="
	case 3:
		payload += "="
	}

	decoded, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return ""
	}

	var claims map[string]any
	if err := json.Unmarshal(decoded, &claims); err != nil {
		return ""
	}

	email, _ := claims["email"].(string)
	return email
}

func refreshGeminiToken(refreshToken, path string) (string, error) {
	clientID, clientSecret, err := discoverGeminiSecrets()
	if err != nil {
		return "", err
	}

	body := fmt.Sprintf("client_id=%s&client_secret=%s&refresh_token=%s&grant_type=refresh_token",
		clientID, clientSecret, refreshToken)

	resp, err := http.Post(tokenRefreshEndpoint, "application/x-www-form-urlencoded", bytes.NewBufferString(body))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("failed to refresh token: %s", resp.Status)
	}

	var newCreds map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&newCreds); err != nil {
		return "", err
	}

	accessToken, _ := newCreds["access_token"].(string)
	expiresIn, _ := newCreds["expires_in"].(float64)

	// Update the file
	data, _ := os.ReadFile(path)
	var oldCreds map[string]any
	json.Unmarshal(data, &oldCreds)
	oldCreds["access_token"] = accessToken
	oldCreds["expiry_date"] = (float64(time.Now().Unix()) + expiresIn) * 1000

	updatedData, _ := json.MarshalIndent(oldCreds, "", "  ")
	os.WriteFile(path, updatedData, 0600)

	return accessToken, nil
}

func getGeminiProjectId(token string) string {
	payload := `{"metadata":{"ideType":"GEMINI_CLI","pluginType":"GEMINI"}}`
	req, _ := http.NewRequest("POST", loadCodeAssistEndpoint, bytes.NewBufferString(payload))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	var ca CodeAssistResponse
	if err := json.NewDecoder(resp.Body).Decode(&ca); err != nil {
		return ""
	}

	// The project ID can be a string or a map
	switch v := ca.CloudaicompanionProject.(type) {
	case string:
		return v
	case map[string]any:
		if id, ok := v["projectId"].(string); ok {
			return id
		}
		if id, ok := v["id"].(string); ok {
			return id
		}
	}

	return ""
}

func fetchGeminiAPIUsage() ([]RateWindow, string, string, error) {
	email, token, err := getGeminiAccountInfo()
	if err != nil {
		return nil, "", email, err
	}

	projectID := getGeminiProjectId(token)
	payload := "{}"
	if projectID != "" {
		payload = fmt.Sprintf(`{"project": "%s"}`, projectID)
	}

	req, _ := http.NewRequest("POST", quotaEndpoint, bytes.NewBufferString(payload))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", email, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, "", email, fmt.Errorf("quota API error: %s", resp.Status)
	}

	var qr QuotaResponse
	if err := json.NewDecoder(resp.Body).Decode(&qr); err != nil {
		return nil, "", email, err
	}

	// Group quotas by model (taking the lowest fraction per model)
	type modelInfo struct {
		fraction  float64
		resetTime string
	}
	models := map[string]modelInfo{}
	for _, b := range qr.Buckets {
		if b.ModelID == "" {
			continue
		}
		if cur, ok := models[b.ModelID]; !ok || b.RemainingFraction < cur.fraction {
			models[b.ModelID] = modelInfo{b.RemainingFraction, b.ResetTime}
		}
	}

	var windows []RateWindow
	// Order: Pro, Flash, Flash Lite
	order := []struct {
		name   string
		filter func(string) bool
	}{
		{"Pro", func(id string) bool { return strings.Contains(id, "pro") }},
		{"Flash", func(id string) bool { return strings.Contains(id, "flash") && !strings.Contains(id, "flash-lite") }},
		{"Flash Lite", func(id string) bool { return strings.Contains(id, "flash-lite") }},
	}

	for _, o := range order {
		var minFraction = 1.0
		var resetTime string
		found := false
		for id, info := range models {
			if o.filter(id) {
				found = true
				if info.fraction < minFraction {
					minFraction = info.fraction
					resetTime = info.resetTime
				}
			}
		}

		if found {
			w := RateWindow{
				Name:    o.name,
				PctUsed: (1 - minFraction) * 100,
				PctLeft: minFraction * 100,
			}
			if resetTime != "" {
				w.ResetsAt, _ = time.Parse(time.RFC3339, resetTime)
			}
			windows = append(windows, w)
		}
	}

	return windows, "Paid", email, nil
}
