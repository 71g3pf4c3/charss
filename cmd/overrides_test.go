package cmd

import (
	"testing"

	"github.com/71g3pf4c3/charss/internal/config"
)

func TestApplyToolOverrides(t *testing.T) {
	tests := []struct {
		name                   string
		browserFlag, chafaFlag string
		browserEnv, chafaEnv   string
		cfgBrowser, cfgChafa   string
		wantBrowser, wantChafa string
	}{
		{
			name:        "flag beats env beats config",
			browserFlag: "flag-browser", chafaFlag: "flag-chafa",
			browserEnv: "env-browser", chafaEnv: "env-chafa",
			cfgBrowser: "cfg-browser", cfgChafa: "cfg-chafa",
			wantBrowser: "flag-browser", wantChafa: "flag-chafa",
		},
		{
			name:       "env beats config",
			browserEnv: "env-browser", chafaEnv: "env-chafa",
			cfgBrowser: "cfg-browser", cfgChafa: "cfg-chafa",
			wantBrowser: "env-browser", wantChafa: "env-chafa",
		},
		{
			name:       "config wins when nothing overrides",
			cfgBrowser: "cfg-browser", cfgChafa: "cfg-chafa",
			wantBrowser: "cfg-browser", wantChafa: "cfg-chafa",
		},
		{
			name:        "levels resolve independently",
			browserFlag: "flag-browser",
			chafaEnv:    "env-chafa",
			cfgBrowser:  "cfg-browser", cfgChafa: "cfg-chafa",
			wantBrowser: "flag-browser", wantChafa: "env-chafa",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("CHARSS_BROWSER", tt.browserEnv)
			t.Setenv("CHARSS_CHAFA", tt.chafaEnv)

			cfg := &config.Config{Browser: tt.cfgBrowser, Chafa: tt.cfgChafa}
			cfg.Options = map[string]string{
				"browser": tt.cfgBrowser,
				"chafa":   tt.cfgChafa,
			}

			applyToolOverrides(cfg, tt.browserFlag, tt.chafaFlag)

			if cfg.Browser != tt.wantBrowser {
				t.Errorf("Browser = %q, want %q", cfg.Browser, tt.wantBrowser)
			}
			if cfg.Chafa != tt.wantChafa {
				t.Errorf("Chafa = %q, want %q", cfg.Chafa, tt.wantChafa)
			}
			// The Options-map alias must stay in sync — macro `set` and
			// the TUI read both spellings.
			if cfg.Options["browser"] != tt.wantBrowser {
				t.Errorf("Options[browser] = %q, want %q", cfg.Options["browser"], tt.wantBrowser)
			}
			if cfg.Options["chafa"] != tt.wantChafa {
				t.Errorf("Options[chafa] = %q, want %q", cfg.Options["chafa"], tt.wantChafa)
			}
		})
	}
}

func TestApplyToolOverridesNilOptions(t *testing.T) {
	cfg := &config.Config{Browser: "cha"}
	applyToolOverrides(cfg, "other", "")
	if cfg.Browser != "other" {
		t.Errorf("Browser = %q, want %q", cfg.Browser, "other")
	}
	if cfg.Options["browser"] != "other" {
		t.Errorf("Options[browser] = %q, want %q", cfg.Options["browser"], "other")
	}
}
