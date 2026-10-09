package mysekai

import (
	"bytes"
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"haruki-cloud/internal/i18n"
	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/common"
	"haruki-cloud/internal/pjsk/render/snapshot"
	"haruki-cloud/utils/usererror"
)

func (c *Controller) ResolvePhoto(query PhotoQuery) (*PhotoResult, error) {
	c = c.withRegion(query.Region)
	if query.Seq == 0 {
		return nil, usererror.Misuse(i18n.M("mysekai.photo.index_invalid"))
	}

	merged, region, err := c.prepareSnapshotOnly(query.Region)
	if err != nil {
		return nil, err
	}

	photos := nestedList(merged, "userMysekaiPhotos")
	if len(photos) == 0 {
		return nil, usererror.New(usererror.CodeNotFound, i18n.M("mysekai.photo.none"))
	}

	seq := query.Seq
	if seq < 0 {
		seq = len(photos) + seq + 1
	}
	if seq < 1 {
		return nil, usererror.New(usererror.CodeOutOfRange, i18n.M("mysekai.photo.out_of_range", i18n.Data{"Count": len(photos)}))
	}
	if seq > len(photos) {
		return nil, usererror.New(usererror.CodeOutOfRange, i18n.M("mysekai.photo.out_of_range", i18n.Data{"Count": len(photos)}))
	}

	photo, ok := photos[seq-1].(map[string]any)
	if !ok {
		return nil, usererror.Wrap(usererror.CodeUnavailable, i18n.M("mysekai.photo.invalid"), errors.New("photo entry is not an object"))
	}

	imagePath := stringValue(photo["imagePath"])
	if imagePath == "" {
		return nil, usererror.Wrap(usererror.CodeUnavailable, i18n.M("mysekai.photo.invalid"), errors.New("photo has no imagePath"))
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
	return c.ensureMasterdata()
}

func (c *Controller) ensureMasterdata() error {
	if c == nil {
		return usererror.Misconfigured(errors.New("mysekai controller is not initialized"))
	}
	if c.masterdata == nil || !c.masterdata.Configured() {
		return usererror.Misconfigured(errors.New("mysekai masterdata is not configured"))
	}
	return nil
}

func (c *Controller) ensureSnapshot() error {
	if c == nil {
		return usererror.Misconfigured(errors.New("mysekai controller is not initialized"))
	}
	// Direct mysekai JSON takes priority (no suite data required).
	if len(c.rawMySekaiJSON) > 0 {
		return nil
	}
	if c.snapshot == nil {
		return snapshot.ErrMySekaiUnavailable
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

func (c *Controller) SnapshotExpired(region string) (bool, error) {
	status, err := c.SnapshotStatus(region, time.Now())
	if err != nil {
		return false, err
	}
	return status.Expired, nil
}

// SnapshotStatus reads only the snapshot's timestamps. It decodes just the
// members the expiry check uses instead of the whole document, falling back
// to the full decode (and its errors) when that is not possible.
func (c *Controller) SnapshotStatus(region string, now time.Time) (SnapshotStatus, error) {
	if err := c.ensureSnapshot(); err != nil {
		return SnapshotStatus{}, err
	}
	rawBytes, err := c.snapshotBytes()
	if err != nil {
		return SnapshotStatus{}, err
	}
	merged, ok := c.decodeSnapshotTimeMembers(rawBytes)
	if !ok {
		if merged, err = c.decodeSnapshotBytes(rawBytes); err != nil {
			return SnapshotStatus{}, err
		}
	}
	resolvedRegion := c.resolveRegion(region)
	if now.IsZero() {
		now = time.Now()
	}

	status := SnapshotStatus{
		Expired: isMysekaiSnapshotExpired(resolvedRegion, merged, now),
	}
	if updateTimeMs := normalizeMySekaiTimestampMs(int64Number(merged["upload_time"], 0)); updateTimeMs > 0 {
		status.LastUpdatedAt = time.UnixMilli(updateTimeMs)
	}
	return status, nil
}

func (c *Controller) decodeSnapshot(region string) (map[string]any, renderregion.Value, error) {
	rawBytes, err := c.snapshotBytes()
	if err != nil {
		return nil, renderregion.Unknown, err
	}
	merged, err := c.decodeSnapshotBytes(rawBytes)
	if err != nil {
		return nil, renderregion.Unknown, err
	}
	return merged, c.resolveRegion(region), nil
}

func (c *Controller) snapshotBytes() ([]byte, error) {
	if len(c.rawMySekaiJSON) > 0 {
		return c.rawMySekaiJSON, nil
	}
	finishCopy := commandtrace.MeasureOperation(c.requestCtx, "mysekai.snapshot_copy")
	defer finishCopy()
	return c.snapshot.RawBytes()
}

// decodeSnapshotTimeMembers returns the map decodeSnapshotBytes would build,
// restricted to the members resolveMysekaiSnapshotTimeMs reads ("now",
// "upload_time" and "now" inside "updatedResources"), with the same values
// and the same raw-MySekai flattening. It streams over the document without
// materializing the rest and reports false whenever the document cannot be
// read that way, so the caller falls back to the full decode and its errors.
func (c *Controller) decodeSnapshotTimeMembers(rawBytes []byte) (map[string]any, bool) {
	finishDecode := commandtrace.MeasureOperation(c.requestCtx, "mysekai.snapshot_status_decode")
	defer finishDecode()
	// Match the full decode's leniency (duplicate names, invalid UTF-8).
	decoder := jsontext.NewDecoder(bytes.NewReader(rawBytes), jsontext.AllowDuplicateNames(true), jsontext.AllowInvalidUTF8(true))
	top, err := readSnapshotTimeFields(decoder, 0)
	if err != nil || !top.isObject {
		return nil, false
	}
	// Flattening copies every member of an updatedResources object to the
	// top level, including a nested updatedResources.
	flatten := len(c.rawMySekaiJSON) > 0 && top.updated != nil && top.updated.isObject
	merged := make(map[string]any, 3)
	for _, key := range []string{"now", "upload_time"} {
		raw, ok := top.values[key]
		if flatten {
			if value, flat := top.updated.values[key]; flat {
				raw, ok = value, true
			}
		}
		if !ok {
			continue
		}
		var value any
		if decodeJSONUseNumber(raw, &value) != nil {
			return nil, false
		}
		merged[key] = value
	}
	updated := top.updated
	if flatten && top.updated.updated != nil {
		updated = top.updated.updated
	}
	if updated != nil && updated.isObject {
		inner := map[string]any{}
		if raw, ok := updated.values["now"]; ok {
			var now any
			if decodeJSONUseNumber(raw, &now) != nil {
				return nil, false
			}
			inner["now"] = now
		}
		merged["updatedResources"] = inner
	}
	return merged, true
}

// snapshotTimeFields holds the raw timestamp members of one JSON value; only
// objects have members. Later duplicates win, as in the full decode.
type snapshotTimeFields struct {
	isObject bool
	values   map[string]jsontext.Value
	updated  *snapshotTimeFields
}

// readSnapshotTimeFields reads one value, keeping "now" and "upload_time"
// and descending into "updatedResources" for two levels; everything else is
// skipped without being decoded.
func readSnapshotTimeFields(decoder *jsontext.Decoder, depth int) (snapshotTimeFields, error) {
	if decoder.PeekKind() != '{' {
		return snapshotTimeFields{}, decoder.SkipValue()
	}
	if _, err := decoder.ReadToken(); err != nil {
		return snapshotTimeFields{}, err
	}
	fields := snapshotTimeFields{isObject: true, values: map[string]jsontext.Value{}}
	for decoder.PeekKind() != '}' {
		name, err := decoder.ReadToken()
		if err != nil {
			return snapshotTimeFields{}, err
		}
		switch key := name.String(); {
		case key == "now" || key == "upload_time":
			value, err := decoder.ReadValue()
			if err != nil {
				return snapshotTimeFields{}, err
			}
			fields.values[key] = value.Clone()
		case key == "updatedResources" && depth < 2:
			nested, err := readSnapshotTimeFields(decoder, depth+1)
			if err != nil {
				return snapshotTimeFields{}, err
			}
			fields.updated = &nested
		default:
			if err := decoder.SkipValue(); err != nil {
				return snapshotTimeFields{}, err
			}
		}
	}
	_, err := decoder.ReadToken()
	return fields, err
}

// decodeSnapshotBytes decodes the whole snapshot document.
func (c *Controller) decodeSnapshotBytes(rawBytes []byte) (map[string]any, error) {
	var merged map[string]any
	finishDecode := commandtrace.MeasureOperation(c.requestCtx, "mysekai.snapshot_decode")
	err := decodeJSONUseNumber(rawBytes, &merged)
	finishDecode()
	if err != nil {
		return nil, usererror.Wrap(usererror.CodeSetup, i18n.M("mysekai.data_invalid"), fmt.Errorf("decode mysekai data: %w", err))
	}

	// When using raw mysekai JSON directly (not merged via userdata.Service),
	// flatten updatedResources so that keys like userMysekaiFixtures are
	// accessible at the top level, matching the merged-snapshot layout.
	if len(c.rawMySekaiJSON) > 0 {
		finishFlatten := commandtrace.MeasureOperation(c.requestCtx, "mysekai.snapshot_flatten")
		if updated, ok := merged["updatedResources"].(map[string]any); ok {
			for key, value := range updated {
				merged[key] = value
			}
		}
		finishFlatten()
	}
	return merged, nil
}

func (c *Controller) mysekaiProfileCard(region renderregion.Value, merged map[string]any, override *drawing.ProfileCardRequest, includeSuite bool) *drawing.ProfileCardRequest {
	var profile *drawing.ProfileCardRequest
	usesRawMySekaiOnly := len(c.rawMySekaiJSON) > 0
	if override != nil {
		cloned := *override
		if override.Profile != nil {
			cloned.Profile = new(*override.Profile)
		}
		if len(override.DataSources) > 0 {
			cloned.DataSources = slices.Clone(override.DataSources)
		}
		cloned.Rank = common.CloneIntPtr(override.Rank)
		profile = &cloned
	} else {
		if c.snapshot == nil {
			return nil
		}
		profile = c.snapshot.ProfileCard(region)
	}
	if profile == nil {
		return nil
	}
	if includeSuite {
		mergeMySekaiDataSources(profile, merged, usesRawMySekaiOnly)
	} else {
		replaceWithMySekaiDataSource(profile, merged)
	}
	stripProfileDataSourceDetails(profile)
	if updated, ok := merged["userMysekaiGamedata"].(map[string]any); ok {
		if level := intNumber(updated["mysekaiRank"], 0); level > 0 {
			profile.MysekaiLevel = &level
		}
	}
	return profile
}

func replaceWithMySekaiDataSource(profile *drawing.ProfileCardRequest, merged map[string]any) {
	if profile == nil {
		return
	}
	entry, ok := mysekaiDataSourceFromMerged(profile, merged)
	if ok {
		profile.DataSources = []drawing.ProfileDataSource{entry}
		return
	}
	profile.DataSources = []drawing.ProfileDataSource{common.NewDataSource(drawing.DataSourceMySekai)}
}

func stripProfileDataSourceDetails(profile *drawing.ProfileCardRequest) {
	if profile == nil {
		return
	}
	for i := range profile.DataSources {
		profile.DataSources[i].Source = nil
		profile.DataSources[i].Mode = nil
	}
}

func mergeMySekaiDataSources(profile *drawing.ProfileCardRequest, merged map[string]any, replaceSingle bool) {
	if profile == nil || merged == nil {
		return
	}
	entry, ok := mysekaiDataSourceFromMerged(profile, merged)
	if !ok {
		if replaceSingle && len(profile.DataSources) == 1 {
			profile.DataSources[0].Name = mySekaiDataLabel()
			profile.DataSources[0].Kind = drawing.DataSourceMySekai
		}
		return
	}

	for i := range profile.DataSources {
		if isMySekaiDataSource(profile.DataSources[i]) {
			profile.DataSources[i] = entry
			return
		}
	}

	if replaceSingle && len(profile.DataSources) == 1 {
		profile.DataSources[0] = entry
		return
	}
	profile.DataSources = append(profile.DataSources, entry)
}

func mysekaiDataSourceFromMerged(profile *drawing.ProfileCardRequest, merged map[string]any) (drawing.ProfileDataSource, bool) {
	updateTime := normalizeMySekaiTimestampMs(int64Number(merged["upload_time"], 0))
	sourceValue := strings.TrimSpace(stringValue(merged["source"]))
	localSource := strings.TrimSpace(stringValue(merged["local_source"]))
	if updateTime == 0 && sourceValue == "" && localSource == "" {
		return drawing.ProfileDataSource{}, false
	}
	if localSource != "" {
		if sourceValue != "" {
			sourceValue += "(" + localSource + ")"
		} else {
			sourceValue = localSource
		}
	}

	entry := common.NewDataSource(drawing.DataSourceMySekai)
	if updateTime > 0 {
		entry.UpdateTime = &updateTime
	}
	if sourceValue != "" {
		entry.Source = &sourceValue
	}
	if profile != nil && len(profile.DataSources) > 0 && profile.DataSources[0].Mode != nil {
		entry.Mode = new(*profile.DataSources[0].Mode)
	}
	return entry, true
}

func normalizeMySekaiTimestampMs(value int64) int64 {
	if value <= 0 {
		return 0
	}
	// MySekai payloads may report timestamps in seconds while Suite payloads
	// use milliseconds. Normalize all timestamp-like fields to milliseconds.
	if value < 1_000_000_000_000 {
		return value * 1000
	}
	return value
}
