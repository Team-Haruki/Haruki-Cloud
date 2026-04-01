package turnstile

import (
	"encoding/json"
	"errors"
	"fmt"

	"haruki-cloud/config"

	"github.com/go-resty/resty/v2"
)

const (
	verifyURL = "https://challenges.cloudflare.com/turnstile/v0/siteverify"
)

var (
	ErrVerificationFailed = errors.New("turnstile verification failed")
	ErrNetworkError       = errors.New("network error during verification")
)

type VerifyResponse struct {
	Success     bool     `json:"success"`
	ChallengeTs string   `json:"challenge_ts,omitempty"`
	Hostname    string   `json:"hostname,omitempty"`
	ErrorCodes  []string `json:"error-codes,omitempty"`
	Action      string   `json:"action,omitempty"`
	Cdata       string   `json:"cdata,omitempty"`
}

type Client struct {
	secretKey string
	client    *resty.Client
}

func NewClient(secretKey string) *Client {
	return &Client{
		secretKey: secretKey,
		client:    resty.New().SetTimeout(config.HTTPClientTimeout),
	}
}

func (c *Client) Verify(token, remoteIP string) (*VerifyResponse, error) {
	if token == "" {
		return nil, errors.New("empty turnstile token")
	}
	formData := map[string]string{
		"secret":   c.secretKey,
		"response": token,
	}
	if remoteIP != "" {
		formData["remoteip"] = remoteIP
	}
	resp, err := c.client.R().
		SetFormData(formData).
		Post(verifyURL)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNetworkError, err)
	}
	if resp.StatusCode() != 200 {
		return nil, fmt.Errorf("%w: status code %d", ErrNetworkError, resp.StatusCode())
	}
	var result VerifyResponse
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &result, nil
}

func (c *Client) VerifyToken(token, remoteIP string) (bool, error) {
	result, err := c.Verify(token, remoteIP)
	if err != nil {
		return false, err
	}
	return result.Success, nil
}
