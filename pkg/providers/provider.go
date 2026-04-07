package providers

import (
	"time"
)

// Provider represents an API or Local data source that provides usage metrics
type Provider interface {
	ID() string
	Name() string
	FetchUsage() (*Usage, error)
}

// Usage represents the quota usage of a provider
type Usage struct {
	TotalLimit   float64   // e.g., 100.0 (could be dollars, credits, or request count)
	Used         float64   // e.g., 45.5
	Remaining    float64   // e.g., 54.5
	SessionReset time.Time // When the current session/window resets
	WeeklyReset  time.Time // When the weekly window resets (if applicable)
	Cost         float64   // Estimated cost
}

// Registry holds all registered providers
var Registry []Provider

func Register(p Provider) {
	Registry = append(Registry, p)
}

// GetUsages fetches usage for all registered providers
func GetUsages() map[string]*Usage {
	results := make(map[string]*Usage)
	for _, p := range Registry {
		usage, err := p.FetchUsage()
		if err == nil && usage != nil {
			results[p.ID()] = usage
		}
	}
	return results
}
