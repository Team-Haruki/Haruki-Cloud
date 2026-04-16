package query

import (
	"context"
	"sort"
	"time"
	"unicode/utf8"

	"haruki-cloud/database/chunithm/maindb/chunithmbinding"
	"haruki-cloud/database/chunithm/maindb/chunithmdefaultserver"
	"haruki-cloud/database/chunithm/maindb/chunithmmusicalias"
	entchunimusic "haruki-cloud/database/chunithm/music"
	"haruki-cloud/database/chunithm/music/chunithmchartdata"
	"haruki-cloud/database/chunithm/music/chunithmmusic"
	"haruki-cloud/database/chunithm/music/chunithmmusicdifficulty"
	"haruki-cloud/utils/types"
)

func (c *Client) GetChunithmMusicIDByAlias(ctx context.Context, alias string) (*types.AliasToIDResponse, error) {
	if err := c.requireChunithm(); err != nil {
		return nil, err
	}
	if !isValidAlias(alias) {
		return nil, ErrInvalidAlias
	}

	rows, err := c.chunithmMain.ChunithmMusicAlias.
		Query().
		Where(chunithmmusicalias.AliasEQ(alias)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrAliasNotFound
	}

	ids := make([]int, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.MusicID)
	}
	return &types.AliasToIDResponse{MatchIDs: ids}, nil
}

func (c *Client) GetChunithmAliasesByMusicID(ctx context.Context, musicID int) (*types.AliasListResponse, error) {
	if err := c.requireChunithm(); err != nil {
		return nil, err
	}
	if musicID <= 0 {
		return nil, ErrInvalidMusicID
	}

	rows, err := c.chunithmMain.ChunithmMusicAlias.
		Query().
		Where(chunithmmusicalias.MusicIDEQ(musicID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrAliasNotFound
	}

	aliases := make([]string, 0, len(rows))
	for _, row := range rows {
		aliases = append(aliases, row.Alias)
	}
	return &types.AliasListResponse{Aliases: aliases}, nil
}

func (c *Client) GetAllChunithmMusic(ctx context.Context) ([]types.ChunithmMusicInfo, error) {
	if err := c.requireChunithm(); err != nil {
		return nil, err
	}

	rows, err := c.chunithmMusic.ChunithmMusic.
		Query().
		Where(
			chunithmmusic.Or(
				chunithmmusic.ReleaseDateIsNil(),
				chunithmmusic.ReleaseDateLTE(time.Now()),
			),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]types.ChunithmMusicInfo, 0, len(rows))
	for _, row := range rows {
		out = append(out, toChunithmMusicInfo(row))
	}
	return out, nil
}

func (c *Client) GetChunithmMusicBasicInfo(ctx context.Context, musicID int) (*types.ChunithmMusicInfo, error) {
	if err := c.requireChunithm(); err != nil {
		return nil, err
	}
	if musicID <= 0 {
		return nil, ErrInvalidMusicID
	}

	row, err := c.chunithmMusic.ChunithmMusic.
		Query().
		Where(chunithmmusic.MusicIDEQ(musicID)).
		First(ctx)
	if err != nil {
		if entchunimusic.IsNotFound(err) {
			return nil, ErrMusicNotFound
		}
		return nil, err
	}

	resp := toChunithmMusicInfo(row)
	return &resp, nil
}

func (c *Client) GetChunithmMusicDifficultyInfo(ctx context.Context, musicID int, version string) (*types.ChunithmMusicDifficulty, error) {
	if err := c.requireChunithm(); err != nil {
		return nil, err
	}
	if musicID <= 0 {
		return nil, ErrInvalidMusicID
	}

	var row *entchunimusic.ChunithmMusicDifficulty
	var err error
	if version != "" {
		row, err = c.chunithmMusic.ChunithmMusicDifficulty.
			Query().
			Where(
				chunithmmusicdifficulty.MusicIDEQ(musicID),
				chunithmmusicdifficulty.VersionEQ(version),
			).
			First(ctx)
		if err != nil && !entchunimusic.IsNotFound(err) {
			return nil, err
		}
	}

	if row == nil {
		row, err = c.chunithmMusic.ChunithmMusicDifficulty.
			Query().
			Where(chunithmmusicdifficulty.MusicIDEQ(musicID)).
			Order(entchunimusic.Desc(chunithmmusicdifficulty.FieldVersion)).
			First(ctx)
		if err != nil {
			if entchunimusic.IsNotFound(err) {
				return nil, ErrMusicNotFound
			}
			return nil, err
		}
	}

	resp := types.ChunithmMusicDifficulty{
		MusicID: row.MusicID,
		Version: row.Version,
		Diff0:   row.Diff0Const,
		Diff1:   row.Diff1Const,
		Diff2:   row.Diff2Const,
		Diff3:   row.Diff3Const,
		Diff4:   row.Diff4Const,
	}
	return &resp, nil
}

