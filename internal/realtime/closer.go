package realtime

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"haruki-cloud/config"
)

// Closer ends the live streams of a subscription version. The API role
// calls it when a subscription is replaced or cancelled.
type Closer interface {
	CloseStreams(ctx context.Context, subscriptionID int, version string) error
}

// CloseStreams implements Closer in-process (embedded mode).
func (s *Service) CloseStreams(_ context.Context, subscriptionID int, version string) error {
	s.Close(subscriptionID, version)
	return nil
}

// HTTPCloser calls the events role's close route.
type HTTPCloser struct {
	BaseURL       string
	Authorization string
	UserAgent     string
	Client        *http.Client
}

// NewHTTPCloser returns a closer for the events role at baseURL, or nil when
// baseURL is empty.
func NewHTTPCloser(baseURL, authorization, userAgent string) *HTTPCloser {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil
	}
	return &HTTPCloser{
		BaseURL:       baseURL,
		Authorization: strings.TrimSpace(authorization),
		UserAgent:     strings.TrimSpace(userAgent),
		Client:        &http.Client{Timeout: config.DefaultEventsCloseTimeout},
	}
}

// CloseStreams posts the close request; it is a no-op for a nil closer.
func (c *HTTPCloser) CloseStreams(ctx context.Context, subscriptionID int, version string) error {
	version = strings.TrimSpace(version)
	if c == nil || subscriptionID <= 0 || version == "" {
		return nil
	}
	endpoint := c.BaseURL + "/internal/subscriptions/" + strconv.Itoa(subscriptionID) + "/close?" +
		url.Values{"subscription_version": []string{version}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		return err
	}
	if c.Authorization != "" {
		req.Header.Set("Authorization", c.Authorization)
	}
	userAgent := c.UserAgent
	if userAgent == "" {
		userAgent = "Haruki-Cloud"
	}
	req.Header.Set("User-Agent", userAgent)
	client := c.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("events close returned status %d", resp.StatusCode)
	}
	return nil
}
