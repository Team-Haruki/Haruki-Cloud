package card

type Query struct {
	Query  string `json:"query"`
	Region string `json:"region"`
	UserID string `json:"user_id,omitempty"`
	Mode   string `json:"mode,omitempty"`
}

type ListRequest struct {
	CardIDs []int  `json:"card_ids"`
	Region  string `json:"region"`
}
