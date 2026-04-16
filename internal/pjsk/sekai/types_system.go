package sekai

// GetSystemResponse is the response returned by GetSystem.
type GetSystemResponse struct {
	ServerDate        int64            `json:"serverDate"`
	Timezone          string           `json:"timezone"`
	Profile           string           `json:"profile"`
	MaintenanceStatus string           `json:"maintenanceStatus"`
	AppVersions       []AppVersionInfo `json:"appVersions"`
}

// AppVersionInfo describes one client version entry in the system response.
type AppVersionInfo struct {
	SystemProfile    string `json:"systemProfile"`
	AppVersion       string `json:"appVersion"`
	MultiPlayVersion string `json:"multiPlayVersion"`
	AssetVersion     string `json:"assetVersion"`
	AppVersionStatus string `json:"appVersionStatus"`
}
