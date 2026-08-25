package censor

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"haruki-cloud/config"
	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/utils/logger"

	json "haruki-cloud/internal/jsonutil"

	"github.com/go-resty/resty/v2"
	"golang.org/x/sync/singleflight"
)

const baiduMaxResponseBytes = 1 << 20

var (
	errBaiduRequestFailed    = errors.New("baidu text censor request failed")
	errBaiduResponseTooLarge = errors.New("baidu text censor response is too large")
)

type BaiduTextCensorClient struct {
	apiKey      string
	secretKey   string
	mu          sync.RWMutex
	accessToken string
	client      *resty.Client
	tokenFlight singleflight.Group
}

type baiduAccessTokenFlightToken byte

type baiduAccessTokenFlightResult struct {
	token      string
	err        error
	operations []commandtrace.Stats
	leader     *baiduAccessTokenFlightToken
}

func NewBaiduTextCensorClient(apiKey, secretKey string) *BaiduTextCensorClient {
	return &BaiduTextCensorClient{
		apiKey:    apiKey,
		secretKey: secretKey,
		client:    newBaiduHTTPClient(),
	}
}

func newBaiduHTTPClient() *resty.Client {
	return resty.New().
		SetTimeout(config.HTTPClientTimeout).
		SetResponseBodyLimit(baiduMaxResponseBytes)
}

func (b *BaiduTextCensorClient) fetchAccessToken(ctx context.Context) (string, error) {
	ctx = censorContext(ctx)
	url := "https://aip.baidubce.com/oauth/2.0/token"
	finishHTTP := commandtrace.MeasureOperation(ctx, "censor.http")
	resp, err := b.httpClient().R().
		SetContext(ctx).
		SetHeader("Content-Type", "application/json").
		SetHeader("Accept", "application/json").
		SetQueryParams(map[string]string{
			"grant_type":    "client_credentials",
			"client_id":     b.apiKey,
			"client_secret": b.secretKey,
		}).
		Post(url)
	finishHTTP()

	if err != nil {
		return "", sanitizeBaiduRequestError(ctx, err)
	}
	if resp.IsError() {
		return "", fmt.Errorf("baidu OAuth returned HTTP %d", resp.StatusCode())
	}

	finishDecode := commandtrace.MeasureOperation(ctx, "censor.decode")
	var data map[string]any
	err = json.Unmarshal(resp.Body(), &data)
	finishDecode()
	if err == nil {
		if token, ok := data["access_token"].(string); ok {
			return token, nil
		} else {
			err = errors.New("baidu OAuth response is missing access_token")
		}
	}
	return "", err
}

func (b *BaiduTextCensorClient) Init() error {
	return b.InitContext(context.Background())
}

func (b *BaiduTextCensorClient) InitContext(ctx context.Context) error {
	b.httpClient()
	_, err := b.accessTokenContext(ctx, false)
	return err
}

func (b *BaiduTextCensorClient) TextCensor(text string) (map[string]any, error) {
	return b.TextCensorContext(context.Background(), text)
}

func (b *BaiduTextCensorClient) TextCensorContext(ctx context.Context, text string) (map[string]any, error) {
	ctx = censorContext(ctx)
	accessToken, err := b.accessTokenContext(ctx, false)
	if err != nil {
		return nil, err
	}

	url := "https://aip.baidubce.com/rest/2.0/solution/v1/text_censor/v2/user_defined"
	finishHTTP := commandtrace.MeasureOperation(ctx, "censor.http")
	resp, err := b.httpClient().R().
		SetContext(ctx).
		SetQueryParam("access_token", accessToken).
		SetHeader("Content-Type", "application/x-www-form-urlencoded").
		SetFormData(map[string]string{
			"text": text,
		}).
		Post(url)
	finishHTTP()

	if err != nil {
		return nil, sanitizeBaiduRequestError(ctx, err)
	}
	if resp.IsError() {
		return nil, fmt.Errorf("baidu text censor returned HTTP %d", resp.StatusCode())
	}

	finishDecode := commandtrace.MeasureOperation(ctx, "censor.decode")
	var result map[string]any
	err = json.Unmarshal(resp.Body(), &result)
	if err == nil {
		err = baiduTextCensorResultError(result)
	}
	finishDecode()
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (b *BaiduTextCensorClient) httpClient() *resty.Client {
	b.mu.RLock()
	client := b.client
	b.mu.RUnlock()
	if client != nil {
		return client
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	if b.client == nil {
		b.client = newBaiduHTTPClient()
	}
	return b.client
}

func (b *BaiduTextCensorClient) cachedAccessToken() string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.accessToken
}

func (b *BaiduTextCensorClient) storeAccessToken(token string) {
	b.mu.Lock()
	b.accessToken = token
	b.mu.Unlock()
}

func (b *BaiduTextCensorClient) accessTokenContext(ctx context.Context, force bool) (string, error) {
	ctx = censorContext(ctx)
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if !force {
		if token := b.cachedAccessToken(); token != "" {
			return token, nil
		}
	}

	callerToken := new(baiduAccessTokenFlightToken)
	resultCh := b.tokenFlight.DoChan("access-token", func() (any, error) {
		detached := logger.WithContextAttrs(context.Background(), slog.Bool("shared_work", true))
		sharedBase, cancel := context.WithTimeout(detached, config.HTTPClientTimeout)
		defer cancel()
		sharedCtx, trace := commandtrace.WithNewTrace(sharedBase)
		complete := func(result baiduAccessTokenFlightResult) baiduAccessTokenFlightResult {
			result.operations = trace.Snapshot().Operations
			return result
		}
		if !force {
			if token := b.cachedAccessToken(); token != "" {
				return complete(baiduAccessTokenFlightResult{token: token, leader: callerToken}), nil
			}
		}

		token, err := b.fetchAccessToken(sharedCtx)
		if err == nil {
			b.storeAccessToken(token)
		}
		return complete(baiduAccessTokenFlightResult{token: token, err: err, leader: callerToken}), nil
	})

	finishWait := commandtrace.MeasureOperation(ctx, "censor.token_wait")
	var result singleflight.Result
	select {
	case <-ctx.Done():
		finishWait()
		return "", ctx.Err()
	case result = <-resultCh:
		finishWait()
	}
	if result.Err != nil {
		return "", result.Err
	}
	resolved, ok := result.Val.(baiduAccessTokenFlightResult)
	if !ok {
		return "", fmt.Errorf("baidu text censor: unexpected access token result type %T", result.Val)
	}
	commandtrace.MergeOperations(ctx, resolved.operations)
	if resolved.leader != callerToken {
		commandtrace.RecordOperation(ctx, "censor.token_shared", 0)
	}
	if resolved.err != nil {
		return "", resolved.err
	}
	return resolved.token, nil
}

func baiduTextCensorResultError(result map[string]any) error {
	_, ok := result["error_code"]
	if !ok {
		return nil
	}
	return errors.New("baidu text censor API rejected the request")
}

func sanitizeBaiduRequestError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if ctx != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
	}
	if errors.Is(err, resty.ErrResponseBodyTooLarge) {
		return errBaiduResponseTooLarge
	}
	return errBaiduRequestFailed
}
