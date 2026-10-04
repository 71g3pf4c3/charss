// Package cmd defines the charss command-line interface (cobra).
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/71g3pf4c3/charss/internal/config"
)

var (
	configFlag string
	urlsFlag   string
)

// rootCmd runs the interactive TUI (the default, like newsboat's bare `newsboat`).
var rootCmd = &cobra.Command{
	Use:   "charss",
	Short: "Terminal RSS reader (newsboat alternative) with chawan/chafa/sixel support",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(configFlag, urlsFlag)
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
}

// Execute runs the root command; it is the process entry point.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
