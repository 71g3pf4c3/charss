// Package config loads charss configuration.
//
// Sources, in order of precedence:
//  1. CLI flags (--config, --urls, and future per-key flags)
//  2. config file (TOML by default: $XDG_CONFIG_HOME/charss/config.toml)
//  3. built-in defaults
//
// The urls file is a separate newsboat-style plain-text list of feeds
// ($XDG_CONFIG_HOME/charss/urls); it is not part of the viper config.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// Config is the resolved application configuration.
type Config struct {
	// Browser is the HTML rendering browser used to display articles.
	// The product requirement is chawan (https://chawan.net); the default
	// assumes it is on $PATH. Overridable via config key `browser`.
	Browser string

	// Chafa is the image-to-terminal converter binary used for image
	// previews. Overridable via config key `chafa`.
	Chafa string

	// ConfigFile is the path of the loaded config file (may be empty if
	// none exists).
	ConfigFile string

	// URLsFile is the path of the feed list file.
	URLsFile string
}

// Defaults for the config keys.
const (
	DefaultBrowser = "chawan"
	DefaultChafa   = "chafa"
)

// Paths returns the default config and urls file locations:
// $XDG_CONFIG_HOME/charss/{config.toml,urls} (falling back to
// ~/.config/charss/...).
func Paths() (configFile, urlsFile string) {
	dir, err := os.UserConfigDir() // honors XDG_CONFIG_HOME on unix
	if err != nil {
		dir = "."
	}
	base := filepath.Join(dir, "charss")
	return filepath.Join(base, "config.toml"), filepath.Join(base, "urls")
}

// Load resolves the configuration. configFlag/urlsFlag are the values of the
// --config/--urls CLI flags ("" = not given). A missing config file is not an
// error (defaults apply); a malformed one is.
func Load(configFlag, urlsFlag string) (*Config, error) {
	defConfig, defURLs := Paths()

	if configFlag != "" {
		if _, err := os.Stat(configFlag); err != nil {
			return nil, fmt.Errorf("config file: %w", err)
		}
	}

	v := viper.New()
	v.SetConfigFile(firstNonEmpty(configFlag, defConfig))
	v.SetConfigType("toml")
	v.SetDefault("browser", DefaultBrowser)
	v.SetDefault("chafa", DefaultChafa)

	if err := v.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if errors.As(err, &notFound) || os.IsNotExist(err) {
			// no config file — defaults are fine
		} else {
			return nil, fmt.Errorf("reading config: %w", err)
		}
	}

	cfg := &Config{
		Browser:    v.GetString("browser"),
		Chafa:      v.GetString("chafa"),
		ConfigFile: v.ConfigFileUsed(),
		URLsFile:   firstNonEmpty(urlsFlag, defURLs),
	}
	return cfg, nil
}

func firstNonEmpty(vals ...string) string {
	for _, s := range vals {
		if s != "" {
			return s
		}
	}
	return ""
}
