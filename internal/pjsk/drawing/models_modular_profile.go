package drawing

const ModularProfileRequestKind = "pjsk_modular_profile"

const ModularProfileRenderVersion = 1

type ModularProfileGrid struct {
	Columns   int `json:"columns"`
	RowHeight int `json:"row_height"`
	Gutter    int `json:"gutter"`
	Padding   int `json:"padding"`
}

type ModularProfileWidgetFrame struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`
}

type ModularProfileWidget struct {
	ID      string                    `json:"id"`
	Type    string                    `json:"type"`
	Family  string                    `json:"family"`
	Title   *string                   `json:"title,omitempty"`
	Frame   ModularProfileWidgetFrame `json:"frame"`
	Options map[string]any            `json:"options,omitempty"`
	Data    map[string]any            `json:"data,omitempty"`
}

type ModularProfilePreset struct {
	ID      string                 `json:"id"`
	Name    string                 `json:"name"`
	Source  string                 `json:"source"`
	Theme   map[string]any         `json:"theme,omitempty"`
	Grid    ModularProfileGrid     `json:"grid"`
	Widgets []ModularProfileWidget `json:"widgets"`
}

type ModularProfileRenderRequest struct {
	SchemaVersion  int                    `json:"schema_version"`
	RenderVersion  int                    `json:"render_version,omitempty"`
	Kind           string                 `json:"kind"`
	Region         string                 `json:"region"`
	Profile        BasicProfile           `json:"profile"`
	DataSources    []ProfileDataSource    `json:"data_sources"`
	BgSettings     *ProfileBgSettings     `json:"bg_settings,omitempty"`
	Preset         ModularProfilePreset   `json:"preset"`
	ProfileContext CustomProfileContext   `json:"profile_context"`
	Resources      CustomProfileResources `json:"resources,omitempty"`
}
