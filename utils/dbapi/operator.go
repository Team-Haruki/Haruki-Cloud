package dbapi

import (
	"fmt"
	"haruki-cloud/utils/model"
)

// HarukiDBOperator extends HarukiDBClient with database operations
type HarukiDBOperator struct {
	*HarukiDBClient
}

// NewHarukiDBOperator creates a new HarukiDBOperator instance
func NewHarukiDBOperator(
	dbAPI string,
	suiteAPI string,
	dbAPIAuthorizationToken string,
	suiteAPIAuthorizationToken string,
) *HarukiDBOperator {
	return &HarukiDBOperator{
		HarukiDBClient: NewHarukiDBClient(
			dbAPI,
			suiteAPI,
			dbAPIAuthorizationToken,
			suiteAPIAuthorizationToken,
		),
	}
}

// ========== PJSK Binding APIs ==========

// GetPjskBinding retrieves PJSK binding information
func (o *HarukiDBOperator) GetPjskBinding(
	platform model.InstantMessengerPlatform,
	imID string,
	server *model.SekaiBindingServerRegion,
) (interface{}, int, error) {
	path := fmt.Sprintf("/pjsk/%s/user/%s/binding", platform.String(), imID)
	var params map[string]string
	if server != nil {
		params = map[string]string{"server": server.String()}
	}
	return o.CallAPI(path, "GET", nil, params, model.RequestDatabaseTypeMain)
}

// AddPjskBinding adds a new PJSK binding
func (o *HarukiDBOperator) AddPjskBinding(
	platform model.InstantMessengerPlatform,
	imID string,
	server *model.SekaiBindingServerRegion,
	data interface{},
) (interface{}, int, error) {
	path := fmt.Sprintf("/pjsk/%s/user/%s/binding", platform.String(), imID)
	var params map[string]string
	if server != nil {
		params = map[string]string{"server": server.String()}
	}
	return o.CallAPI(path, "POST", data, params, model.RequestDatabaseTypeMain)
}

// GetPjskDefaultBinding retrieves the default PJSK binding
func (o *HarukiDBOperator) GetPjskDefaultBinding(
	platform model.InstantMessengerPlatform,
	imID string,
	server model.SekaiBindingServerRegion,
) (interface{}, int, error) {
	path := fmt.Sprintf("/pjsk/%s/user/%s/default", platform.String(), imID)
	params := map[string]string{"server": server.String()}
	return o.CallAPI(path, "GET", nil, params, model.RequestDatabaseTypeMain)
}

// SetPjskDefaultBinding sets the default PJSK binding
func (o *HarukiDBOperator) SetPjskDefaultBinding(
	platform model.InstantMessengerPlatform,
	imID string,
	server model.SekaiBindingServerRegion,
	data interface{},
) (interface{}, int, error) {
	path := fmt.Sprintf("/pjsk/%s/user/%s/default", platform.String(), imID)
	params := map[string]string{"server": server.String()}
	return o.CallAPI(path, "PUT", data, params, model.RequestDatabaseTypeMain)
}

// DeletePjskDefaultBinding deletes the default PJSK binding
func (o *HarukiDBOperator) DeletePjskDefaultBinding(
	platform model.InstantMessengerPlatform,
	imID string,
	server model.SekaiBindingServerRegion,
) (interface{}, int, error) {
	path := fmt.Sprintf("/pjsk/%s/user/%s/default", platform.String(), imID)
	params := map[string]string{"server": server.String()}
	return o.CallAPI(path, "DELETE", nil, params, model.RequestDatabaseTypeMain)
}

// DeletePjskBinding deletes a PJSK binding
func (o *HarukiDBOperator) DeletePjskBinding(
	platform model.InstantMessengerPlatform,
	imID string,
	server *model.SekaiBindingServerRegion,
) (interface{}, int, error) {
	path := fmt.Sprintf("/pjsk/%s/user/%s/binding", platform.String(), imID)
	var params map[string]string
	if server != nil {
		params = map[string]string{"server": server.String()}
	}
	return o.CallAPI(path, "DELETE", nil, params, model.RequestDatabaseTypeMain)
}

// UpdatePjskBinding updates a PJSK binding
func (o *HarukiDBOperator) UpdatePjskBinding(
	platform model.InstantMessengerPlatform,
	imID string,
	server *model.SekaiBindingServerRegion,
	data interface{},
) (interface{}, int, error) {
	path := fmt.Sprintf("/pjsk/%s/user/%s/binding", platform.String(), imID)
	var params map[string]string
	if server != nil {
		params = map[string]string{"server": server.String()}
	}
	return o.CallAPI(path, "PUT", data, params, model.RequestDatabaseTypeMain)
}

