package sekai

import (
	"context"
	"fmt"
	"strings"
	"time"

	"haruki-cloud/config"
	"haruki-cloud/internal/core/upstream"

	"github.com/bytedance/sonic"
	"github.com/go-resty/resty/v2"
)

const (
	maxRetries    = 4
	retryWaitTime = time.Second
	apiTimeout    = 5 * time.Second
	tokenHeader   = "X-Haruki-Sekai-Token"
)

type HarukiSekaiAPIClient struct {
	http   *resty.Client
	config *config.SekaiAPIConfig
	pool   *upstream.Pool
}

// NewSekaiAPIClient constructs a HarukiSekaiAPIClient bound to the supplied config.
// Callers own the returned client; pass it via dependency injection rather than
// reaching for a package-level singleton.
func NewSekaiAPIClient(cfg *config.SekaiAPIConfig) *HarukiSekaiAPIClient {
	var resolvedTargets []upstream.TargetConfig
	if cfg != nil {
		resolvedTargets = upstream.ResolveTargets(cfg.BaseURL, cfg.Targets, "sekai-api")
	}
	return &HarukiSekaiAPIClient{
		http:   newRestyClient().SetTimeout(apiTimeout),
		config: cfg,
		pool:   upstream.NewPool(resolvedTargets),
	}
}

// authReq returns a pre-configured request with the token header set.
func (c *HarukiSekaiAPIClient) authReq() *resty.Request {
	request := c.http.R()
	if c != nil && c.config != nil && c.config.Token != "" {
		request.SetHeader(tokenHeader, c.config.Token)
	}
	return request
}

// GetUserProfile fetches a user's game profile.
//
//	GET /api/{server}/{userID}/profile
//
// Returns ErrUserNotFound on HTTP 404.
func (c *HarukiSekaiAPIClient) GetUserProfile(server, userID string) (*GetAnotherProfileResponse, error) {
	if c == nil {
		return nil, ErrClientNotConfigured
	}
	url := fmt.Sprintf("/api/%s/%s/profile", server, userID)
	body, err := c.get(url)
	if err != nil {
		return nil, err
	}
	var result GetAnotherProfileResponse
	if err := sonic.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("sekai api: failed to unmarshal profile response: %w", err)
	}
	return &result, nil
}

// GetSystem fetches the current game system status.
//
//	GET /api/{server}/system
func (c *HarukiSekaiAPIClient) GetSystem(server string) (*GetSystemResponse, error) {
	if c == nil {
		return nil, ErrClientNotConfigured
	}
	url := fmt.Sprintf("/api/%s/system", server)
	body, err := c.get(url)
	if err != nil {
		return nil, err
	}
	var result GetSystemResponse
	if err := sonic.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("sekai api: failed to unmarshal system response: %w", err)
	}
	return &result, nil
}

// GetInformation fetches in-game information / announcements.
//
//	GET /api/{server}/information
func (c *HarukiSekaiAPIClient) GetInformation(server string) (*GetInformationResponse, error) {
	if c == nil {
		return nil, ErrClientNotConfigured
	}
	url := fmt.Sprintf("/api/%s/information", server)
	body, err := c.get(url)
	if err != nil {
		return nil, err
	}
	var result GetInformationResponse
	if err := sonic.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("sekai api: failed to unmarshal information response: %w", err)
	}
	return &result, nil
}

// GetMySekaiImage downloads a MySekai photo image.
//
//	GET /image/{server}/mysekai/{imagePath}
//
// imagePath is the sub-path identifying the photo, e.g. "12345/6" for CN/TW
// or the raw imagePath value returned by the suite API for JP/EN.
func (c *HarukiSekaiAPIClient) GetMySekaiImage(server, imagePath string) ([]byte, error) {
	if c == nil {
		return nil, ErrClientNotConfigured
	}
	url := fmt.Sprintf("/image/%s/mysekai/%s", server, imagePath)
	return c.get(url)
}

// get executes a GET request with auth and maps HTTP status codes to typed errors.
func (c *HarukiSekaiAPIClient) get(path string) ([]byte, error) {
	baseURL, lease, err := c.acquireTarget()
	if err != nil {
		return nil, err
	}
	if lease != nil {
		defer lease.Release()
	}

	resp, err := c.authReq().Get(baseURL + path)
	if err != nil {
		return nil, fmt.Errorf("sekai api: request failed after retries: %w", sanitizeNetworkError(err))
	}

	switch resp.StatusCode() {
	case 200:
		return resp.Body(), nil
	case 404:
		return nil, ErrUserNotFound
	case 503:
		return nil, ErrServerMaintenance
	default:
		msg := parseMessage(resp.Body())
		return nil, &APIError{StatusCode: resp.StatusCode(), Message: msg}
	}
}

func (c *HarukiSekaiAPIClient) acquireTarget() (string, *upstream.Lease, error) {
	if c == nil {
		return "", nil, ErrClientNotConfigured
	}
	if c.pool != nil && c.pool.Enabled() {
		lease, err := c.pool.Acquire(context.Background())
		if err != nil {
			return "", nil, fmt.Errorf("sekai api: upstream unavailable: %w", err)
		}
		return lease.Target.BaseURL, lease, nil
	}
	if c.config == nil {
		return "", nil, fmt.Errorf("sekai api: base_url is empty")
	}
	baseURL := strings.TrimRight(strings.TrimSpace(c.config.BaseURL), "/")
	if baseURL == "" {
		return "", nil, fmt.Errorf("sekai api: base_url is empty")
	}
	return baseURL, nil, nil
}
