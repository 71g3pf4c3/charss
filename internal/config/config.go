// Package config loads charss configuration.
//
// charss uses newsboat-compatible config syntax: plain `name value`
// option lines, `bind-key`/`unbind-key`/`macro`/`color`/`include`
// directives, `#` comments, blank lines ignored. Point --config at an
// existing newsboat config and it parses as-is; unknown options,
// operations, contexts and attributes are kept but reported in
// Config.Warnings instead of being rejected.
//
// Resolution order: --config flag > $XDG_CONFIG_HOME/charss/config >
// built-in defaults. A missing config file is not an error (defaults
// apply); a malformed one is, with file and line. `include` paths are
// relative to the including file and may start with ~; cycles are an
// error.
//
// The legacy TOML config ($XDG_CONFIG_HOME/charss/config.toml, parsed
// with viper in earlier versions) was dropped: charss is pre-release and
// the newsboat config replaces it. A leftover config.toml produces a
// warning pointing at the new location.
//
// The urls file stays a separate newsboat-style plain-text file
// ($XDG_CONFIG_HOME/charss/urls); it is not part of the config.
package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// Config is the resolved application configuration.
type Config struct {
	// Browser is the HTML rendering browser used to display articles.
	// The product requirement is chawan (https://chawan.net); the default
	// assumes it is on $PATH. Config option `browser`.
	Browser string

	// Chafa is the image-to-terminal converter binary used for image
	// previews. Config option `chafa`.
	Chafa string

	// ConfigFile is the path of the loaded config file ("" if none
	// existed; includes are not listed separately).
	ConfigFile string

	// URLsFile is the path of the feed list file.
	URLsFile string

	// Options holds every plain `name value` option, defaults included;
	// a re-assigned option keeps its last value, like newsboat.
	Options map[string]string

	// Bindings is the resolved key map: default bindings plus the parsed
	// bind-key/unbind-key directives in file order.
	Bindings Bindings

	// Macros holds the parsed `macro <key> <operations...>` definitions.
	Macros map[string]Macro

	// Colors holds the parsed `color` rules, in file order.
	Colors []ColorRule

	// Warnings collects non-fatal issues: unknown options, operations,
	// contexts and color attributes, plus the legacy-config notice.
	Warnings []string
}

// Paths returns the default config and urls file locations:
// $XDG_CONFIG_HOME/charss/{config,urls} (falling back to
// ~/.config/charss/...).
func Paths() (configFile, urlsFile string) {
	dir, err := os.UserConfigDir() // honors XDG_CONFIG_HOME on unix
	if err != nil {
		dir = "."
	}
	base := filepath.Join(dir, "charss")
	return filepath.Join(base, "config"), filepath.Join(base, "urls")
}

// Load resolves the configuration. configFlag/urlsFlag are the values of
// the --config/--urls CLI flags ("" = not given). A missing config file
// is not an error (defaults apply); a malformed one is.
func Load(configFlag, urlsFlag string) (*Config, error) {
	defConfig, defURLs := Paths()

	if configFlag != "" {
		if _, err := os.Stat(configFlag); err != nil {
			return nil, fmt.Errorf("config file: %w", err)
		}
	}

	path := firstNonEmpty(configFlag, defConfig)
	if _, err := os.Stat(path); err != nil {
		path = "" // no config file: defaults, not an error
	}

	cfg := &Config{
		Options:  defaultOptionsCopy(),
		Bindings: defaultBindings(),
		Macros:   map[string]Macro{},
		URLsFile: firstNonEmpty(urlsFlag, defURLs),
	}

	if path != "" {
		res, err := parseConfig(path)
		if err != nil {
			return nil, err
		}
		for name, val := range res.options {
			cfg.Options[name] = val // file beats defaults; last one set wins
		}
		cfg.Bindings.apply(res.bindings)
		cfg.Macros = res.macros
		cfg.Colors = res.colors
		cfg.Warnings = res.warnings
		cfg.ConfigFile = path
	} else if legacy, ok := legacyConfigFile(); ok {
		cfg.Warnings = append(cfg.Warnings,
			fmt.Sprintf("ignoring legacy config %s: charss now reads the newsboat-style config at %s", legacy, defConfig))
	}

	cfg.Browser = cfg.Options["browser"]
	cfg.Chafa = cfg.Options["chafa"]
	return cfg, nil
}

// legacyConfigFile reports the pre-v2 TOML config path if one is left
// over, so Load can point the user at the new location.
func legacyConfigFile() (string, bool) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", false
	}
	p := filepath.Join(dir, "charss", "config.toml")
	if _, err := os.Stat(p); err == nil {
		return p, true
	}
	return "", false
}

func firstNonEmpty(vals ...string) string {
	for _, s := range vals {
		if s != "" {
			return s
		}
	}
	return ""
}
