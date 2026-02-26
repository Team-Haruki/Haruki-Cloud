package toolbox

import (
	"bytes"
	"fmt"
	"io"
	"sync"

	"haruki-cloud/config"

	"github.com/go-resty/resty/v2"
	"github.com/klauspost/compress/zstd"
)

var (
	once   sync.Once
	client *ToolboxClient
)

type ToolboxClient struct {
	http   *resty.Client
	config *config.ToolboxConfig
}

func GetClient() *ToolboxClient {
	once.Do(func() {
		client = &ToolboxClient{
			http:   resty.New(),
			config: &config.Cfg.Toolbox,
		}
	})
	return client
}

func (c *ToolboxClient) GetPrivateData(server, dataType string, userID int64, platform, platformUserID string) ([]byte, error) {
	req := c.http.R().
		SetHeader("Authorization", c.config.APIToken).
		SetHeader("User-Agent", c.config.UserAgent).
		SetHeader("Accept-Encoding", "zstd").
		SetQueryParams(map[string]string{
			"platform":         platform,
			"platform_user_id": platformUserID,
		})

	url := fmt.Sprintf("%s/private/%s/%s/%d", c.config.BaseURL, server, dataType, userID)
	resp, err := req.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("toolbox api error: status %d, body: %s", resp.StatusCode(), string(resp.Body()))
	}

	body := resp.Body()

	if resp.Header().Get("Content-Encoding") == "zstd" {
		decoder, err := zstd.NewReader(bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("failed to create zstd reader: %w", err)
		}
		defer decoder.Close()

		decodedBody, err := io.ReadAll(decoder)
		if err != nil {
			return nil, fmt.Errorf("failed to read zstd decompressed body: %w", err)
		}
		return decodedBody, nil
	}

	return body, nil
}
