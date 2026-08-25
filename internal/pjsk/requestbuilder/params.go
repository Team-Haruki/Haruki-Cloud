package requestbuilder

import (
	"fmt"

	"haruki-cloud/utils/logger"

	json "haruki-cloud/internal/jsonutil"
)

// MergeParams unmarshals the JSON params into target. Absent or invalid
// params are treated as no-op so prefilled fields stay intact.
// Exported so handler/ can call the same implementation.
func MergeParams(params json.RawMessage, target any) {
	if len(params) == 0 {
		return
	}
	if err := json.Unmarshal(params, target); err != nil {
		logger.Warn("command parameters decode failed",
			"target_type", fmt.Sprintf("%T", target),
			"raw_bytes", len(params),
			"error_type", fmt.Sprintf("%T", err),
		)
	}
}
