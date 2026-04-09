package sekai

import (
	"fmt"
	"sync"
	"time"

	"haruki-cloud/config"

	"github.com/bytedance/sonic"
	"github.com/go-resty/resty/v2"
)

const (
	maxRetries    = 4
	retryWaitTime = time.Second
	apiTimeout    = 5 * time.Second
	tokenHeader   = "X-Haruki-Sekai-Token"
)

var (
	apiOnce   sync.Once
	apiClient *SekaiAPIClient
)

type SekaiAPIClient struct {
	http   *resty.Client
	config *config.SekaiAPIConfig
}

func GetSekaiAPIClient() *SekaiAPIClient {
	apiOnce.Do(func() {
		c := newRestyClient().SetTimeout(apiTimeout)

		apiClient = &SekaiAPIClient{
			http:   c,
			config: &config.Cfg.SekaiAPI,
		}
	})
	return apiClient
}

// authReq returns a pre-configured request with the token header set.
func (c *SekaiAPIClient) authReq() *resty.Request {
	return c.http.R().SetHeader(tokenHeader, c.config.Token)
}

// GetUserProfile fetches a user's game profile.
//
//	GET /api/{server}/{userID}/profile
//
// Returns ErrUserNotFound on HTTP 404.
func (c *SekaiAPIClient) GetUserProfile(server, userID string) (*GetAnotherProfileResponse, error) {
	url := fmt.Sprintf("%s/api/%s/%s/profile", c.config.BaseURL, server, userID)
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
func (c *SekaiAPIClient) GetSystem(server string) (*GetSystemResponse, error) {
	url := fmt.Sprintf("%s/api/%s/system", c.config.BaseURL, server)
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
func (c *SekaiAPIClient) GetInformation(server string) (*GetInformationResponse, error) {
	url := fmt.Sprintf("%s/api/%s/information", c.config.BaseURL, server)
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
func (c *SekaiAPIClient) GetMySekaiImage(server, imagePath string) ([]byte, error) {
	url := fmt.Sprintf("%s/image/%s/mysekai/%s", c.config.BaseURL, server, imagePath)
	return c.get(url)
}

// get executes a GET request with auth and maps HTTP status codes to typed errors.
func (c *SekaiAPIClient) get(url string) ([]byte, error) {
	resp, err := c.authReq().Get(url)
	if err != nil {
		return nil, fmt.Errorf("sekai api: request failed after retries: %w", err)
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
		return nil, &SekaiAPIError{StatusCode: resp.StatusCode(), Message: msg}
	}
}
