package requestbuilder

import "encoding/json"

// mergeParams keeps the bridge-side behavior: absent or invalid params are
// treated as no-op so prefilled fields stay intact.
func mergeParams(params json.RawMessage, target interface{}) {
	if len(params) == 0 {
		return
	}
	_ = json.Unmarshal(params, target)
}
