package podcast

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

// Downloader fetches queued enclosures sequentially into Dir.
//
// Client defaults to a plain &http.Client{} (no overall timeout — a
// Timeout there also bounds the response body, which breaks large
// downloads; use ctx for cancellation). OnProgress, when set, is called
// from the goroutine running Next on every read with cumulative
// done/total byte counts (total -1 when the server did not report a
// length).
type Downloader struct {
	Dir        string // download dir; caller decides the default
	Client     *http.Client
	OnProgress func(url string, done, total int64)
}

// Next downloads ONE item: the first with status Queued (or Downloading —
// a leftover from an interrupted run, resumed via the Range header), and
// drives its state machine Queued/Downloading -> Done/Failed. It returns
// the (updated) item; on error the returned item reflects the queue state
// where possible. ErrQueueEmpty means nothing is actionable.
//
// Failure model:
//   - non-2xx response, or any transport error other than ctx
//     cancellation -> MarkFailed; re-enqueue (remove + add) to retry;
//   - ctx canceled mid-download -> the .part file and the Downloading
//     status are kept, so the next Next call resumes from the partial
//     file (servers that ignore Range restart it cleanly instead).
//
// Next is meant to be called sequentially by one driver at a time; there
// is no cross-process lock on "Downloading" items.
func (d *Downloader) Next(ctx context.Context, q *Queue) (Item, error) {
	var item Item
	found := false
	for _, it := range q.Items() {
		if it.Status == StatusQueued || it.Status == StatusDownloading {
			item = it
			found = true
			break
		}
	}
	if !found {
		return Item{}, ErrQueueEmpty
	}

	if err := q.MarkDownloading(item.URL); err != nil {
		return item, err
	}
	item.Status = StatusDownloading
	item.Error = ""
	if err := os.MkdirAll(d.Dir, 0o755); err != nil {
		d.fail(q, &item, fmt.Sprintf("create %s: %v", d.Dir, err))
		return item, fmt.Errorf("podcast: create %s: %w", d.Dir, err)
	}

	target := reservePath(d.Dir, filenameFromURL(item.URL))
	part := target + ".part"

	// Resume: pick up an existing .part file.
	var offset int64
	if fi, err := os.Stat(part); err == nil {
		offset = fi.Size()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, item.URL, nil)
	if err != nil {
		d.fail(q, &item, err.Error())
		return item, fmt.Errorf("podcast: %w", err)
	}
	if offset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
	}

	if d.Client == nil {
		d.Client = &http.Client{}
	}
	resp, err := d.Client.Do(req)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			// caller-initiated stop: keep .part + Downloading for resume
			return item, fmt.Errorf("podcast: download %s: %w", item.URL, err)
		}
		d.fail(q, &item, err.Error())
		return item, fmt.Errorf("podcast: download %s: %w", item.URL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := fmt.Sprintf("HTTP %d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
		d.fail(q, &item, msg)
		return item, fmt.Errorf("podcast: download %s: %s", item.URL, msg)
	}

	f, err := os.OpenFile(part, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		d.fail(q, &item, err.Error())
		return item, fmt.Errorf("podcast: open %s: %w", part, err)
	}

	// A 200 (or a 206 that does not match our offset) means the server
	// ignored the Range request and is sending the full body: restart
	// the temp file instead of appending a second copy.
	if resp.StatusCode == http.StatusPartialContent && rangeStart(resp) == offset && offset > 0 {
		// resumed at the expected offset; keep appending
	} else {
		if err := f.Truncate(0); err != nil {
			f.Close()
			d.fail(q, &item, err.Error())
			return item, fmt.Errorf("podcast: truncate %s: %w", part, err)
		}
		offset = 0
	}

	total := int64(-1)
	if resp.ContentLength >= 0 {
		total = offset + resp.ContentLength
	}
	if d.OnProgress != nil {
		d.OnProgress(item.URL, offset, total)
	}
	_, err = io.Copy(f, &progressReader{r: resp.Body, url: item.URL, done: offset, total: total, on: d.OnProgress})
	if err != nil {
		f.Close()
		if errors.Is(err, context.Canceled) {
			// keep .part + Downloading; next run resumes
			return item, fmt.Errorf("podcast: download %s: %w", item.URL, err)
		}
		d.fail(q, &item, err.Error())
		return item, fmt.Errorf("podcast: download %s: %w", item.URL, err)
	}
	if err := f.Close(); err != nil {
		d.fail(q, &item, err.Error())
		return item, fmt.Errorf("podcast: close %s: %w", part, err)
	}
	if err := os.Rename(part, target); err != nil {
		d.fail(q, &item, err.Error())
		return item, fmt.Errorf("podcast: rename %s -> %s: %w", part, target, err)
	}
	if err := q.MarkDownloaded(item.URL, target); err != nil {
		return item, err
	}

	item.Status = StatusDone
	item.FilePath = target
	item.Error = ""
	return item, nil
}

func (d *Downloader) fail(q *Queue, item *Item, msg string) {
	_ = q.MarkFailed(item.URL, msg)
	item.Status = StatusFailed
	item.Error = msg
}

// rangeStart parses a Content-Range header ("bytes 5-99/100") and returns
// its start offset; -1 when missing or unparsable.
func rangeStart(resp *http.Response) int64 {
	cr := resp.Header.Get("Content-Range")
	if !strings.HasPrefix(cr, "bytes ") {
		return -1
	}
	rest := strings.TrimPrefix(cr, "bytes ")
	dash := strings.IndexByte(rest, '-')
	if dash <= 0 {
		return -1
	}
	start, err := strconv.ParseInt(rest[:dash], 10, 64)
	if err != nil {
		return -1
	}
	return start
}

// progressReader forwards each read to the OnProgress callback with
// cumulative counts.
type progressReader struct {
	r     io.Reader
	url   string
	done  int64
	total int64
	on    func(url string, done, total int64)
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.done += int64(n)
	if p.on != nil && n > 0 {
		p.on(p.url, p.done, p.total)
	}
	return n, err
}

// filenameFromURL derives a safe file name from the URL's path basename
// (query string excluded); "download" when there is none.
func filenameFromURL(raw string) string {
	name := ""
	if u, err := url.Parse(raw); err == nil {
		name = path.Base(u.Path)
	}
	if name == "" || name == "." || name == "/" || name == `\` {
		name = "download"
	}

	var b strings.Builder
	for _, r := range name {
		switch {
		case r < 0x20 || r == 0x7f, strings.ContainsRune(`<>:"|?*\/`, r):
			b.WriteByte('_')
		default:
			b.WriteRune(r)
		}
	}
	name = strings.TrimRight(b.String(), " .")
	if name == "" {
		name = "download"
	}
	if runes := []rune(name); len(runes) > 200 { // keep room for " (NNN)" + extension
		name = string(runes[len(runes)-200:])
	}
	return name
}

// reservePath returns dir/name, appending " (2)", " (3)"... before the
// extension while the candidate already exists.
func reservePath(dir, name string) string {
	cand := filepath.Join(dir, name)
	if _, err := os.Stat(cand); errors.Is(err, os.ErrNotExist) {
		return cand
	}
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	for i := 2; ; i++ {
		cand = filepath.Join(dir, fmt.Sprintf("%s (%d)%s", stem, i, ext))
		if _, err := os.Stat(cand); errors.Is(err, os.ErrNotExist) {
			return cand
		}
	}
}
