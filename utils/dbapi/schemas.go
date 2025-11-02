package dbapi

import "haruki-cloud/utils/model"

// PjskBindingRequest represents a PJSK binding request with optional server parameter
type PjskBindingRequest struct {
	Server *model.SekaiBindingServerRegion `json:"server,omitempty"`
}

// PjskSuiteDataPolicyUpdate represents an update to PJSK suite data policy
type PjskSuiteDataPolicyUpdate struct {
	Configs map[string]interface{} `json:"configs,omitempty"`
}