func (c *Client) GetChunithmChartData(ctx context.Context, musicID int) ([]types.ChunithmChartData, error) {
	if err := c.requireChunithm(); err != nil {
		return nil, err
	}
	if musicID <= 0 {
		return nil, ErrInvalidMusicID
	}

	rows, err := c.chunithmMusic.ChunithmChartData.
		Query().
		Where(chunithmchartdata.MusicIDEQ(musicID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrMusicNotFound
	}

	out := make([]types.ChunithmChartData, 0, len(rows))
	for _, row := range rows {
		out = append(out, types.ChunithmChartData{
			Difficulty: row.Difficulty,
			Creator:    row.Creator,
			BPM:        row.Bpm,
			TapCount:   row.TapCount,
			HoldCount:  row.HoldCount,
			SlideCount: row.SlideCount,
			AirCount:   row.AirCount,
			FlickCount: row.FlickCount,
			TotalCount: row.TotalCount,
		})
	}
	return out, nil
}

func (c *Client) QueryChunithmMusicDataBatch(ctx context.Context, musicIDs []int, version string) (map[int]types.ChunithmMusicBatchItem, error) {
	if err := c.requireChunithm(); err != nil {
		return nil, err
	}
	if len(musicIDs) == 0 {
		return map[int]types.ChunithmMusicBatchItem{}, nil
	}

	musicRows, err := c.chunithmMusic.ChunithmMusic.
		Query().
		Where(chunithmmusic.MusicIDIn(musicIDs...)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	musicMap := make(map[int]*entchunimusic.ChunithmMusic, len(musicRows))
	for _, row := range musicRows {
		musicMap[row.MusicID] = row
	}

	diffRows, err := c.chunithmMusic.ChunithmMusicDifficulty.
		Query().
		Where(chunithmmusicdifficulty.MusicIDIn(musicIDs...)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	sort.Slice(diffRows, func(i, j int) bool {
		return diffRows[i].Version > diffRows[j].Version
	})
	diffMap := make(map[int]*entchunimusic.ChunithmMusicDifficulty, len(diffRows))
	for _, row := range diffRows {
		if _, ok := diffMap[row.MusicID]; !ok || row.Version == version {
			diffMap[row.MusicID] = row
		}
	}

	out := make(map[int]types.ChunithmMusicBatchItem, len(musicIDs))
	for _, musicID := range musicIDs {
		musicRow := musicMap[musicID]
		diffRow := diffMap[musicID]

		item := types.ChunithmMusicBatchItem{
			Difficulty: []*float64{nil, nil, nil, nil, nil},
			Info:       unknownMusicInfo(musicID),
		}
		if musicRow != nil {
			item.Info = toChunithmMusicInfo(musicRow)
			item.Version = musicRow.Version
		}
		if diffRow != nil {
			item.Difficulty = []*float64{
				diffRow.Diff0Const,
				diffRow.Diff1Const,
				diffRow.Diff2Const,
				diffRow.Diff3Const,
				diffRow.Diff4Const,
			}
		}

		out[musicID] = item
	}
	return out, nil
}

func (c *Client) GetChunithmDefaultServer(ctx context.Context, harukiUserID int) (*types.ChunithmDefaultServer, error) {
	if err := c.requireChunithm(); err != nil {
		return nil, err
	}
	if harukiUserID <= 0 {
		return nil, ErrInvalidUserID
	}

	row, err := c.chunithmMain.ChunithmDefaultServer.
		Query().
		Where(chunithmdefaultserver.HarukiUserIDEQ(harukiUserID)).
		First(ctx)
	if err != nil {
		return nil, ErrBindingNotFound
	}

	resp := types.ChunithmDefaultServer{
		UserID: row.HarukiUserID,
		Server: row.Server,
	}
	return &resp, nil
}

func (c *Client) GetChunithmBinding(ctx context.Context, harukiUserID int, server string) (*types.ChunithmBinding, error) {
	if err := c.requireChunithm(); err != nil {
		return nil, err
	}
	if harukiUserID <= 0 {
		return nil, ErrInvalidUserID
	}
	if _, err := types.ParseBindingServer(server); err != nil {
		return nil, err
	}

	row, err := c.chunithmMain.ChunithmBinding.
		Query().
		Where(
			chunithmbinding.HarukiUserIDEQ(harukiUserID),
			chunithmbinding.ServerEQ(server),
		).
		First(ctx)
	if err != nil {
		return nil, ErrBindingNotFound
	}

	resp := types.ChunithmBinding{
		UserID: row.HarukiUserID,
		Server: &row.Server,
		AimeID: &row.AimeID,
	}
	return &resp, nil
}

func isValidAlias(alias string) bool {
	if alias == "" {
		return false
	}
	return utf8.RuneCountInString(alias) <= types.MaxAliasLength
}

func toChunithmMusicInfo(row *entchunimusic.ChunithmMusic) types.ChunithmMusicInfo {
	deleted := row.IsDeleted
	return types.ChunithmMusicInfo{
		MusicID:        row.MusicID,
		Title:          row.Title,
		Artist:         row.Artist,
		Category:       row.Category,
		Version:        row.Version,
		ReleaseDate:    row.ReleaseDate,
		IsDeleted:      &deleted,
		DeletedVersion: row.DeletedVersion,
	}
}

func unknownMusicInfo(musicID int) types.ChunithmMusicInfo {
	title := "Unknown"
	artist := "Unknown"
	category := "Unknown"
	deleted := false
	return types.ChunithmMusicInfo{
		MusicID:   musicID,
		Title:     title,
		Artist:    artist,
		Category:  &category,
		IsDeleted: &deleted,
	}
}
