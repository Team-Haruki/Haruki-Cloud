package database

import (
	"fmt"
	"haruki-cloud/config"
	"haruki-cloud/utils/model"

	"github.com/go-resty/resty/v2"
)

type HarukiDBClient struct {
	dbAPI                      string
	suiteAPI                   string
	dbAPIAuthorizationToken    string
	suiteAPIAuthorizationToken string
	client                     *resty.Client
}

func NewHarukiDBClient(
	dbAPI string,
	suiteAPI string,
	dbAPIAuthorizationToken string,
	suiteAPIAuthorizationToken string,
) *HarukiDBClient {
	return &HarukiDBClient{
		dbAPI:                      dbAPI,
		suiteAPI:                   suiteAPI,
		dbAPIAuthorizationToken:    dbAPIAuthorizationToken,
		suiteAPIAuthorizationToken: suiteAPIAuthorizationToken,
		client:                     nil,
	}
}

func (c *HarukiDBClient) Init() {
	c.client = resty.New()
	c.client.SetHeader("User-Agent", fmt.Sprintf("Haruki-Cloud/v%s", config.Version))
	c.client.SetHeader("Content-Type", "application/json")
	c.client.SetHeader("Accept", "application/json")
}

func (c *HarukiDBClient) Close() {
	c.client = nil
}

// Request executes an HTTP request and unmarshals the response into result if provided
func (c *HarukiDBClient) Request(
	method string,
	path string,
	body interface{},
	queryParams map[string]string,
	dbType model.RequestDatabaseType,
	result interface{},
) (int, error) {
	if c.client == nil {
		return 0, ErrClientNotInitialized
	}

	var token, apiBase string
	switch dbType {
	case model.RequestDatabaseTypeMain:
		token = c.dbAPIAuthorizationToken
		apiBase = c.dbAPI
	case model.RequestDatabaseTypeSuite:
		token = c.suiteAPIAuthorizationToken
		apiBase = c.suiteAPI
	default:
		return 0, ErrInvalidDatabaseType
	}

	req := c.client.R().
		SetHeader("Authorization", fmt.Sprintf("Bearer %s", token))

	if queryParams != nil && len(queryParams) > 0 {
		req.SetQueryParams(queryParams)
	}

	if body != nil {
		req.SetBody(body)
	}

	if result != nil {
		req.SetResult(result)
	}

	url := apiBase + path
	var resp *resty.Response
	var err error

	switch method {
	case "GET":
		resp, err = req.Get(url)
	case "POST":
		resp, err = req.Post(url)
	case "PUT":
		resp, err = req.Put(url)
	case "DELETE":
		resp, err = req.Delete(url)
	case "PATCH":
		resp, err = req.Patch(url)
	default:
		return 0, fmt.Errorf("unsupported HTTP method: %s", method)
	}

	if err != nil {
		return 0, fmt.Errorf("request failed: %w", err)
	}

	return resp.StatusCode(), nil
}
