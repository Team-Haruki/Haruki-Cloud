package provider

// MySekaiProvider exposes MySekai masterdata in a generic key-value form.
// The underlying data may come from JSON files or a database, but callers
// interact uniformly via filename keys (e.g. "mysekaiFixtures.json").
type MySekaiProvider interface {
	Configured() bool
	LoadList(filename string) []map[string]any
	LoadMapByID(filename string) map[int]map[string]any
	LoadObject(filename string, target any) bool
}
