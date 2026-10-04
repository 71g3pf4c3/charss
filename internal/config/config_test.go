package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// isolateXDG points XDG_CONFIG_HOME at a fresh temp dir so Load cannot see
// the developer's real config.
func isolateXDG(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	return dir
}

func writeConfig(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadDefaultsWhenNothingExists(t *testing.T) {
	isolateXDG(t)
	cfg, err := Load("", "")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Browser != DefaultBrowser || cfg.Chafa != DefaultChafa {
		t.Errorf("defaults not applied: browser=%q chafa=%q", cfg.Browser, cfg.Chafa)
	}
	if cfg.ConfigFile != "" {
		t.Errorf("ConfigFile = %q, want \"\" when no config exists", cfg.ConfigFile)
	}
	if len(cfg.Warnings) != 0 {
		t.Errorf("unexpected warnings: %v", cfg.Warnings)
	}
	// Defaults are visible through the option map too.
	if cfg.Options["browser"] != DefaultBrowser {
		t.Errorf(`Options["browser"] = %q`, cfg.Options["browser"])
	}
	// Default bindings survive.
	if got := cfg.Bindings.Lookup("feedlist", "ENTER"); got != OpOpen {
		t.Errorf(`Lookup(feedlist, ENTER) = %q`, got)
	}
}

func TestLoadXDGConfig(t *testing.T) {
	dir := isolateXDG(t)
	writeConfig(t, filepath.Join(dir, "charss", "config"), `
browser xdg-browser
bind-key g goto-url
macro m quit
color list red blue
max-items 42
`)
	cfg, err := Load("", "")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Browser != "xdg-browser" {
		t.Errorf("Browser = %q", cfg.Browser)
	}
	if cfg.ConfigFile != filepath.Join(dir, "charss", "config") {
		t.Errorf("ConfigFile = %q", cfg.ConfigFile)
	}
	if got := cfg.Bindings.Lookup("feedlist", "g"); got != OpGotoURL {
		t.Errorf(`Lookup(feedlist, g) = %q`, got)
	}
	if _, ok := cfg.Macros["m"]; !ok {
		t.Error("macro m missing")
	}
	if len(cfg.Colors) != 1 || cfg.Colors[0].Fg != "red" {
		t.Errorf("colors = %+v", cfg.Colors)
	}
	if cfg.Options["max-items"] != "42" {
		t.Errorf(`Options["max-items"] = %q`, cfg.Options["max-items"])
	}
	if !hasWarning(cfg.Warnings, `unknown option "max-items"`) {
		t.Errorf("missing unknown-option warning: %v", cfg.Warnings)
	}
}

func TestLoadFlagBeatsXDG(t *testing.T) {
	dir := isolateXDG(t)
	writeConfig(t, filepath.Join(dir, "charss", "config"), "browser xdg\n")

	flagDir := t.TempDir()
	flagPath := filepath.Join(flagDir, "my-config")
	writeConfig(t, flagPath, "browser from-flag\n")

	cfg, err := Load(flagPath, "")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Browser != "from-flag" {
		t.Errorf("Browser = %q, want from-flag", cfg.Browser)
	}
	if cfg.ConfigFile != flagPath {
		t.Errorf("ConfigFile = %q, want %q", cfg.ConfigFile, flagPath)
	}
}

func TestLoadFlagMissingFileIsError(t *testing.T) {
	isolateXDG(t)
	if _, err := Load(filepath.Join(t.TempDir(), "nope"), ""); err == nil {
		t.Fatal("missing --config file must be an error")
	}
}

func TestLoadMalformedIsErrorWithLine(t *testing.T) {
	dir := isolateXDG(t)
	writeConfig(t, filepath.Join(dir, "charss", "config"), "browser ok\n\nbind-key broken\n")
	_, err := Load("", "")
	if err == nil {
		t.Fatal("malformed config must be an error")
	}
	if !strings.Contains(err.Error(), ":3:") {
		t.Errorf("error lacks line number: %s", err)
	}
}

func TestLoadLegacyTomlWarns(t *testing.T) {
	dir := isolateXDG(t)
	writeConfig(t, filepath.Join(dir, "charss", "config.toml"), "browser = \"legacy\"\n")

	cfg, err := Load("", "")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Browser != DefaultBrowser {
		t.Errorf("legacy toml must not be parsed, Browser = %q", cfg.Browser)
	}
	if !hasWarning(cfg.Warnings, "legacy") || !hasWarning(cfg.Warnings, "config.toml") {
		t.Errorf("missing legacy warning: %v", cfg.Warnings)
	}
}

func TestLoadURLsFile(t *testing.T) {
	dir := isolateXDG(t)
	_, defURLs := Paths()

	cfg, err := Load("", "")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.URLsFile != defURLs {
		t.Errorf("URLsFile = %q, want default %q", cfg.URLsFile, defURLs)
	}
	if filepath.Dir(cfg.URLsFile) != filepath.Join(dir, "charss") {
		t.Errorf("URLsFile in unexpected dir: %q", cfg.URLsFile)
	}

	cfg, err = Load("", "/custom/urls")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.URLsFile != "/custom/urls" {
		t.Errorf("URLsFile = %q, want /custom/urls", cfg.URLsFile)
	}
}

func TestLoadOptionLastWinsAndUnknownKept(t *testing.T) {
	dir := isolateXDG(t)
	writeConfig(t, filepath.Join(dir, "charss", "config"), "browser one\nbrowser two\nwhatever keeps-value\n")
	cfg, err := Load("", "")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Browser != "two" {
		t.Errorf("Browser = %q, want two", cfg.Browser)
	}
	if cfg.Options["whatever"] != "keeps-value" {
		t.Errorf(`Options["whatever"] = %q`, cfg.Options["whatever"])
	}
}

func TestLoadUnbindRemovesDefault(t *testing.T) {
	dir := isolateXDG(t)
	writeConfig(t, filepath.Join(dir, "charss", "config"), "unbind-key r all\n")
	cfg, err := Load("", "")
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Bindings.Lookup("feedlist", "r"); got != "" {
		t.Errorf(`default r not unbound: %q`, got)
	}
	if got := cfg.Bindings.Lookup("feedlist", "ENTER"); got != OpOpen {
		t.Errorf(`ENTER lost: %q`, got)
	}
}
