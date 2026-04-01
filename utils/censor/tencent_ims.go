package censor

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"haruki-cloud/config"
)

const (
	imsHost    = "ims.tencentcloudapi.com"
	imsService = "ims"
	imsVersion = "2020-12-29"
	imsAction  = "ImageModeration"
)

// IMSSuggestion is the content moderation verdict returned by Tencent IMS.
type IMSSuggestion string

const (
	IMSSuggestionPass   IMSSuggestion = "Pass"
	IMSSuggestionReview IMSSuggestion = "Review"
	IMSSuggestionBlock  IMSSuggestion = "Block"
)

// TencentIMSClient is a client for Tencent Cloud Image Moderation Service (IMS).
// Request signing follows the TC3-HMAC-SHA256 algorithm (API v3).
type TencentIMSClient struct {
	secretID   string
	secretKey  string
	region     string
	bizType    string
	httpClient *http.Client
}

// NewTencentIMSClient returns a new Tencent IMS client.
// region defaults to "ap-guangzhou" if empty.
func NewTencentIMSClient(secretID, secretKey, region, bizType string) *TencentIMSClient {
	if region == "" {
		region = "ap-guangzhou"
	}
	return &TencentIMSClient{
		secretID:   secretID,
		secretKey:  secretKey,
		region:     region,
		bizType:    bizType,
		httpClient: &http.Client{Timeout: config.TencentIMSTimeout},
	}
}

// ImageModerationURL submits an image URL for content moderation.
// Returns Pass, Review, or Block.
func (t *TencentIMSClient) ImageModerationURL(ctx context.Context, fileURL string) (IMSSuggestion, error) {
	return t.imageModeration(ctx, fileURL, "")
}

// ImageModerationBase64 submits a base64-encoded image for content moderation.
func (t *TencentIMSClient) ImageModerationBase64(ctx context.Context, b64 string) (IMSSuggestion, error) {
	return t.imageModeration(ctx, "", b64)
}

func (t *TencentIMSClient) imageModeration(ctx context.Context, fileURL, fileContent string) (IMSSuggestion, error) {
	type reqBody struct {
		BizType     string `json:"BizType,omitempty"`
		FileURL     string `json:"FileUrl,omitempty"`
		FileContent string `json:"FileContent,omitempty"`
	}
	body := reqBody{BizType: t.bizType, FileURL: fileURL, FileContent: fileContent}
	payload, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	respData, err := t.doSignedRequest(ctx, payload)
	if err != nil {
		return "", err
	}

	suggestion, _ := respData["Suggestion"].(string)
	if suggestion == "" {
		return "", fmt.Errorf("tencent IMS: missing Suggestion in response: %v", respData)
	}
	return IMSSuggestion(suggestion), nil
}

// doSignedRequest performs a TC3-HMAC-SHA256 signed POST to the IMS endpoint.
func (t *TencentIMSClient) doSignedRequest(ctx context.Context, payload []byte) (map[string]interface{}, error) {
	now := time.Now().UTC()
	timestamp := strconv.FormatInt(now.Unix(), 10)
	date := now.Format("2006-01-02")

	// Step 1: Canonical request
	payloadHash := tc3HexSHA256(payload)
	canonicalHeaders := "content-type:application/json; charset=utf-8\nhost:" + imsHost + "\n"
	canonicalRequest := strings.Join([]string{
		"POST", "/", "",
		canonicalHeaders,
		"content-type;host",
		payloadHash,
	}, "\n")

	// Step 2: String to sign
	credScope := date + "/" + imsService + "/tc3_request"
	stringToSign := "TC3-HMAC-SHA256\n" + timestamp + "\n" + credScope + "\n" + tc3HexSHA256([]byte(canonicalRequest))

	// Step 3: Derive signing key (TC3 key derivation: 3 nested HMAC rounds)
	signingKey := tc3HMAC(tc3HMAC(tc3HMAC([]byte("TC3"+t.secretKey), date), imsService), "tc3_request")

	// Step 4: Signature
	signature := hex.EncodeToString(tc3HMACRaw(signingKey, []byte(stringToSign)))

	// Step 5: Authorization
	authorization := fmt.Sprintf(
		"TC3-HMAC-SHA256 Credential=%s/%s, SignedHeaders=content-type;host, Signature=%s",
		t.secretID, credScope, signature)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://"+imsHost, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", authorization)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Host", imsHost)
	req.Header.Set("X-TC-Action", imsAction)
	req.Header.Set("X-TC-Version", imsVersion)
	req.Header.Set("X-TC-Timestamp", timestamp)
	req.Header.Set("X-TC-Region", t.region)

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tencent IMS: http: %w", err)
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("tencent IMS: read body: %w", err)
	}

	var outer struct {
		Response map[string]interface{} `json:"Response"`
	}
	if err := json.Unmarshal(b, &outer); err != nil {
		return nil, fmt.Errorf("tencent IMS: unmarshal: %w", err)
	}
	if outer.Response == nil {
		return nil, fmt.Errorf("tencent IMS: empty Response wrapper")
	}
	if apiErr, ok := outer.Response["Error"]; ok {
		return nil, fmt.Errorf("tencent IMS API error: %v", apiErr)
	}
	return outer.Response, nil
}

// tc3HexSHA256 returns the lowercase hex SHA-256 of data.
func tc3HexSHA256(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// tc3HMACRaw returns raw HMAC-SHA256 bytes.
func tc3HMACRaw(key, data []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(data)
	return mac.Sum(nil)
}

// tc3HMAC returns HMAC-SHA256 of a string data with the given key bytes.
func tc3HMAC(key []byte, data string) []byte {
	return tc3HMACRaw(key, []byte(data))
}
