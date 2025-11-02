package dbapi

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

func (c *HarukiDBClient) CallAPI(
	path string,
	method string,
	data interface{},
	params map[string]string,
	dbType model.RequestDatabaseType,
) (interface{}, int, error) {
	if c.client == nil {
		return nil, 0, ErrClientNotInitialized
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
		return nil, 0, ErrInvalidDatabaseType
	}
	req := c.client.R().
		SetHeader("Authorization", fmt.Sprintf("Bearer %s", token))
	if params != nil && len(params) > 0 {
		req.SetQueryParams(params)
	}
	if data != nil {
		req.SetBody(data)
	}
	var result interface{}
	req.SetResult(&result)
	var resp *resty.Response
	var err error
	url := apiBase + path
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
		return nil, 0, fmt.Errorf("unsupported HTTP method: %s", method)
	}
	if err != nil {
		return nil, 0, fmt.Errorf("failed to execute request: %w", err)
	}
	return result, resp.StatusCode(), nil
}

func (c *HarukiDBClient) Get(path string, params map[string]string, dbType model.RequestDatabaseType) (interface{}, int, error) {
	return c.CallAPI(path, "GET", nil, params, dbType)
}

func (c *HarukiDBClient) Post(path string, data interface{}, params map[string]string, dbType model.RequestDatabaseType) (interface{}, int, error) {
	return c.CallAPI(path, "POST", data, params, dbType)
}

func (c *HarukiDBClient) Put(path string, data interface{}, params map[string]string, dbType model.RequestDatabaseType) (interface{}, int, error) {
	return c.CallAPI(path, "PUT", data, params, dbType)
}

func (c *HarukiDBClient) Delete(path string, params map[string]string, dbType model.RequestDatabaseType) (interface{}, int, error) {
	return c.CallAPI(path, "DELETE", nil, params, dbType)
}

func (c *HarukiDBClient) Patch(path string, data interface{}, params map[string]string, dbType model.RequestDatabaseType) (interface{}, int, error) {
	return c.CallAPI(path, "PATCH", data, params, dbType)
}
