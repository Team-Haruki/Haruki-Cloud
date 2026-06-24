package drawing

// =========================== Inventory Models ===========================

type InventoryItem struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	Description   string `json:"description,omitempty"`
	Category      string `json:"category"`
	ResourceType  string `json:"resource_type"`
	IconPath      string `json:"icon_path"`
	Quantity      int    `json:"quantity"`
	Seq           int    `json:"seq"`
	RecoveryValue *int   `json:"recovery_value,omitempty"`
}

type InventorySection struct {
	Key   string          `json:"key"`
	Title string          `json:"title"`
	Items []InventoryItem `json:"items"`
}

type InventoryListRequest struct {
	Profile    DetailedProfileCardRequest `json:"profile"`
	Sections   []InventorySection         `json:"sections"`
	TotalItems int                        `json:"total_items"`
}
