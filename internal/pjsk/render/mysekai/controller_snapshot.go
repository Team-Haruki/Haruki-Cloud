package mysekai

import (
	"encoding/json"
	"fmt"
	"time"

	renderregion "haruki-cloud/internal/pjsk/render/region"
	"haruki-cloud/utils/drawing"
)

func (c *Controller) ResolvePhoto(query PhotoQuery) (*PhotoResult, error) {
	if query.Seq == 0 {
		return nil, fmt.Errorf("请输入正确的照片编号（从1或-1开始）")
	}

	merged, region, err := c.prepareSnapshotOnly(query.Region)
	if err != nil {
		return nil, err
	}

	photos := nestedList(merged, "userMysekaiPhotos")
	if len(photos) == 0 {
		return nil, fmt.Errorf("当前账号没有可用的 MySekai 照片数据")
	}

	seq := query.Seq
	if seq < 0 {
		seq = len(photos) + seq + 1
	}
	if seq < 1 {
		return nil, fmt.Errorf("照片编号超出范围（当前共有%d张）", len(photos))
	}
	if seq > len(photos) {
		return nil, fmt.Errorf("照片编号大于照片数量(%d)", len(photos))
	}

	photo, ok := photos[seq-1].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("照片数据格式错误")
	}

	imagePath := stringValue(photo["imagePath"])
	if imagePath == "" {
		return nil, fmt.Errorf("该照片缺少 imagePath，无法下载")
	}

	result := &PhotoResult{
		Region:    region.String(),
		Seq:       seq,
		Total:     len(photos),
		ImagePath: imagePath,
	}
	if obtainedAt := int64Number(photo["obtainedAt"], 0); obtainedAt > 0 {
		result.ObtainedAt = time.UnixMilli(obtainedAt)
	}
	return result, nil
}

func (c *Controller) ensure() error {
	if err := c.ensureSnapshot(); err != nil {
		return err
	}
	if c.masterdata == nil || !c.masterdata.Configured() {
		return fmt.Errorf("mysekai masterdata is not configured")
	}
	return nil
}

func (c *Controller) ensureSnapshot() error {
	if c == nil {
		return fmt.Errorf("mysekai controller is not initialized")
	}
	// Direct mysekai JSON takes priority (no suite data required).
	if len(c.rawMySekaiJSON) > 0 {
		return nil
	}
	if c.snapshot == nil {
		return fmt.Errorf("user snapshot is not available (bind Toolbox or provide snapshot)")
	}
	if err := c.snapshot.Require(); err != nil {
		return err
	}
	return nil
}

func (c *Controller) resolveRegion(region string) renderregion.Value {
	normalized := renderregion.Normalize(region)
	if !normalized.IsZero() {
		return normalized
	}
	if !c.defaultRegion.IsZero() {
		return c.defaultRegion
	}
	return renderregion.JP
}

func (c *Controller) prepareSnapshot(region string) (map[string]any, renderregion.Value, error) {
	if err := c.ensure(); err != nil {
		return nil, renderregion.Unknown, err
	}
	return c.decodeSnapshot(region)
}

func (c *Controller) prepareSnapshotOnly(region string) (map[string]any, renderregion.Value, error) {
	if err := c.ensureSnapshot(); err != nil {
		return nil, renderregion.Unknown, err
	}
	return c.decodeSnapshot(region)
}

func (c *Controller) decodeSnapshot(region string) (map[string]any, renderregion.Value, error) {
	var rawBytes []byte
	var err error

	if len(c.rawMySekaiJSON) > 0 {
		rawBytes = c.rawMySekaiJSON
	} else {
		rawBytes, err = c.snapshot.RawBytes()
		if err != nil {
			return nil, renderregion.Unknown, err
		}
	}

	var merged map[string]any
	if err := json.Unmarshal(rawBytes, &merged); err != nil {
		return nil, renderregion.Unknown, fmt.Errorf("decode mysekai data: %w", err)
	}

	// When using raw mysekai JSON directly (not merged via userdata.Service),
	// flatten updatedResources so that keys like userMysekaiFixtures are
	// accessible at the top level, matching the merged-snapshot layout.
	if len(c.rawMySekaiJSON) > 0 {
		if updated, ok := merged["updatedResources"].(map[string]any); ok {
			for key, value := range updated {
				merged[key] = value
			}
		}
	}

	return merged, c.resolveRegion(region), nil
}

func (c *Controller) mysekaiProfileCard(region renderregion.Value, merged map[string]any, override *drawing.ProfileCardRequest) *drawing.ProfileCardRequest {
	var profile *drawing.ProfileCardRequest
	if override != nil {
		cloned := *override
		if override.Profile != nil {
			basic := *override.Profile
			cloned.Profile = &basic
		}
		if len(override.DataSources) > 0 {
			cloned.DataSources = append([]drawing.ProfileDataSource(nil), override.DataSources...)
		}
		profile = &cloned
	} else {
		if c == nil || c.snapshot == nil {
			return nil
		}
		profile = c.snapshot.ProfileCard(region)
	}
	if profile == nil {
		return nil
	}
	if updated, ok := merged["userMysekaiGamedata"].(map[string]any); ok {
		if level := intNumber(updated["mysekaiRank"], 0); level > 0 {
			profile.MysekaiLevel = &level
		}
	}
	return profile
}