// ========== PJSK Alias APIs ==========

// QueryObjectIDByAlias queries object ID by alias
func (o *HarukiDBOperator) QueryObjectIDByAlias(
	alias string,
	aliasType model.PjskAliasType,
	groupID *int,
) (interface{}, int, error) {
	path := fmt.Sprintf("/pjsk/alias/%s/%s", aliasType.String(), alias)
	var params map[string]string
	if groupID != nil {
		params = map[string]string{"group_id": fmt.Sprintf("%d", *groupID)}
	}
	return o.CallAPI(path, "GET", nil, params, model.RequestDatabaseTypeMain)
}

// GetAllAliases retrieves all aliases for an object
func (o *HarukiDBOperator) GetAllAliases(
	aliasID int,
	aliasType model.PjskAliasType,
	groupID *int,
) (interface{}, int, error) {
	path := fmt.Sprintf("/pjsk/alias/%s/%d/all", aliasType.String(), aliasID)
	var params map[string]string
	if groupID != nil {
		params = map[string]string{"group_id": fmt.Sprintf("%d", *groupID)}
	}
	return o.CallAPI(path, "GET", nil, params, model.RequestDatabaseTypeMain)
}

// AddAlias adds a new alias
func (o *HarukiDBOperator) AddAlias(
	aliasID int,
	aliasType model.PjskAliasType,
	groupID *int,
	data interface{},
) (interface{}, int, error) {
	path := fmt.Sprintf("/pjsk/alias/%s/%d", aliasType.String(), aliasID)
	var params map[string]string
	if groupID != nil {
		params = map[string]string{"group_id": fmt.Sprintf("%d", *groupID)}
	}
	return o.CallAPI(path, "POST", data, params, model.RequestDatabaseTypeMain)
}

// DeleteAlias deletes an alias
func (o *HarukiDBOperator) DeleteAlias(
	aliasID int,
	aliasType model.PjskAliasType,
	groupID *int,
) (interface{}, int, error) {
	path := fmt.Sprintf("/pjsk/alias/%s/%d", aliasType.String(), aliasID)
	var params map[string]string
	if groupID != nil {
		params = map[string]string{"group_id": fmt.Sprintf("%d", *groupID)}
	}
	return o.CallAPI(path, "DELETE", nil, params, model.RequestDatabaseTypeMain)
}

// GetPendingReviewAliases retrieves pending review aliases
func (o *HarukiDBOperator) GetPendingReviewAliases() (interface{}, int, error) {
	return o.CallAPI("/pjsk/alias/pending", "GET", nil, nil, model.RequestDatabaseTypeMain)
}

// ApproveAlias approves a pending alias
func (o *HarukiDBOperator) ApproveAlias(pendingReviewID int) (interface{}, int, error) {
	path := fmt.Sprintf("/pjsk/alias/pending/%d/approve", pendingReviewID)
	return o.CallAPI(path, "POST", nil, nil, model.RequestDatabaseTypeMain)
}

// RejectAlias rejects a pending alias
func (o *HarukiDBOperator) RejectAlias(pendingReviewID int, reason *string) (interface{}, int, error) {
	path := fmt.Sprintf("/pjsk/alias/pending/%d/reject", pendingReviewID)
	var data interface{}
	if reason != nil {
		data = map[string]string{"reason": *reason}
	}
	return o.CallAPI(path, "POST", data, nil, model.RequestDatabaseTypeMain)
}

// GetPendingReviewAliasStatus retrieves the status of a pending alias
func (o *HarukiDBOperator) GetPendingReviewAliasStatus(pendingReviewID int) (interface{}, int, error) {
	path := fmt.Sprintf("/pjsk/alias/pending/%d", pendingReviewID)
	return o.CallAPI(path, "GET", nil, nil, model.RequestDatabaseTypeMain)
}

// ========== PJSK User Preferences APIs ==========

// GetAllPreferences retrieves all user preferences
func (o *HarukiDBOperator) GetAllPreferences(
	platform model.InstantMessengerPlatform,
	imID string,
) (interface{}, int, error) {
	path := fmt.Sprintf("/pjsk/%s/user/%s/preferences", platform.String(), imID)
	return o.CallAPI(path, "GET", nil, nil, model.RequestDatabaseTypeMain)
}

