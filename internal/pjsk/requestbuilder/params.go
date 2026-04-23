package requestbuilder

import (
	"encoding/json"
	sonic "github.com/bytedance/sonic"

	"haruki-cloud/utils/logger"
)

// MergeParams unmarshals the JSON params into target. Absent or invalid
// params are treated as no-op so prefilled fields stay intact.
// Exported so handler/ can call the same implementation.
func MergeParams(params json.RawMessage, target any) {
	if len(params) == 0 {
		return
	}
	if err := sonic.Unmarshal(params, target); err != nil {
		logger.Warnf("pjsk: failed to parse command params into %T: %v (raw_len=%d)", target, err, len(params))
	}
}
