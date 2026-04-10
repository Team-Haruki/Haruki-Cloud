package requestbuilder

import (
	"encoding/json"

	"haruki-cloud/utils/logger"
)

// mergeParams keeps the bridge-side behavior: absent or invalid params are
// treated as no-op so prefilled fields stay intact.
func mergeParams(params json.RawMessage, target interface{}) {
	if len(params) == 0 {
		return
	}
	if err := json.Unmarshal(params, target); err != nil {
		logger.Warnf("requestbuilder: failed to parse params into %T: %v (raw_len=%d)", target, err, len(params))
	}
}
