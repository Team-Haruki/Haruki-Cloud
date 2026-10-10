package realtime

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"haruki-cloud/config"
	harukiLogger "haruki-cloud/utils/logger"
)

// forwardConcurrency bounds in-flight legacy forwards; beyond it a forward
// is dropped rather than queued.
const forwardConcurrency = 32

// forwarder tees accepted ingest bodies to the legacy gateway while both run
// side by side. It is best effort: failures are logged and never retried.
type forwarder struct {
	url           string
	authorization string
	client        *http.Client
	log           *harukiLogger.Logger
	slots         chan struct{}
	inflight      sync.WaitGroup
}

func newForwarder(url, token string, log *harukiLogger.Logger) *forwarder {
	return &forwarder{
		url:           url,
		authorization: bearer(token),
		client:        &http.Client{Timeout: config.DefaultEventsForwardTimeout},
		log:           log,
		slots:         make(chan struct{}, forwardConcurrency),
	}
}

// bearer prefixes token with "Bearer " unless it already carries a scheme.
func bearer(token string) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return ""
	}
	if len(token) >= len("bearer ") && strings.EqualFold(token[:len("bearer ")], "bearer ") {
		return token
	}
	return "Bearer " + token
}

// forward posts body asynchronously.
func (f *forwarder) forward(body []byte) {
	select {
	case f.slots <- struct{}{}:
	default:
		f.log.Warn("realtime legacy forward dropped", "event", "realtime_forward_dropped")
		return
	}
	payload := bytes.Clone(body)
	f.inflight.Add(1)
	go func() {
		defer f.inflight.Done()
		defer func() { <-f.slots }()
		if err := f.post(payload); err != nil {
			f.log.Warn("realtime legacy forward failed", "event", "realtime_forward_failed", "error", err.Error())
		}
	}()
}

func (f *forwarder) post(body []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), config.DefaultEventsForwardTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, f.url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Haruki-Cloud")
	if f.authorization != "" {
		req.Header.Set("Authorization", f.authorization)
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return fmt.Errorf("post: %T", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return nil
}

// wait blocks until in-flight forwards finish or timeout passes.
func (f *forwarder) wait(timeout time.Duration) {
	done := make(chan struct{})
	go func() {
		f.inflight.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
	}
}