// GetSpecificPreference retrieves a specific preference
func (o *HarukiDBOperator) GetSpecificPreference(
	platform model.InstantMessengerPlatform,
	imID string,
	option string,
) (interface{}, int, error) {
	path := fmt.Sprintf("/pjsk/%s/user/%s/preferences/%s", platform.String(), imID, option)
	return o.CallAPI(path, "GET", nil, nil, model.RequestDatabaseTypeMain)
}

// UpdateSpecificPreference updates a specific preference
func (o *HarukiDBOperator) UpdateSpecificPreference(
	platform model.InstantMessengerPlatform,
	imID string,
	option string,
	value string,
) (interface{}, int, error) {
	path := fmt.Sprintf("/pjsk/%s/user/%s/preferences/%s", platform.String(), imID, option)
	data := map[string]string{"value": value}
	return o.CallAPI(path, "PUT", data, nil, model.RequestDatabaseTypeMain)
}

// DeleteSpecificPreference deletes a specific preference
func (o *HarukiDBOperator) DeleteSpecificPreference(
	platform model.InstantMessengerPlatform,
	imID string,
	option string,
) (interface{}, int, error) {
	path := fmt.Sprintf("/pjsk/%s/user/%s/preferences/%s", platform.String(), imID, option)
	return o.CallAPI(path, "DELETE", nil, nil, model.RequestDatabaseTypeMain)
}

// ========== Chunithm Binding APIs ==========

// GetChunithmDefaultServer retrieves the default Chunithm server
func (o *HarukiDBOperator) GetChunithmDefaultServer(
	platform model.InstantMessengerPlatform,
	imID string,
) (interface{}, int, error) {
	path := fmt.Sprintf("/chunithm/%s/user/%s/default", platform.String(), imID)
	return o.CallAPI(path, "GET", nil, nil, model.RequestDatabaseTypeMain)
}

// SetChunithmDefaultServer sets the default Chunithm server
func (o *HarukiDBOperator) SetChunithmDefaultServer(
	platform model.InstantMessengerPlatform,
	imID string,
	server string,
) (interface{}, int, error) {
	path := fmt.Sprintf("/chunithm/%s/user/%s/default", platform.String(), imID)
	data := map[string]string{"server": server}
	return o.CallAPI(path, "PUT", data, nil, model.RequestDatabaseTypeMain)
}

// DeleteChunithmDefaultServer deletes the default Chunithm server
func (o *HarukiDBOperator) DeleteChunithmDefaultServer(
	platform model.InstantMessengerPlatform,
	imID string,
	server string,
) (interface{}, int, error) {
	path := fmt.Sprintf("/chunithm/%s/user/%s/default", platform.String(), imID)
	params := map[string]string{"server": server}
	return o.CallAPI(path, "DELETE", nil, params, model.RequestDatabaseTypeMain)
}

// GetChunithmBinding retrieves Chunithm binding
func (o *HarukiDBOperator) GetChunithmBinding(
	platform model.InstantMessengerPlatform,
	imID string,
	server string,
) (interface{}, int, error) {
	path := fmt.Sprintf("/chunithm/%s/user/%s/binding", platform.String(), imID)
	params := map[string]string{"server": server}
	return o.CallAPI(path, "GET", nil, params, model.RequestDatabaseTypeMain)
}

// UpdateChunithmBinding updates Chunithm binding
func (o *HarukiDBOperator) UpdateChunithmBinding(
	platform model.InstantMessengerPlatform,
	imID string,
	server string,
	data interface{},
) (interface{}, int, error) {
	path := fmt.Sprintf("/chunithm/%s/user/%s/binding", platform.String(), imID)
	params := map[string]string{"server": server}
	return o.CallAPI(path, "PUT", data, params, model.RequestDatabaseTypeMain)
}

// DeleteChunithmBinding deletes Chunithm binding
func (o *HarukiDBOperator) DeleteChunithmBinding(
	platform model.InstantMessengerPlatform,
	imID string,
	server string,
	aimeID int,
) (interface{}, int, error) {
	path := fmt.Sprintf("/chunithm/%s/user/%s/binding", platform.String(), imID)
	params := map[string]string{
		"server":  server,
		"aime_id": fmt.Sprintf("%d", aimeID),
	}
	return o.CallAPI(path, "DELETE", nil, params, model.RequestDatabaseTypeMain)
}

