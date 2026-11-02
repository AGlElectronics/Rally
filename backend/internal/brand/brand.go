// Package brand supplies the frontend brand package (product name, logo,
// colors, login copy). The source is pluggable so Rally can run standalone
// off a local file today and later pair with the central portal (`Den`,
// provisional name) without changing feature code.
//
// Scope: this scaffold ships a working standalone provider and a Den stub
// that satisfies the interface but returns a clear "not paired" error. The
// real Den pairing client lands with the Den work and is intentionally not
// built here.
package brand

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/AGlElectronics/rally/backend/internal/config"
)

// Brand is the public JSON shape returned by GET /api/v1/brand and consumed
// by the SPA on boot. Keep the fields small and additive — the frontend
// tolerates missing optional fields but will not re-render on structural
// changes mid-request.
type Brand struct {
	Revision         int               `json:"revision"`
	Source           string            `json:"source"` // "standalone" | "den"
	ProductName      string            `json:"productName"`
	OrganizationName string            `json:"organizationName,omitempty"`
	LoginSubtitle    string            `json:"loginSubtitle,omitempty"`
	LoginButtonLabel string            `json:"loginButtonLabel,omitempty"`
	LogoURL          string            `json:"logoUrl,omitempty"`
	ThemeColor       string            `json:"themeColor,omitempty"`
	Colors           map[string]string `json:"colors,omitempty"`
	CSSVariables     map[string]string `json:"cssVariables,omitempty"`
}

// Provider loads the current brand package. Implementations MUST be safe
// for concurrent reads and SHOULD cache aggressively — Get is called on
// every SPA boot.
type Provider interface {
	Get(ctx context.Context) (*Brand, error)
	Source() config.BrandSource
}

// New returns the Provider implementation that matches cfg.BrandSource.
// Validation of the source value happens in config.Load, so a non-nil cfg
// always produces a non-nil provider.
func New(cfg *config.Config) Provider {
	switch cfg.BrandSource {
	case config.BrandSourceDen:
		return &denProvider{baseURL: cfg.DenBaseURL, key: cfg.DenPairingKey}
	default:
		return &standaloneProvider{path: cfg.BrandLocalPath}
	}
}

// defaultBrand is returned when no local file is configured. It is
// intentionally generic — nothing hard-codes a specific convention here.
func defaultBrand() *Brand {
	return &Brand{
		Revision:         1,
		Source:           string(config.BrandSourceStandalone),
		ProductName:      "Rally",
		OrganizationName: "",
		LoginSubtitle:    "Events & convention management",
		LoginButtonLabel: "Continue with your organisation login",
		LogoURL:          "",
		ThemeColor:       "#18181b",
		Colors: map[string]string{
			"primary": "#4f46e5",
			"carbon":  "#18181b",
			"shadow":  "#241e20",
		},
	}
}

// -----------------------------------------------------------------------------
// Standalone provider — reads a local JSON file (or returns a default).
// -----------------------------------------------------------------------------

type standaloneProvider struct {
	path string

	mu     sync.RWMutex
	cached *Brand
	loaded time.Time
}

func (p *standaloneProvider) Source() config.BrandSource { return config.BrandSourceStandalone }

func (p *standaloneProvider) Get(_ context.Context) (*Brand, error) {
	// Cheap in-process cache: the file is read at most every 30 seconds.
	// For a scaffold this is fine; the full implementation can watch the
	// file for changes if operators want hot reloads.
	p.mu.RLock()
	if p.cached != nil && time.Since(p.loaded) < 30*time.Second {
		b := *p.cached
		p.mu.RUnlock()
		return &b, nil
	}
	p.mu.RUnlock()

	b := defaultBrand()
	if p.path != "" {
		data, err := os.ReadFile(p.path)
		if err != nil {
			return nil, fmt.Errorf("read brand file %q: %w", p.path, err)
		}
		if err := json.Unmarshal(data, b); err != nil {
			return nil, fmt.Errorf("parse brand file %q: %w", p.path, err)
		}
		b.Source = string(config.BrandSourceStandalone)
	}

	p.mu.Lock()
	p.cached = b
	p.loaded = time.Now()
	p.mu.Unlock()
	return b, nil
}

// -----------------------------------------------------------------------------
// Den pairing stub — placeholder for the future pairing client.
// -----------------------------------------------------------------------------

type denProvider struct {
	baseURL string
	key     string
}

func (p *denProvider) Source() config.BrandSource { return config.BrandSourceDen }

// Get returns a clear "not yet implemented" error so operators who enable
// BRAND_SOURCE=den before the pairing client lands see an explicit failure
// rather than silent fallback. The baseURL/key are kept so the surface
// is already shaped correctly for the real implementation.
func (p *denProvider) Get(_ context.Context) (*Brand, error) {
	if p.baseURL == "" {
		return nil, errors.New("den brand provider: DEN_BASE_URL not configured")
	}
	// TODO(rally/den): swap for a real signed HTTP call to Den's brand
	// endpoint. The pairing-key flow is intentionally left for the Den
	// implementation task.
	return nil, errors.New("den brand provider is not yet implemented in this scaffold")
}
