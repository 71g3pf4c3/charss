// Package cmd defines the charss command-line interface (cobra).
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	configFlag  string
	urlsFlag    string
	browserFlag string
	chafaFlag   string
)

// rootCmd runs the interactive TUI (the default, like newsboat's bare `newsboat`).
var rootCmd = &cobra.Command{
	Use:   "charss",
	Short: "Terminal RSS reader (newsboat alternative) with chawan/chafa/sixel support",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := loadConfig()
		if err != nil {
			return err
		}
		return runTUI(cfg)
	},
}

func init() {
	rootCmd.PersistentFlags().StringVar(&configFlag, "config", "",
		"config file (default $XDG_CONFIG_HOME/charss/config)")
	rootCmd.PersistentFlags().StringVar(&urlsFlag, "urls", "",
		"feed list file (default $XDG_CONFIG_HOME/charss/urls)")
	rootCmd.PersistentFlags().StringVar(&browserFlag, "browser", "",
		"article browser/pager binary (overrides $CHARSS_BROWSER and the browser config option)")
	rootCmd.PersistentFlags().StringVar(&chafaFlag, "chafa", "",
		"image renderer binary (overrides $CHARSS_CHAFA and the chafa config option)")
}

// Execute runs the root command; it is the process entry point.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
