package accountdata

import (
	"context"
	"strings"

	pjskdb "haruki-cloud/database/pjsk"
	"haruki-cloud/database/pjsk/profilebackground"
	"haruki-cloud/internal/pjsk/drawing"
)

func profileBackgroundKey(server, userID string) string {
	return strings.TrimSpace(strings.ToLower(server)) + "\x00" + strings.TrimSpace(userID)
}

func resolveBindingProfileBG(bgMap map[string]*drawing.ProfileBgSettings, binding *pjskdb.UserBinding) *drawing.ProfileBgSettings {
	if binding == nil {
		return nil
	}
	if bgMap != nil {
		if bg := bgMap[profileBackgroundKey(binding.Server, binding.UserID)]; bg != nil {
			if !hasCustomProfileBGImage(bg) {
				return nil
			}
			return cloneProfileBGSettings(bg)
		}
	}
	return nil
}

func hasCustomProfileBGImage(bg *drawing.ProfileBgSettings) bool {
	return bg != nil && bg.ImgPath != nil && strings.TrimSpace(*bg.ImgPath) != ""
}

func sameProfileBGPath(left, right *drawing.ProfileBgSettings) bool {
	if !hasCustomProfileBGImage(left) || !hasCustomProfileBGImage(right) {
		return false
	}
	return strings.TrimSpace(*left.ImgPath) == strings.TrimSpace(*right.ImgPath)
}

func mergeUploadedProfileBGSettings(current, uploaded *drawing.ProfileBgSettings) *drawing.ProfileBgSettings {
	if current == nil {
		return cloneProfileBGSettings(uploaded)
	}
	merged := cloneProfileBGSettings(current)
	if uploaded != nil && uploaded.ImgPath != nil {
		path := strings.TrimSpace(*uploaded.ImgPath)
		merged.ImgPath = &path
	}
	return merged
}

func clearProfileBGImagePath(current *drawing.ProfileBgSettings) *drawing.ProfileBgSettings {
	if current == nil {
		return nil
	}
	cleared := cloneProfileBGSettings(current)
	cleared.ImgPath = nil
	return cleared
}

func loadProfileBackground(ctx context.Context, db *pjskdb.Client, server, userID string) (*drawing.ProfileBgSettings, error) {
	if db == nil {
		return nil, nil
	}
	row, err := db.ProfileBackground.Query().
		Where(
			profilebackground.ServerEQ(strings.TrimSpace(strings.ToLower(server))),
			profilebackground.UserIDEQ(strings.TrimSpace(userID)),
		).
		Only(ctx)
	if err != nil {
		if pjskdb.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return cloneProfileBGSettings(row.Bg), nil
}

func loadProfileBackgroundMap(ctx context.Context, db *pjskdb.Client, bindings []*pjskdb.UserBinding) (map[string]*drawing.ProfileBgSettings, error) {
	if db == nil || len(bindings) == 0 {
		return nil, nil
	}

	serverSet := make(map[string]struct{}, len(bindings))
	userIDSet := make(map[string]struct{}, len(bindings))
	bindingKeys := make(map[string]struct{}, len(bindings))
	for _, binding := range bindings {
		if binding == nil {
			continue
		}
		server := strings.TrimSpace(strings.ToLower(binding.Server))
		userID := strings.TrimSpace(binding.UserID)
		if server == "" || userID == "" {
			continue
		}
		serverSet[server] = struct{}{}
		userIDSet[userID] = struct{}{}
		bindingKeys[profileBackgroundKey(server, userID)] = struct{}{}
	}
	if len(bindingKeys) == 0 {
		return nil, nil
	}

	servers := make([]string, 0, len(serverSet))
	for server := range serverSet {
		servers = append(servers, server)
	}
	userIDs := make([]string, 0, len(userIDSet))
	for userID := range userIDSet {
		userIDs = append(userIDs, userID)
	}

	rows, err := db.ProfileBackground.Query().
		Where(
			profilebackground.ServerIn(servers...),
			profilebackground.UserIDIn(userIDs...),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}

	result := make(map[string]*drawing.ProfileBgSettings, len(rows))
	for _, row := range rows {
		if row == nil {
			continue
		}
		key := profileBackgroundKey(row.Server, row.UserID)
		if _, ok := bindingKeys[key]; !ok {
			continue
		}
		result[key] = cloneProfileBGSettings(row.Bg)
	}
	return result, nil
}

func upsertProfileBackground(ctx context.Context, db *pjskdb.Client, server, userID string, settings *drawing.ProfileBgSettings) error {
	if db == nil || settings == nil {
		return nil
	}

	server = strings.TrimSpace(strings.ToLower(server))
	userID = strings.TrimSpace(userID)
	existing, err := db.ProfileBackground.Query().
		Where(
			profilebackground.ServerEQ(server),
			profilebackground.UserIDEQ(userID),
		).
		Only(ctx)
	switch {
	case err == nil:
		_, err = db.ProfileBackground.UpdateOneID(existing.ID).
			SetBg(cloneProfileBGSettings(settings)).
			Save(ctx)
		return err
	case pjskdb.IsNotFound(err):
		_, err = db.ProfileBackground.Create().
			SetServer(server).
			SetUserID(userID).
			SetBg(cloneProfileBGSettings(settings)).
			Save(ctx)
		return err
	default:
		return err
	}
}

func deleteProfileBackground(ctx context.Context, db *pjskdb.Client, server, userID string) error {
	if db == nil {
		return nil
	}
	_, err := db.ProfileBackground.Delete().
		Where(
			profilebackground.ServerEQ(strings.TrimSpace(strings.ToLower(server))),
			profilebackground.UserIDEQ(strings.TrimSpace(userID)),
		).
		Exec(ctx)
	return err
}
