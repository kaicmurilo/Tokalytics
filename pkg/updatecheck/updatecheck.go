package updatecheck

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/mod/semver"
)

const githubLatestURL = "https://api.github.com/repos/kaicmurilo/Tokalytics/releases/latest"

// Result é a resposta JSON para o dashboard.
type Result struct {
	Current         string `json:"current"`
	Latest          string `json:"latest"`
	UpdateAvailable bool   `json:"updateAvailable"`
	InstallCommand  string `json:"installCommand"`
	ReleaseURL      string `json:"releaseUrl,omitempty"`
}

var (
	mu           sync.Mutex
	cached       *Result
	cachedAt     time.Time
	cacheTTL     = time.Hour
	lastFailure  time.Time
	failCooldown = 5 * time.Minute
)

func semverKey(v string) string {
	v = strings.TrimSpace(strings.TrimPrefix(v, "v"))
	if v == "" || strings.EqualFold(v, "dev") {
		return ""
	}
	full := "v" + v
	if semver.IsValid(full) {
		return full
	}
	return ""
}

// Check compara a versão do binário com a última release no GitHub (com cache).
func Check(current string) *Result {
	out := &Result{
		Current:        current,
		InstallCommand: "npm install -g tokalytics",
	}

	curKey := semverKey(current)
	if curKey == "" {
		return out
	}
	out.Current = strings.TrimPrefix(curKey, "v")

	mu.Lock()
	defer mu.Unlock()

	if cached != nil && time.Since(cachedAt) < cacheTTL {
		return withLocalCompare(cloneResult(cached), curKey)
	}
	if cached == nil && time.Since(lastFailure) < failCooldown {
		return out
	}

	client := &http.Client{Timeout: 14 * time.Second}
	req, err := http.NewRequest(http.MethodGet, githubLatestURL, nil)
	if err != nil {
		lastFailure = time.Now()
		if cached != nil {
			return withLocalCompare(cloneResult(cached), curKey)
		}
		return out
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "Tokalytics-update-check")
	if t := os.Getenv("GITHUB_TOKEN"); t != "" {
		req.Header.Set("Authorization", "Bearer "+t)
	}

	resp, err := client.Do(req)
	if err != nil {
		lastFailure = time.Now()
		if cached != nil {
			return withLocalCompare(cloneResult(cached), curKey)
		}
		return out
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 512))
		lastFailure = time.Now()
		if cached != nil {
			return withLocalCompare(cloneResult(cached), curKey)
		}
		return out
	}

	var rel struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		lastFailure = time.Now()
		if cached != nil {
			return withLocalCompare(cloneResult(cached), curKey)
		}
		return out
	}

	latest := strings.TrimSpace(strings.TrimPrefix(rel.TagName, "v"))
	latestKey := semverKey(latest)
	if latestKey == "" {
		out.Latest = latest
		return out
	}

	r := &Result{
		Current:        strings.TrimPrefix(curKey, "v"),
		Latest:         strings.TrimPrefix(latestKey, "v"),
		InstallCommand: "npm install -g tokalytics",
		ReleaseURL:     rel.HTMLURL,
	}
	if semver.Compare(latestKey, curKey) > 0 {
		r.UpdateAvailable = true
	}

	cached = r
	cachedAt = time.Now()
	lastFailure = time.Time{}
	return withLocalCompare(cloneResult(r), curKey)
}

func withLocalCompare(r *Result, curKey string) *Result {
	if r == nil {
		return nil
	}
	x := *r
	x.Current = strings.TrimPrefix(curKey, "v")
	if lk := semverKey(x.Latest); lk != "" {
		x.UpdateAvailable = semver.Compare(lk, curKey) > 0
	} else {
		x.UpdateAvailable = false
	}
	return &x
}

func cloneResult(r *Result) *Result {
	if r == nil {
		return nil
	}
	cp := *r
	return &cp
}