// ========== Chunithm Music APIs ==========

// GetAllChunithmMusic retrieves all Chunithm music
func (o *HarukiDBOperator) GetAllChunithmMusic() (interface{}, int, error) {
	return o.CallAPI("/chunithm/music/all", "GET", nil, nil, model.RequestDatabaseTypeMain)
}

// GetChunithmMusicDifficultyInfo retrieves music difficulty info
func (o *HarukiDBOperator) GetChunithmMusicDifficultyInfo(musicID int) (interface{}, int, error) {
	path := fmt.Sprintf("/chunithm/music/%d/difficulty", musicID)
	return o.CallAPI(path, "GET", nil, nil, model.RequestDatabaseTypeMain)
}

// GetChunithmMusicBasicInfo retrieves music basic info
func (o *HarukiDBOperator) GetChunithmMusicBasicInfo(musicID int) (interface{}, int, error) {
	path := fmt.Sprintf("/chunithm/music/%d/basic", musicID)
	return o.CallAPI(path, "GET", nil, nil, model.RequestDatabaseTypeMain)
}

// GetChunithmChartData retrieves chart data
func (o *HarukiDBOperator) GetChunithmChartData(musicID int) (interface{}, int, error) {
	path := fmt.Sprintf("/chunithm/music/%d/chart", musicID)
	return o.CallAPI(path, "GET", nil, nil, model.RequestDatabaseTypeMain)
}

// QueryChunithmMusicDataBatch queries music data in batch
func (o *HarukiDBOperator) QueryChunithmMusicDataBatch(musicIDs []int) (interface{}, int, error) {
	data := map[string][]int{"music_ids": musicIDs}
	return o.CallAPI("/chunithm/music/batch", "POST", data, nil, model.RequestDatabaseTypeMain)
}

// ========== Chunithm Music Alias APIs ==========

// GetChunithmMusicIDByAlias retrieves music ID by alias
func (o *HarukiDBOperator) GetChunithmMusicIDByAlias(alias string) (interface{}, int, error) {
	path := fmt.Sprintf("/chunithm/music/alias/%s", alias)
	return o.CallAPI(path, "GET", nil, nil, model.RequestDatabaseTypeMain)
}

// GetChunithmMusicAllAliases retrieves all aliases for a music
func (o *HarukiDBOperator) GetChunithmMusicAllAliases(musicID int) (interface{}, int, error) {
	path := fmt.Sprintf("/chunithm/music/%d/aliases", musicID)
	return o.CallAPI(path, "GET", nil, nil, model.RequestDatabaseTypeMain)
}

// AddChunithmMusicAlias adds a music alias
func (o *HarukiDBOperator) AddChunithmMusicAlias(musicID int, alias string) (interface{}, int, error) {
	path := fmt.Sprintf("/chunithm/music/%d/alias", musicID)
	data := map[string]string{"alias": alias}
	return o.CallAPI(path, "POST", data, nil, model.RequestDatabaseTypeMain)
}

// DeleteChunithmMusicAlias deletes a music alias
func (o *HarukiDBOperator) DeleteChunithmMusicAlias(musicID int) (interface{}, int, error) {
	path := fmt.Sprintf("/chunithm/music/%d/alias", musicID)
	return o.CallAPI(path, "DELETE", nil, nil, model.RequestDatabaseTypeMain)
}

// ========== PJSK Suite Data API ==========

// GetPjskSuiteData retrieves PJSK suite data
func (o *HarukiDBOperator) GetPjskSuiteData(
	userID int,
	server model.SekaiBindingServerRegion,
	dataType model.SekaiSuiteDataType,
) (interface{}, int, error) {
	path := fmt.Sprintf("/private/%s/%s/%d", server.String(), dataType.String(), userID)
	return o.CallAPI(path, "GET", nil, nil, model.RequestDatabaseTypeSuite)
}

// UpdatePjskSuiteDataPolicy updates PJSK suite data policy
func (o *HarukiDBOperator) UpdatePjskSuiteDataPolicy(
	userID int,
	server model.SekaiBindingServerRegion,
	dataType model.SekaiSuiteDataType,
	configs map[string]interface{},
) (interface{}, int, error) {
	path := fmt.Sprintf("/private/%s/%s/%d/policy", server.String(), dataType.String(), userID)
	return o.CallAPI(path, "PUT", configs, nil, model.RequestDatabaseTypeSuite)
}
