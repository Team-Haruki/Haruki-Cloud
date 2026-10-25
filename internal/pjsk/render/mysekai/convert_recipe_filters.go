package mysekai

import "strings"

func (catalog convertRecipeCatalog) matchesOutput(entry convertRecipeEntry, row map[string]any, query string) bool {
	needle := strings.ToLower(convertRecipeText(query))
	if needle == "" {
		return true
	}
	for _, output := range entry.recipe.Outputs {
		if strings.Contains(strings.ToLower(convertRecipeText(output.Name)), needle) {
			return true
		}
	}
	// A resource box can name a product bundle. Match that actual title, never
	// the fallback title assembled from resource names and quantities.
	if stringValue(row["mysekaiConvertItemObtainType"]) != "resource_box" {
		return false
	}
	box := catalog.boxes[convertRecipeBoxKey{"mysekai_convert_obtain", intNumber(row["externalId"], 0)}]
	name := strings.ToLower(convertRecipeText(stringValue(box["name"])))
	return strings.Contains(name, needle)
}
