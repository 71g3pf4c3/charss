package image

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFetch_Happy(t *testing.T) {
	payload := []byte("pretend-png-bytes")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	f := NewFetcher()
	got, err := f.Fetch(context.Background(), srv.URL+"/img.png")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if string(got) != string(payload) {
		t.Errorf("Fetch = %q, want %q", got, payload)
	}
}

func TestFetch_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	f := NewFetcher()
	_, err := f.Fetch(context.Background(), srv.URL+"/missing.png")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("err = %q, want it to mention 404", err)
	}
}

func TestFetch_ContextCancel(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release // hold the response until the test lets go
	}))
	defer func() {
		close(release)
		srv.Close()
	}()

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	f := NewFetcher()
	_, err := f.Fetch(ctx, srv.URL+"/slow.png")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err := ctx.Err(); err != context.Canceled {
		t.Errorf("request context not cancelled: %v", err)
	}
}

func TestFetch_TimeoutOption(t *testing.T) {
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-block
	}))
	defer func() {
		// unblock the handler before waiting on srv.Close(), else Close deadlocks
		close(block)
		srv.Close()
	}()

	f := NewFetcher(WithTimeout(50 * time.Millisecond))
	if f.client.Timeout != 50*time.Millisecond {
		t.Fatalf("client timeout = %v, want 50ms", f.client.Timeout)
	}
	_, err := f.Fetch(context.Background(), srv.URL+"/hang.png")
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}
