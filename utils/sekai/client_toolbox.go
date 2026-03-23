package sekai

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"

	"haruki-cloud/config"

	"github.com/go-resty/resty/v2"
	"github.com/gofiber/fiber/v3"
	"github.com/klauspost/compress/zstd"
)

var (
	toolboxOnce   sync.Once
	toolboxClient *HarukiToolboxClient
)

type HarukiToolboxClient struct {
	http   *resty.Client
	config *config.ToolboxConfig
}

func GetToolboxClient() *HarukiToolboxClient {
	toolboxOnce.Do(func() {
		c := resty.New().
			SetRetryCount(maxRetries).
			SetRetryWaitTime(retryWaitTime).
			AddRetryCondition(func(r *resty.Response, err error) bool {
				// Retry on network-level errors (connection refused, timeout, DNS…)
				if err != nil {
					if _, ok := errors.AsType[net.Error](err); ok {
						return true
					}
					msg := err.Error()
					return strings.Contains(msg, "connection refused") ||
						strings.Contains(msg, "no such host") ||
						strings.Contains(msg, "i/o timeout") ||
						strings.Contains(msg, "EOF")
				}
				// Retry on 5xx — transient server errors
				return r.StatusCode() >= 500
			})

		toolboxClient = &HarukiToolboxClient{
			http:   c,
			config: &config.Cfg.Toolbox,
		}
	})
	return toolboxClient
}

// GetPrivateData fetches private game-data snapshots from the Toolbox API.
//
// On success it returns the raw (decompressed) JSON body.
//
// Typed errors that callers should handle:
//   - ErrAccountBindingNotFound — user has not bound a game account on the suite
//   - ErrGameDataNotFound       — user has not uploaded any data yet
//   - ErrInvalidPlatformUser    — platform/user combo not authorised for this data
//   - ErrAccountOwnerBanned     — game account owner is banned
//   - *ToolboxAPIError          — any other unexpected non-2xx status
func (c *HarukiToolboxClient) GetPrivateData(server, dataType string, userID int64, platform, platformUserID string) ([]byte, error) {
	url := fmt.Sprintf("%s/api/private/%s/%s/%d", c.config.BaseURL, server, dataType, userID)

	resp, err := c.http.R().
		SetHeader("Authorization", c.config.APIToken).
		SetHeader("User-Agent", c.config.UserAgent).
		SetHeader("Accept-Encoding", "zstd").
		SetQueryParams(map[string]string{
			"platform":         platform,
			"platform_user_id": platformUserID,
		}).
		Get(url)
	if err != nil {
		return nil, fmt.Errorf("toolbox: request failed after retries: %w", err)
	}

	switch resp.StatusCode() {
	case fiber.StatusOK:
		return decompress(resp)

	case fiber.StatusForbidden:
		msg := parseMessage(resp.Body())
		switch {
		case strings.Contains(msg, "invalid platform or platform_user_id"):
			return nil, ErrInvalidPlatformUser
		case strings.Contains(msg, "account owner is banned"):
			return nil, ErrAccountOwnerBanned
		default:
			return nil, &ToolboxAPIError{StatusCode: fiber.StatusForbidden, Message: msg}
		}

	case fiber.StatusNotFound:
		msg := parseMessage(resp.Body())
		switch {
		case strings.Contains(msg, "account binding not found"):
			return nil, ErrAccountBindingNotFound
		case strings.Contains(msg, "game data not found"):
			return nil, ErrGameDataNotFound
		default:
			return nil, &ToolboxAPIError{StatusCode: fiber.StatusNotFound, Message: msg}
		}

	case fiber.StatusServiceUnavailable:
		return nil, &ToolboxAPIError{StatusCode: fiber.StatusServiceUnavailable, Message: "toolbox service unavailable"}

	default:
		msg := parseMessage(resp.Body())
		return nil, &ToolboxAPIError{StatusCode: resp.StatusCode(), Message: msg}
	}
}

// decompress handles transparent zstd decompression when the server indicates it.
func decompress(resp *resty.Response) ([]byte, error) {
	body := resp.Body()
	if resp.Header().Get("Content-Encoding") != "zstd" {
		return body, nil
	}

	decoder, err := zstd.NewReader(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("toolbox: zstd reader init failed: %w", err)
	}
	defer decoder.Close()

	out, err := io.ReadAll(decoder)
	if err != nil {
		return nil, fmt.Errorf("toolbox: zstd decompression failed: %w", err)
	}
	return out, nil
}
