package meta

// regionURLs maps SekaiServerRegion strings to their remote music_metas.json URLs.
// Source: sekai-data.3-3.dev (community-maintained, updated alongside game releases).
// Note: "tw" region uses the "-tc" suffix in the filename.
var regionURLs = map[string]string{
	"jp": "https://sekai-data.3-3.dev/music_metas.json",
	"en": "https://sekai-data.3-3.dev/music_metas-en.json",
	"tw": "https://sekai-data.3-3.dev/music_metas-tc.json",
	"kr": "https://sekai-data.3-3.dev/music_metas-kr.json",
	"cn": "https://sekai-data.3-3.dev/music_metas-cn.json",
}

var regionFilenames = map[string]string{
	"jp": "music_metas.json",
	"en": "music_metas-en.json",
	"tw": "music_metas-tc.json",
	"kr": "music_metas-kr.json",
	"cn": "music_metas-cn.json",
}

// Regions returns all supported region keys in a stable order.
func Regions() []string {
	return []string{"jp", "en", "tw", "kr", "cn"}
}
