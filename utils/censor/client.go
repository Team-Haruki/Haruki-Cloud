package censor

import (
	"fmt"

	"haruki-cloud/config"

	"github.com/bytedance/sonic"

	"github.com/go-resty/resty/v2"
)

type BaiduTextCensorClient struct {
	apiKey      string
	secretKey   string
	accessToken string
	client      *resty.Client
}

func NewBaiduTextCensorClient(apiKey, secretKey string) *BaiduTextCensorClient {
	return &BaiduTextCensorClient{
		apiKey:    apiKey,
		secretKey: secretKey,
		client:    resty.New().SetTimeout(config.HTTPClientTimeout),
	}
}

func (b *BaiduTextCensorClient) getAccessToken() error {
	url := "https://aip.baidubce.com/oauth/2.0/token"
	resp, err := b.client.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Accept", "application/json").
		SetQueryParams(map[string]string{
			"grant_type":    "client_credentials",
			"client_id":     b.apiKey,
			"client_secret": b.secretKey,
		}).
		Post(url)

	if err != nil {
		return err
	}

	var data map[string]any
	if err := sonic.Unmarshal(resp.Body(), &data); err != nil {
		return err
	}

	token, ok := data["access_token"].(string)
	if !ok {
		return fmt.Errorf("failed to parse access_token: %v", data)
	}
	b.accessToken = token
	return nil
}

func (b *BaiduTextCensorClient) Init() error {
	if b.client == nil {
		b.client = resty.New().SetTimeout(config.HTTPClientTimeout)
	}
	if b.accessToken == "" {
		return b.getAccessToken()
	}
	return nil
}

func (b *BaiduTextCensorClient) TextCensor(text string) (map[string]any, error) {
	if b.accessToken == "" {
		if err := b.Init(); err != nil {
			return nil, err
		}
	}

	url := fmt.Sprintf("https://aip.baidubce.com/rest/2.0/solution/v1/text_censor/v2/user_defined?access_token=%s", b.accessToken)
	resp, err := b.client.R().
		SetHeader("Content-Type", "application/x-www-form-urlencoded").
		SetFormData(map[string]string{
			"text": text,
		}).
		Post(url)

	if err != nil {
		return nil, err
	}
	if resp.IsError() {
		return nil, fmt.Errorf("baidu text censor HTTP %d: %s", resp.StatusCode(), resp.String())
	}

	var result map[string]any
	if err := sonic.Unmarshal(resp.Body(), &result); err != nil {
		return nil, err
	}
	if err := baiduTextCensorResultError(result); err != nil {
		return nil, err
	}

	return result, nil
}

func baiduTextCensorResultError(result map[string]any) error {
	code, ok := result["error_code"]
	if !ok {
		return nil
	}
	return fmt.Errorf("baidu text censor API error: code=%v msg=%v", code, result["error_msg"])
}
