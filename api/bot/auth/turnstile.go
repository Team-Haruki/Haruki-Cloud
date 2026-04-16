package auth

import (
	"encoding/json"
	"errors"
	"fmt"

	"haruki-cloud/config"

	"github.com/go-resty/resty/v2"
)

const (
	turnstileVerifyURL = "https://challenges.cloudflare.com/turnstile/v0/siteverify"
)

var (
	ErrTurnstileVerificationFailed = errors.New("turnstile verification failed")
	ErrTurnstileNetworkError       = errors.New("network error during verification")
)

type turnstileVerifyResponse struct {
	Success     bool     `json:"success"`
	ChallengeTs string   `json:"challenge_ts,omitempty"`
	Hostname    string   `json:"hostname,omitempty"`
	ErrorCodes  []string `json:"error-codes,omitempty"`
	Action      string   `json:"action,omitempty"`
	Cdata       string   `json:"cdata,omitempty"`
}

type turnstileClient struct {
	secretKey string
	client    *resty.Client
}

func newTurnstileClient(secretKey string) *turnstileClient {
	return &turnstileClient{
		secretKey: secretKey,
		client:    resty.New().SetTimeout(config.HTTPClientTimeout),
	}
}

func (c *turnstileClient) verify(token, remoteIP string) (*turnstileVerifyResponse, error) {
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
		Post(turnstileVerifyURL)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrTurnstileNetworkError, err)
	}
	if resp.StatusCode() != 200 {
		return nil, fmt.Errorf("%w: status code %d", ErrTurnstileNetworkError, resp.StatusCode())
	}
	var result turnstileVerifyResponse
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &result, nil
}

func (c *turnstileClient) VerifyToken(token, remoteIP string) (bool, error) {
	result, err := c.verify(token, remoteIP)
	if err != nil {
		return false, err
	}
	return result.Success, nil
}