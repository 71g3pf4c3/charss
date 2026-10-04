package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"strconv"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/71g3pf4c3/charss/internal/podcast"
)

var (
	podcastDownloadAll bool
	podcastDownloadDir string
)

// podcastCmd manages the persistent podcast download queue
// ($XDG_CACHE_HOME/charss/queue.json). Enqueueing from the TUI comes
// later; these are the queue-management commands.
var podcastCmd = &cobra.Command{
	Use:   "podcast",
	Short: "Manage the podcast download queue",
}

var podcastListCmd = &cobra.Command{
	Use:   "list",
	Short: "List queued podcast downloads",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		q, err := podcast.OpenQueue("")
		if err != nil {
			return err
		}
		items := q.Items()
		if len(items) == 0 {
			fmt.Println("download queue is empty")
			return nil
		}
		tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
		fmt.Fprintln(tw, "STATUS\tSIZE\tITEM")
		for _, it := range items {
			row := fmt.Sprintf("%s\t%s\t%s", it.Status, humanSize(it.Size), displayURL(it))
			switch {
			case it.FilePath != "":
				row += "\t-> " + it.FilePath
			case it.Error != "":
				row += "\t! " + it.Error
			}
			fmt.Fprintln(tw, row)
		}
		return tw.Flush()
	},
}

var podcastDownloadCmd = &cobra.Command{
	Use:   "download",
	Short: "Download queued podcasts (one item, or --all to drain the queue)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		q, err := podcast.OpenQueue("")
		if err != nil {
			return err
		}

		dir := podcastDownloadDir
		if dir == "" {
			if dir, err = defaultPodcastDir(); err != nil {
				return err
			}
		}

		// SIGINT/SIGTERM cancel the current transfer; the partial .part
		// file and the Downloading status are kept and resumed next run.
		ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
		defer stop()

		progress := func(url string, done, total int64) {
			fmt.Fprintf(os.Stderr, "\r  %s %s/%s", urlBasename(url), humanSize(done), humanSize(total))
		}
		d := &podcast.Downloader{
			Dir:        dir,
			Client:     &http.Client{},
			OnProgress: progress,
		}

		var ok, failed int
		for {
			item, err := d.Next(ctx, q)
			if errors.Is(err, podcast.ErrQueueEmpty) {
				break
			}
			if err != nil {
				failed++
				fmt.Fprintln(os.Stderr) // end any partial progress line
				fmt.Fprintf(os.Stderr, "error: %s: %v\n", item.URL, err)
				if errors.Is(err, context.Canceled) {
					fmt.Fprintln(os.Stderr, "interrupted; the partial download will resume on the next run")
					fmt.Printf("downloaded %d, failed %d (interrupted)\n", ok, failed)
					return nil
				}
				if !podcastDownloadAll {
					return err
				}
				continue
			}
			ok++
			fmt.Fprintln(os.Stderr) // end the progress line
			fmt.Fprintf(os.Stderr, "downloaded %s -> %s\n", item.URL, item.FilePath)
		}
		fmt.Printf("downloaded %d, failed %d\n", ok, failed)
		return nil
	},
}

var podcastRemoveCmd = &cobra.Command{
	Use:   "remove <url>",
	Short: "Remove a URL from the download queue",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		q, err := podcast.OpenQueue("")
		if err != nil {
			return err
		}
		if err := q.Remove(args[0]); err != nil {
			return err
		}
		fmt.Printf("removed %s\n", args[0])
		return nil
	},
}

var podcastClearFailedCmd = &cobra.Command{
	Use:   "clear-failed",
	Short: "Remove all failed items from the download queue",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		q, err := podcast.OpenQueue("")
		if err != nil {
			return err
		}
		var n int
		for _, it := range q.Items() {
			if it.Status != podcast.StatusFailed {
				continue
			}
			if err := q.Remove(it.URL); err != nil {
				return err
			}
			n++
		}
		fmt.Printf("removed %d failed item(s)\n", n)
		return nil
	},
}

func init() {
	podcastDownloadCmd.Flags().BoolVar(&podcastDownloadAll, "all", false,
		"download until the queue has no queued items left")
	podcastDownloadCmd.Flags().StringVar(&podcastDownloadDir, "dir", "",
		"download directory (default ~/.local/share/charss/downloads)")
	podcastCmd.AddCommand(podcastListCmd)
	podcastCmd.AddCommand(podcastDownloadCmd)
	podcastCmd.AddCommand(podcastRemoveCmd)
	podcastCmd.AddCommand(podcastClearFailedCmd)
	rootCmd.AddCommand(podcastCmd)
}

// defaultPodcastDir is ~/.local/share/charss/downloads (XDG data home).
func defaultPodcastDir() (string, error) {
	if base := os.Getenv("XDG_DATA_HOME"); base != "" {
		return filepath.Join(base, "charss", "downloads"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locating data dir: %w", err)
	}
	return filepath.Join(home, ".local", "share", "charss", "downloads"), nil
}

// urlBasename is the URL's path basename (query stripped), for display.
func urlBasename(raw string) string {
	if u, err := url.Parse(raw); err == nil && u.Path != "" {
		if b := path.Base(u.Path); b != "" && b != "/" && b != "." {
			return b
		}
	}
	return raw
}

// displayURL prefers the title with the URL in brackets for readability.
func displayURL(it podcast.Item) string {
	if it.Title != "" {
		return fmt.Sprintf("%s (%s)", it.Title, it.URL)
	}
	return it.URL
}

// humanSize renders byte counts for humans; -1 (unknown) becomes "?".
func humanSize(n int64) string {
	if n < 0 {
		return "?"
	}
	const unit = 1024
	if n < unit {
		return strconv.FormatInt(n, 10) + "B"
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%c", float64(n)/float64(div), "KMGTPE"[exp])
}
