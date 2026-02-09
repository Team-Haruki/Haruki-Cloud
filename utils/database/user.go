package database

import (
	"fmt"
	"haruki-cloud/utils/model"
)

// GetUserByPlatform queries a user by platform and user ID
func (c *HarukiDBClient) GetUserByPlatform(platform string, userID string) (*model.UserResponse, error) {
	path := "/user/"
	params := map[string]string{
		"platform":       platform,
		"haruki_user_id": userID,
	}
	var resp model.ApiResponse[model.UserResponse]
	_, err := c.Request("GET", path, nil, params, model.RequestDatabaseTypeMain, &resp)
	if err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// CreateUser creates a new user
func (c *HarukiDBClient) CreateUser(req model.CreateUserRequest) (*model.UserResponse, error) {
	path := "/user/"
	var resp model.ApiResponse[model.UserResponse]
	_, err := c.Request("POST", path, req, nil, model.RequestDatabaseTypeMain, &resp)
	if err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// GetUserIDByPlatform queries a user ID by platform and user ID (assuming this was the intent of get user by platform)
// The previous implementation might have been slightly different, but based on swagger:
// GET /user/ -> params: platform, haruki_user_id (wait, swagger says haruki_user_id, but description says platform user ID. Let's check swagger again)

/*
   /user/:
     get:
       parameters:
         - name: platform
           description: 平台标识
         - name: haruki_user_id
           description: 平台用户 ID (Wait, param name is haruki_user_id but desc is platform user ID? That's confusing in swagger)
		   // Actually, let's look at the parameters in swagger.
		   // name: haruki_user_id, in: query, description: 平台用户 ID
		   // This implies the query param key is 'haruki_user_id' but it holds the platform's user id.
*/

// GetUserByHarukiID queries a user by Haruki ID
func (c *HarukiDBClient) GetUserByHarukiID(harukiID int) (*model.UserResponse, error) {
	path := fmt.Sprintf("/user/%d", harukiID)
	var resp model.ApiResponse[model.UserResponse]
	_, err := c.Request("GET", path, nil, nil, model.RequestDatabaseTypeMain, &resp)
	if err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// UpdateUserBan updates user ban status
func (c *HarukiDBClient) UpdateUserBan(harukiID int, req model.UpdateBanRequest) (*model.UserResponse, error) {
	path := fmt.Sprintf("/user/%d/ban", harukiID)
	var resp model.ApiResponse[model.UserResponse]
	_, err := c.Request("PATCH", path, req, nil, model.RequestDatabaseTypeMain, &resp)
	if err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// UpdateFeatureBan updates a specific feature ban status
func (c *HarukiDBClient) UpdateFeatureBan(harukiID int, feature string, subFeature string, req model.UpdateFeatureBanRequest) (*model.UserResponse, error) {
	var path string
	if subFeature != "" {
		path = fmt.Sprintf("/user/%d/ban/%s/%s", harukiID, feature, subFeature)
	} else {
		path = fmt.Sprintf("/user/%d/ban/%s", harukiID, feature)
	}
	var resp model.ApiResponse[model.UserResponse]
	_, err := c.Request("PATCH", path, req, nil, model.RequestDatabaseTypeMain, &resp)
	if err != nil {
		return nil, err
	}
	return &resp.Data, nil
}
