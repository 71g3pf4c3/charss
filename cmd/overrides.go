// Tool overrides: the --browser/--chafa flags beat the CHARSS_BROWSER/
// CHARSS_CHAFA environment variables, which beat the config file's
// `browser`/`chafa` options. This is the one place the precedence is
// defined; every command goes through loadConfig.
package cmd

import (
	"os"

	"github.com/71g3pf4c3/charss/internal/config"
)

// loadConfig loads the config file (configFlag/urlsFlag) and applies the
// runtime tool overrides on top.
func loadConfig() (*config.Config, error) {
	cfg, err := config.Load(configFlag, urlsFlag)
	if err != nil {
		return nil, err
	}
	applyToolOverrides(cfg, browserFlag, chafaFlag)
	return cfg, nil
}

// applyToolOverrides resolves the effective article browser and image
// renderer binaries and writes them back into cfg, keeping the
// Options-map entries and the extracted fields in sync (both spellings
// are read downstream: cfg.Browser by the render driver, Options by the
// TUI for macro `set` and display).
func applyToolOverrides(cfg *config.Config, browserFlagVal, chafaFlagVal string) {
	if cfg.Options == nil {
		cfg.Options = map[string]string{}
	}
	if v := overrideOption(browserFlagVal, os.Getenv("CHARSS_BROWSER"), cfg.Browser); v != "" {
		cfg.Browser = v
		cfg.Options["browser"] = v
	}
	if v := overrideOption(chafaFlagVal, os.Getenv("CHARSS_CHAFA"), cfg.Chafa); v != "" {
		cfg.Chafa = v
		cfg.Options["chafa"] = v
	}
}

// overrideOption resolves one tool: flag beats env beats config.
func overrideOption(flagVal, envVal, cfgVal string) string {
	if flagVal != "" {
		return flagVal
	}
	if envVal != "" {
		return envVal
	}
	return cfgVal
}
