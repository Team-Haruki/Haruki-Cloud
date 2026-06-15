package sk

import (
	"fmt"
	"strconv"
	"strings"

	"haruki-cloud/internal/pjsk/drawing"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
)

func (c *Controller) buildSingleRankFromTracker(server string, eventID, rank int, wlCharacterID *int) (drawing.RankInfo, error) {
	info, uid, ok, err := c.buildSingleRankBaseFromTracker(server, eventID, rank, wlCharacterID)
	if err != nil {
		return drawing.RankInfo{}, err
	}
	c.enrichRankInfoPreferUser(server, eventID, info.Rank, uid, ok, wlCharacterID, &info)
	if c.isTrackerEventTitleName(server, eventID, info.Name) {
		info.Name = fmt.Sprintf("Rank %d", info.Rank)
	}
	return info, nil
}

func (c *Controller) buildSingleRankLatestFromTracker(server string, eventID, rank int, wlCharacterID *int) (drawing.RankInfo, error) {
	info, _, _, err := c.buildSingleRankBaseFromTracker(server, eventID, rank, wlCharacterID)
	return info, err
}

func (c *Controller) buildSingleRankBaseFromTracker(server string, eventID, rank int, wlCharacterID *int) (drawing.RankInfo, int64, bool, error) {
	if wlCharacterID != nil && *wlCharacterID > 0 {
		latest, err := c.tracker.GetLatestWorldBloomRankingByRank(server, eventID, *wlCharacterID, rank)
		if err != nil {
			return drawing.RankInfo{}, 0, false, err
		}
		rankValue := rank
		if latest.RankData.Rank > 0 {
			rankValue = latest.RankData.Rank
		}
		score := latest.RankData.Score
		name := strings.TrimSpace(latest.UserData.Name)
		userID := strings.TrimSpace(latest.RankData.UserID)
		if userID == "" {
			userID = strings.TrimSpace(latest.UserData.UserID)
		}
		if rankValue <= 100 || c.shouldResolveTrackerNameByUser(server, eventID, name) {
			if resolved := c.resolveTrackerNameByUserID(server, eventID, userID, wlCharacterID); strings.TrimSpace(resolved) != "" {
				name = resolved
			}
		}
		info := drawing.RankInfo{
			Rank:  rankValue,
			Name:  pickTrackerDisplayName(c.censorTrackerName(name, server), rankValue),
			Score: drawing.IntPtr(score),
			Time:  formatTrackerTimestamp(latest.RankData.Timestamp),
		}
		uid, ok := parseTrackerUserID(latest.RankData.UserID, latest.UserData.UserID)
		return info, uid, ok, nil
	}

	latest, err := c.tracker.GetLatestRankingByRank(server, eventID, rank)
	if err != nil {
		return drawing.RankInfo{}, 0, false, err
	}
	rankValue := rank
	if latest.RankData.Rank > 0 {
		rankValue = latest.RankData.Rank
	}
	score := latest.RankData.Score
	name := strings.TrimSpace(latest.UserData.Name)
	userID := strings.TrimSpace(latest.RankData.UserID)
	if userID == "" {
		userID = strings.TrimSpace(latest.UserData.UserID)
	}
	if rankValue <= 100 || c.shouldResolveTrackerNameByUser(server, eventID, name) {
		if resolved := c.resolveTrackerNameByUserID(server, eventID, userID, wlCharacterID); strings.TrimSpace(resolved) != "" {
			name = resolved
		}
	}
	info := drawing.RankInfo{
		Rank:  rankValue,
		Name:  pickTrackerDisplayName(c.censorTrackerName(name, server), rankValue),
		Score: drawing.IntPtr(score),
		Time:  formatTrackerTimestamp(latest.RankData.Timestamp),
	}
	uid, ok := parseTrackerUserID(latest.RankData.UserID, latest.UserData.UserID)
	return info, uid, ok, nil
}

