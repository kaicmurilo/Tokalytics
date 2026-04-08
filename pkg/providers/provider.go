package providers

import (
	"log"
	"sync"
	"time"
)

// Provider represents an API or Local data source that provides usage metrics
type Provider interface {
	ID() string
	Name() string
	FetchUsage() (*Usage, error)
}

// RateWindow holds data for a single usage window (session, weekly, auto, API, etc.)
type RateWindow struct {
	Name     string    `json:"name"`
	PctUsed  float64   `json:"pctUsed"`
	PctLeft  float64   `json:"pctLeft"`
	Deficit  float64   `json:"deficit,omitempty"` // % over limit
	ResetsAt time.Time `json:"resetsAt,omitempty"`
	RunsOut  time.Time `json:"runsOut,omitempty"` // projected exhaustion
}

// Usage represents the quota usage of a provider
type Usage struct {
	ProviderID string `json:"providerID"`
	Name       string `json:"name,omitempty"`
	Email      string `json:"email,omitempty"`
	Plan       string `json:"plan,omitempty"`
	UpdatedAt  string `json:"updatedAt,omitempty"`

	// Primary window (backwards compat)
	TotalLimit   float64   `json:"totalLimit"`
	Used         float64   `json:"used"`
	Remaining    float64   `json:"remaining"`
	SessionReset time.Time `json:"sessionReset"`
	WeeklyReset  time.Time `json:"weeklyReset"`
	Cost         float64   `json:"cost"`
	Details      string    `json:"details,omitempty"`

	// Rich windows (Session, Weekly, Total, Auto, API…)
	Windows []RateWindow `json:"windows,omitempty"`

	// Cost tracking from local files
	TodayCostUSD    float64 `json:"todayCostUSD,omitempty"`
	TodayTokens     int     `json:"todayTokens,omitempty"`
	Last30CostUSD   float64 `json:"last30CostUSD,omitempty"`
	Last30Tokens    int     `json:"last30Tokens,omitempty"`
}

// Registry holds all registered providers
var Registry []Provider

func Register(p Provider) {
	Registry = append(Registry, p)
}

// cache holds the last successful fetch result per provider
var (
	cacheMu      sync.Mutex
	usageCache   = map[string]*Usage{}
	cacheTime    = map[string]time.Time{}
	cacheTTL     = 4 * time.Minute
)

// RefreshUsages forces a fresh fetch for all providers and updates cache
func RefreshUsages() map[string]*Usage {
	results := make(map[string]*Usage)
	for _, p := range Registry {
		usage, err := p.FetchUsage()
		if err != nil {
			log.Printf("[%s] FetchUsage error: %v", p.ID(), err)
			// Return stale cache on error
			cacheMu.Lock()
			if cached, ok := usageCache[p.ID()]; ok {
				results[p.ID()] = cached
			}
			cacheMu.Unlock()
			continue
		}
		if usage != nil {
			cacheMu.Lock()
			usageCache[p.ID()] = usage
			cacheTime[p.ID()] = time.Now()
			cacheMu.Unlock()
			results[p.ID()] = usage
		}
	}
	return results
}

// GetUsages returns cached usage data, refreshing only stale entries
func GetUsages() map[string]*Usage {
	results := make(map[string]*Usage)
	now := time.Now()

	for _, p := range Registry {
		cacheMu.Lock()
		cached, hasCached := usageCache[p.ID()]
		lastFetch := cacheTime[p.ID()]
		cacheMu.Unlock()

		if hasCached && now.Sub(lastFetch) < cacheTTL {
			results[p.ID()] = cached
			continue
		}

		// Cache stale or missing — fetch fresh
		usage, err := p.FetchUsage()
		if err != nil {
			log.Printf("[%s] FetchUsage error: %v", p.ID(), err)
			if hasCached {
				results[p.ID()] = cached // serve stale on error
			}
			continue
		}
		if usage != nil {
			cacheMu.Lock()
			usageCache[p.ID()] = usage
			cacheTime[p.ID()] = now
			cacheMu.Unlock()
			results[p.ID()] = usage
		}
	}
	return results
}
