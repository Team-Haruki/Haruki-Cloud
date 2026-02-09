package database

import (
	"fmt"
	"haruki-cloud/utils/model"
)

// ================= PJSK Binding APIs =================

// GetPjskBindings retrieves all bindings for a user
func (c *HarukiDBClient) GetPjskBindings(harukiUserID int, server *model.SekaiBindingServerRegion) (*model.PJSKBindingResponse, error) {
	path := fmt.Sprintf("/pjsk/user/%d/binding", harukiUserID)
	var params map[string]string
	if server != nil {
		params = map[string]string{"server": server.String()}
	}

	var resp model.ApiResponse[model.PJSKBindingResponse]
	_, err := c.Request("GET", path, nil, params, model.RequestDatabaseTypeMain, &resp)
	if err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// CreatePjskBinding creates a new binding
func (c *HarukiDBClient) CreatePjskBinding(harukiUserID int, req model.CreatePJSKBindingRequest) error {
	path := fmt.Sprintf("/pjsk/user/%d/binding", harukiUserID)
	_, err := c.Request("POST", path, req, nil, model.RequestDatabaseTypeMain, nil)
	return err
}

// GetPjskDefaultBinding retrieves the default binding
func (c *HarukiDBClient) GetPjskDefaultBinding(harukiUserID int, server *model.SekaiBindingServerRegion) (*model.PJSKBindingResponse, error) {
	path := fmt.Sprintf("/pjsk/user/%d/binding/default", harukiUserID)
	var params map[string]string
	if server != nil {
		params = map[string]string{"server": server.String()}
	}

	var resp model.ApiResponse[model.PJSKBindingResponse]
	_, err := c.Request("GET", path, nil, params, model.RequestDatabaseTypeMain, &resp)
	if err != nil {
		return nil, err
	}
	// The API returns a BindingResponse which contains a single binding or list.
	// Based on handler code: BindingResponse{Binding: ...}
	return &resp.Data, nil
}

// SetPjskDefaultBinding sets the default binding
func (c *HarukiDBClient) SetPjskDefaultBinding(harukiUserID int, req model.SetPJSKDefaultBindingRequest) error {
	path := fmt.Sprintf("/pjsk/user/%d/binding/default", harukiUserID)
	_, err := c.Request("PUT", path, req, nil, model.RequestDatabaseTypeMain, nil)
	return err
}

// DeletePjskDefaultBinding deletes the default binding
func (c *HarukiDBClient) DeletePjskDefaultBinding(harukiUserID int, req model.DeletePJSKDefaultBindingRequest) error {
	path := fmt.Sprintf("/pjsk/user/%d/binding/default", harukiUserID)
	_, err := c.Request("DELETE", path, req, nil, model.RequestDatabaseTypeMain, nil)
	return err
}

// UpdatePjskBindingVisibility updates the visibility of a binding
func (c *HarukiDBClient) UpdatePjskBindingVisibility(harukiUserID int, bindingID int, req model.UpdatePJSKBindingVisibilityRequest) error {
	path := fmt.Sprintf("/pjsk/user/%d/binding/%d", harukiUserID, bindingID)
	_, err := c.Request("PATCH", path, req, nil, model.RequestDatabaseTypeMain, nil)
	return err
}

// DeletePjskBinding deletes a binding
func (c *HarukiDBClient) DeletePjskBinding(harukiUserID int, bindingID int) error {
	path := fmt.Sprintf("/pjsk/user/%d/binding/%d", harukiUserID, bindingID)
	_, err := c.Request("DELETE", path, nil, nil, model.RequestDatabaseTypeMain, nil)
	return err
}

// ================= PJSK Preference APIs =================

// GetPjskPreferences retrieves all preferences for a user
func (c *HarukiDBClient) GetPjskPreferences(harukiUserID int) (*model.PJSKPreferencesResponse, error) {
	path := fmt.Sprintf("/pjsk/user/%d/preference", harukiUserID)
	var resp model.ApiResponse[model.PJSKPreferencesResponse]
	_, err := c.Request("GET", path, nil, nil, model.RequestDatabaseTypeMain, &resp)
	if err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// GetPjskPreference retrieves a specific preference
func (c *HarukiDBClient) GetPjskPreference(harukiUserID int, option string) (*model.PJSKPreferencesResponse, error) {
	path := fmt.Sprintf("/pjsk/user/%d/preference/%s", harukiUserID, option)
	var resp model.ApiResponse[model.PJSKPreferencesResponse]
	_, err := c.Request("GET", path, nil, nil, model.RequestDatabaseTypeMain, &resp)
	if err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// UpdatePjskPreference updates a specific preference
func (c *HarukiDBClient) UpdatePjskPreference(harukiUserID int, req model.PJSKPreference) error {
	path := fmt.Sprintf("/pjsk/user/%d/preference/%s", harukiUserID, req.Option)
	_, err := c.Request("PUT", path, req, nil, model.RequestDatabaseTypeMain, nil)
	return err
}

// DeletePjskPreference deletes a specific preference
func (c *HarukiDBClient) DeletePjskPreference(harukiUserID int, option string) error {
	path := fmt.Sprintf("/pjsk/user/%d/preference/%s", harukiUserID, option)
	_, err := c.Request("DELETE", path, nil, nil, model.RequestDatabaseTypeMain, nil)
	return err
}

// ================= PJSK Alias APIs =================

// GetPjskGroupAliasToID queries group alias ID
func (c *HarukiDBClient) GetPjskGroupAliasToID(platform string, groupID string, aliasType string, alias string) (*model.AliasToIDResponse, error) {
	path := fmt.Sprintf("/pjsk/alias/group/%s/%s/%s/by-alias", platform, groupID, aliasType)
	params := map[string]string{"alias": alias}
	var resp model.ApiResponse[model.AliasToIDResponse]
	_, err := c.Request("GET", path, nil, params, model.RequestDatabaseTypeMain, &resp)
	if err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// GetPjskGroupAliasesByID queries group aliases by ID
func (c *HarukiDBClient) GetPjskGroupAliasesByID(platform string, groupID string, aliasType string, aliasTypeID int) (*model.AliasListResponse, error) {
	path := fmt.Sprintf("/pjsk/alias/group/%s/%s/%s/%d", platform, groupID, aliasType, aliasTypeID)
	var resp model.ApiResponse[model.AliasListResponse]
	_, err := c.Request("GET", path, nil, nil, model.RequestDatabaseTypeMain, &resp)
	if err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// AddPjskGroupAlias adds a group alias
func (c *HarukiDBClient) AddPjskGroupAlias(platform string, groupID string, aliasType string, aliasTypeID int, req model.AliasRequest) error {
	path := fmt.Sprintf("/pjsk/alias/group/%s/%s/%s/%d", platform, groupID, aliasType, aliasTypeID)
	_, err := c.Request("POST", path, req, nil, model.RequestDatabaseTypeMain, nil)
	return err
}

// DeletePjskGroupAlias deletes a group alias
func (c *HarukiDBClient) DeletePjskGroupAlias(platform string, groupID string, aliasType string, aliasTypeID int, req model.AliasRequest) error {
	path := fmt.Sprintf("/pjsk/alias/group/%s/%s/%s/%d", platform, groupID, aliasType, aliasTypeID)
	_, err := c.Request("DELETE", path, req, nil, model.RequestDatabaseTypeMain, nil)
	return err
}

// GetPjskGlobalAliasToID queries global alias ID
func (c *HarukiDBClient) GetPjskGlobalAliasToID(aliasType string, alias string) (*model.AliasToIDResponse, error) {
	path := fmt.Sprintf("/pjsk/alias/%s/by-alias", aliasType)
	params := map[string]string{"alias": alias}
	var resp model.ApiResponse[model.AliasToIDResponse]
	_, err := c.Request("GET", path, nil, params, model.RequestDatabaseTypeMain, &resp)
	if err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// GetPjskGlobalAliasesByID queries global aliases by ID
func (c *HarukiDBClient) GetPjskGlobalAliasesByID(aliasType string, aliasTypeID int) (*model.AliasListResponse, error) {
	path := fmt.Sprintf("/pjsk/alias/%s/%d", aliasType, aliasTypeID)
	var resp model.ApiResponse[model.AliasListResponse]
	_, err := c.Request("GET", path, nil, nil, model.RequestDatabaseTypeMain, &resp)
	if err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// AddPjskGlobalAlias adds a global alias
func (c *HarukiDBClient) AddPjskGlobalAlias(aliasType string, aliasTypeID int, harukiUserID int, req model.AliasRequest) error {
	path := fmt.Sprintf("/pjsk/alias/%s/%d", aliasType, aliasTypeID)
	params := map[string]string{"haruki_user_id": fmt.Sprintf("%d", harukiUserID)}
	_, err := c.Request("POST", path, req, params, model.RequestDatabaseTypeMain, nil)
	return err
}

// DeletePjskGlobalAlias deletes a global alias
func (c *HarukiDBClient) DeletePjskGlobalAlias(aliasType string, aliasTypeID int, harukiUserID int, req model.AliasRequest) error {
	path := fmt.Sprintf("/pjsk/alias/%s/%d", aliasType, aliasTypeID)
	params := map[string]string{"haruki_user_id": fmt.Sprintf("%d", harukiUserID)}
	_, err := c.Request("DELETE", path, req, params, model.RequestDatabaseTypeMain, nil)
	return err
}
