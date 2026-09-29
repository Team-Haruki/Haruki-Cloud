package inventory

import (
	"context"
	"math"
	"strconv"
	"time"

	json "haruki-cloud/internal/jsonutil"

	"haruki-cloud/internal/pjsk/displaytime"
	"haruki-cloud/internal/pjsk/drawing"
)

// applyMaterialExpiry drops materials whose master row says they expired
// (materials.expiredAt, JP 7.0.0) and sends the rest with expired_at.
// Regions whose rows carry no expiredAt are returned unchanged.
func (c *Controller) applyMaterialExpiry(items []drawing.InventoryItem, rows MaterialRowSource) []drawing.InventoryItem {
	expiry := materialExpiryByID(c.expiryContext(), rows)
	if len(expiry) == 0 {
		return items
	}
	now := c.clock().UnixMilli()
	kept := items[:0]
	for _, item := range items {
		expiredAt, ok := expiry[item.ID]
		if item.ResourceType != "material" || !ok {
			kept = append(kept, item)
			continue
		}
		if expiredAt <= now {
			continue
		}
		// Drawing renders the expiry line (contract drawing-700 §5).
		item.ExpiredAt = new(expiredAt)
		kept = append(kept, item)
	}
	return kept
}

func materialExpiryByID(ctx context.Context, rows MaterialRowSource) map[int]int64 {
	if rows == nil {
		return nil
	}
	materials, ok := rows.LoadMasterRows(ctx, "materials.json")
	if !ok {
		return nil
	}
	expiry := make(map[int]int64)
	for id, row := range materials {
		if value, ok := int64Number(row["expiredAt"]); ok && value > 0 {
			expiry[id] = displaytime.NormalizeUnixMillis(value)
		}
	}
	return expiry
}

func (c *Controller) expiryContext() context.Context {
	if c != nil && c.requestCtx != nil {
		return c.requestCtx
	}
	return context.Background()
}

func (c *Controller) clock() time.Time {
	if c != nil && c.now != nil {
		return c.now()
	}
	return time.Now()
}

func int64Number(value any) (int64, bool) {
	switch v := value.(type) {
	case int64:
		return v, true
	case int:
		return int64(v), true
	case int32:
		return int64(v), true
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return 0, false
		}
		return int64(v), true
	case json.Number:
		n, err := v.Int64()
		return n, err == nil
	case string:
		n, err := strconv.ParseInt(v, 10, 64)
		return n, err == nil
	default:
		return 0, false
	}
}
