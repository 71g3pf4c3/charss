package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/71g3pf4c3/charss/internal/config"
	"github.com/71g3pf4c3/charss/internal/image"
)

// previewCmd E2E-completes the image pipeline: detect the terminal's
// graphics protocol (sixel first), fetch the image, convert it with
// chafa and write the raw escape-sequence stream to stdout.
var previewCmd = &cobra.Command{
	Use:   "preview <image-url>",
	Short: "Fetch an image and render it in the terminal (chafa/sixel)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := loadConfig()
		if err != nil {
			return err
		}
		return runPreview(cfg, args[0])
	},
}

func init() {
	rootCmd.AddCommand(previewCmd)
}

func runPreview(cfg *config.Config, imageURL string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	format := image.Detect(nil) // TERM-based; sixel when available

	data, err := image.NewFetcher(image.WithTimeout(30*time.Second)).Fetch(ctx, imageURL)
	if err != nil {
		return err
	}

	r := image.New(image.Options{Format: format, Binary: cfg.Chafa})
	out, err := r.Convert(ctx, data)
	if err != nil {
		return err
	}

	// out is a raw terminal escape-sequence stream — write it verbatim,
	// no printing dance around it.
	if _, err := os.Stdout.Write(out); err != nil {
		return fmt.Errorf("writing image output: %w", err)
	}
	return nil
}