func (c *Controller) resolveTrackerNameByUserID(server string, eventID int, userID string, wlCharacterID *int) string {
	uid, err := strconv.ParseInt(strings.TrimSpace(userID), 10, 64)
	if err != nil || uid <= 0 {
		return ""
	}
	candidates := make([]string, 0, 2)
	userData, userErr := c.tracker.GetUserEventData(server, eventID, uid)
	if userErr == nil && userData != nil {
		candidates = append(candidates, strings.TrimSpace(userData.Name))
	}
	if wlCharacterID != nil && *wlCharacterID > 0 {
		latest, latestErr := c.tracker.GetLatestWorldBloomRankingByUser(server, eventID, *wlCharacterID, uid)
		if latestErr == nil && latest != nil {
			candidates = append(candidates, strings.TrimSpace(latest.UserData.Name))
		}
		return c.pickTrackerResolvedName(server, eventID, candidates...)
	}
	latest, latestErr := c.tracker.GetLatestRankingByUser(server, eventID, uid)
	if latestErr == nil && latest != nil {
		candidates = append(candidates, strings.TrimSpace(latest.UserData.Name))
	}
	return c.pickTrackerResolvedName(server, eventID, candidates...)
}

func (c *Controller) buildSingleUserFromTracker(server string, eventID int, userID int64, wlCharacterID *int) (drawing.RankInfo, error) {
	info, err := c.buildSingleUserBaseFromTracker(server, eventID, userID, wlCharacterID)
	if err != nil {
		return drawing.RankInfo{}, err
	}
	c.enrichRankInfoByUser(server, eventID, userID, wlCharacterID, &info)
	return info, nil
}

func (c *Controller) buildSingleUserLatestFromTracker(server string, eventID int, userID int64, wlCharacterID *int) (drawing.RankInfo, error) {
	return c.buildSingleUserBaseFromTracker(server, eventID, userID, wlCharacterID)
}

func (c *Controller) buildSingleUserBaseFromTracker(server string, eventID int, userID int64, wlCharacterID *int) (drawing.RankInfo, error) {
	if wlCharacterID != nil && *wlCharacterID > 0 {
		latest, err := c.tracker.GetLatestWorldBloomRankingByUser(server, eventID, *wlCharacterID, userID)
		if err != nil {
			return drawing.RankInfo{}, err
		}
		if latest == nil || latest.RankData.Rank <= 0 {
			return drawing.RankInfo{}, sekaiapi.ErrRankingNotFound
		}
		rankValue := latest.RankData.Rank
		score := latest.RankData.Score
		name := strings.TrimSpace(latest.UserData.Name)
		if c.shouldResolveTrackerNameByUser(server, eventID, name) {
			lookupUserID := strings.TrimSpace(latest.RankData.UserID)
			if lookupUserID == "" {
				lookupUserID = strconv.FormatInt(userID, 10)
			}
			if resolved := c.resolveTrackerNameByUserID(server, eventID, lookupUserID, wlCharacterID); strings.TrimSpace(resolved) != "" {
				name = resolved
			}
		}
		info := drawing.RankInfo{
			Rank:  rankValue,
			Name:  pickTrackerDisplayName(c.censorTrackerName(name, server), rankValue),
			Score: drawing.IntPtr(score),
			Time:  formatTrackerTimestamp(latest.RankData.Timestamp),
		}
		if c.isTrackerEventTitleName(server, eventID, info.Name) {
			info.Name = fmt.Sprintf("Rank %d", info.Rank)
		}
		return info, nil
	}

	latest, err := c.tracker.GetLatestRankingByUser(server, eventID, userID)
	if err != nil {
		return drawing.RankInfo{}, err
	}
	if latest == nil || latest.RankData.Rank <= 0 {
		return drawing.RankInfo{}, sekaiapi.ErrRankingNotFound
	}
	rankValue := latest.RankData.Rank
	score := latest.RankData.Score
	name := strings.TrimSpace(latest.UserData.Name)
	if c.shouldResolveTrackerNameByUser(server, eventID, name) {
		lookupUserID := strings.TrimSpace(latest.RankData.UserID)
		if lookupUserID == "" {
			lookupUserID = strconv.FormatInt(userID, 10)
		}
		if resolved := c.resolveTrackerNameByUserID(server, eventID, lookupUserID, wlCharacterID); strings.TrimSpace(resolved) != "" {
			name = resolved
		}
	}
	info := drawing.RankInfo{
		Rank:  rankValue,
		Name:  pickTrackerDisplayName(c.censorTrackerName(name, server), rankValue),
		Score: drawing.IntPtr(score),
		Time:  formatTrackerTimestamp(latest.RankData.Timestamp),
	}
	if c.isTrackerEventTitleName(server, eventID, info.Name) {
		info.Name = fmt.Sprintf("Rank %d", info.Rank)
	}
	return info, nil
}

func parseTrackerUserID(userIDs ...string) (int64, bool) {
	for _, raw := range userIDs {
		uid, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
		if err == nil && uid > 0 {
			return uid, true
		}
	}
	return 0, false
}
