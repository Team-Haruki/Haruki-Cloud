package handler

// Keep command scopes explicit. Unlisted commands continue to request full Suite
// until their raw-data and profile dependencies have been audited together.
func mysekaiSuiteFields(mode string) []string {
	var fields []string
	switch mode {
	case mySekaiShopCommand:
		fields = []string{
			"userMysekaiColorfulPass", "userMysekaiGamedata",
			"userMysekaiMaterialPossession", "userMysekaiShops",
			"userMysekaiBlueprintShopItems", "userMysekaiBlueprints",
		}
	case mySekaiTalkListCommand:
		fields = []string{"userMysekaiCharacterTalks", "userMysekaiBlueprints", "userMysekaiGamedata"}
	case mySekaiDoorUpgradeCommand:
		fields = []string{"userMysekaiGates", "userMysekaiMaterials", "userMysekaiGamedata"}
	case mySekaiInfoPanelCommand:
		fields = []string{"userMysekaiGamedata"}
	default:
		return nil
	}
	// Public profiles fall back to Suite cards/decks and read Suite frames. These
	// also retain the factory's identity, leader image and data-source metadata.
	return append(fields, "userGamedata", "userProfile", "userDecks", "userCards", "userPlayerFrames", "now", "upload_time")
}
