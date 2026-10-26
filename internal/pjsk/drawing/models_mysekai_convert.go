package drawing

type MysekaiConvertResource struct {
	Name         string `json:"name"`
	ImagePath    string `json:"image_path"`
	Quantity     *int   `json:"quantity"`
	ResourceType string `json:"resource_type"`
	ID           int    `json:"id"`
}

type MysekaiConvertRecipe struct {
	ID            int                      `json:"id"`
	Name          string                   `json:"name"`
	Category      string                   `json:"category"`
	CategoryKey   string                   `json:"category_key"`
	Inputs        []MysekaiConvertResource `json:"inputs"`
	Outputs       []MysekaiConvertResource `json:"outputs"`
	Duration      string                   `json:"duration"`
	RequiredLevel int                      `json:"required_level"`
	Limits        string                   `json:"limits"`
}

type MysekaiConvertRecipesRequest struct {
	Title   string                 `json:"title"`
	Total   int                    `json:"total"`
	Filter  string                 `json:"filter"`
	Recipes []MysekaiConvertRecipe `json:"recipes"`
	Labels  map[string]string      `json:"labels,omitempty"`
	Text    string                 `json:"-"`
}
