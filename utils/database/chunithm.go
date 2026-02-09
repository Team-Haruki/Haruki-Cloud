package database

import (
	"fmt"
	"haruki-cloud/utils/model"
)

// ================= Chunithm Alias APIs =================

// GetChunithmMusicIDByAlias queries music ID by alias
func (c *HarukiDBClient) GetChunithmMusicIDByAlias(alias string) (*model.AliasToIDResponse, error) {
	path := "/chunithm/alias/music-id"
	params := map[string]string{"alias": alias}
	var resp model.ApiResponse[model.AliasToIDResponse]
	_, err := c.Request("GET", path, nil, params, model.RequestDatabaseTypeMain, &resp)
	if err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// GetChunithmAliasesByMusicID queries aliases by music ID
func (c *HarukiDBClient) GetChunithmAliasesByMusicID(musicID int) (*model.AliasListResponse, error) {
	path := fmt.Sprintf("/chunithm/alias/%d", musicID)
	var resp model.ApiResponse[model.AliasListResponse]
	_, err := c.Request("GET", path, nil, nil, model.RequestDatabaseTypeMain, &resp)
	if err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// AddChunithmMusicAlias adds a music alias
func (c *HarukiDBClient) AddChunithmMusicAlias(musicID int, alias string) error {
	path := fmt.Sprintf("/chunithm/alias/%d", musicID)
	req := model.AliasRequest{Alias: alias}
	_, err := c.Request("POST", path, req, nil, model.RequestDatabaseTypeMain, nil)
	return err
}

// DeleteChunithmMusicAlias deletes a music alias
func (c *HarukiDBClient) DeleteChunithmMusicAlias(musicID int, alias string) error {
	path := fmt.Sprintf("/chunithm/alias/%d", musicID)
	req := model.AliasRequest{Alias: alias}
	_, err := c.Request("DELETE", path, req, nil, model.RequestDatabaseTypeMain, nil)
	return err
}

// ================= Chunithm Binding APIs =================

// GetChunithmDefaultServer queries the default server
func (c *HarukiDBClient) GetChunithmDefaultServer(harukiUserID int) (*model.ChunithmDefaultServer, error) {
	path := fmt.Sprintf("/chunithm/user/%d/default", harukiUserID)
	var resp model.ApiResponse[model.ChunithmDefaultServer]
	_, err := c.Request("GET", path, nil, nil, model.RequestDatabaseTypeMain, &resp)
	if err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// SetChunithmDefaultServer sets the default server
func (c *HarukiDBClient) SetChunithmDefaultServer(harukiUserID int, server string) error {
	path := fmt.Sprintf("/chunithm/user/%d/default/%s", harukiUserID, server)
	_, err := c.Request("PUT", path, nil, nil, model.RequestDatabaseTypeMain, nil)
	return err
}

// DeleteChunithmDefaultServer deletes the default server
func (c *HarukiDBClient) DeleteChunithmDefaultServer(harukiUserID int, server string) error {
	path := fmt.Sprintf("/chunithm/user/%d/default", harukiUserID)
	req := model.SetChunithmDefaultServerRequest{Server: server} // Reusing the struct, body needs server
	_, err := c.Request("DELETE", path, req, nil, model.RequestDatabaseTypeMain, nil)
	return err
}

// GetChunithmBinding queries a binding
func (c *HarukiDBClient) GetChunithmBinding(harukiUserID int, server string) (*model.ChunithmBinding, error) {
	path := fmt.Sprintf("/chunithm/user/%d/%s", harukiUserID, server)
	var resp model.ApiResponse[model.ChunithmBinding]
	_, err := c.Request("GET", path, nil, nil, model.RequestDatabaseTypeMain, &resp)
	if err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// SetChunithmBinding sets a binding
func (c *HarukiDBClient) SetChunithmBinding(harukiUserID int, server string, aimeID string) error {
	path := fmt.Sprintf("/chunithm/user/%d/%s/%s", harukiUserID, server, aimeID)
	_, err := c.Request("PUT", path, nil, nil, model.RequestDatabaseTypeMain, nil)
	return err
}

// DeleteChunithmBinding deletes a binding
func (c *HarukiDBClient) DeleteChunithmBinding(harukiUserID int, server string, aimeID string) error {
	path := fmt.Sprintf("/chunithm/user/%d/%s/%s", harukiUserID, server, aimeID)
	_, err := c.Request("DELETE", path, nil, nil, model.RequestDatabaseTypeMain, nil)
	return err
}

// ================= Chunithm Music APIs =================

// GetAllChunithmMusic queries all music
func (c *HarukiDBClient) GetAllChunithmMusic() ([]model.ChunithmMusicInfo, error) {
	path := "/chunithm/music/all-music"
	var resp model.ApiResponse[[]model.ChunithmMusicInfo]
	_, err := c.Request("GET", path, nil, nil, model.RequestDatabaseTypeMain, &resp)
	if err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// GetChunithmMusicDifficultyInfo queries music difficulty info
func (c *HarukiDBClient) GetChunithmMusicDifficultyInfo(musicID int, version string) (*model.ChunithmMusicDifficulty, error) {
	path := fmt.Sprintf("/chunithm/music/%d/difficulty-info", musicID)
	params := map[string]string{"version": version}
	var resp model.ApiResponse[model.ChunithmMusicDifficulty]
	_, err := c.Request("GET", path, nil, params, model.RequestDatabaseTypeMain, &resp)
	if err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// GetChunithmMusicBasicInfo queries music basic info
func (c *HarukiDBClient) GetChunithmMusicBasicInfo(musicID int) (*model.ChunithmMusicInfo, error) {
	path := fmt.Sprintf("/chunithm/music/%d/basic-info", musicID)
	var resp model.ApiResponse[model.ChunithmMusicInfo]
	_, err := c.Request("GET", path, nil, nil, model.RequestDatabaseTypeMain, &resp)
	if err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// GetChunithmChartData queries chart data
func (c *HarukiDBClient) GetChunithmChartData(musicID int) ([]model.ChunithmChartData, error) {
	path := fmt.Sprintf("/chunithm/music/%d/chart-data", musicID)
	var resp model.ApiResponse[[]model.ChunithmChartData]
	_, err := c.Request("GET", path, nil, nil, model.RequestDatabaseTypeMain, &resp)
	if err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// QueryChunithmMusicDataBatch queries music data in batch
// Note: The response structure for batch query is complex map[int]MusicBatchItemSchema
// We need to define MusicBatchItemSchema in dbclient.go if we want strict typing.
// For now, using interface{} or defining a local struct? Let's add it to dbclient.go later if needed,
// but looking at implementation_plan, I should try to be complete.
// I will use map[string]interface{} for dynamic response or add specific type.
// Given dbclient.go content, I missed MusicBatchItemSchema.
// Let's use generic map for now or a custom struct here.

type ChunithmMusicBatchItem struct {
	Version    *string                 `json:"version"`
	Difficulty []*float64              `json:"difficulty"`
	Info       model.ChunithmMusicInfo `json:"info"`
}

func (c *HarukiDBClient) QueryChunithmMusicDataBatch(musicIDs []int, version string) (map[string]ChunithmMusicBatchItem, error) {
	path := "/chunithm/music/query-batch"
	req := map[string]interface{}{
		"music_ids": musicIDs,
		"version":   version,
	}
	var resp model.ApiResponse[map[string]ChunithmMusicBatchItem]
	_, err := c.Request("POST", path, req, nil, model.RequestDatabaseTypeMain, &resp)
	if err != nil {
		return nil, err
	}
	return resp.Data, nil
}
