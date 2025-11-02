package model

// RequestDatabaseType represents the type of database to request
type RequestDatabaseType string

const (
	RequestDatabaseTypeMain  RequestDatabaseType = "main"
	RequestDatabaseTypeSuite RequestDatabaseType = "suite"
)

func (r RequestDatabaseType) String() string {
	return string(r)
}

// SekaiBindingServerRegion represents the server region for Sekai binding
type SekaiBindingServerRegion string

const (
	SekaiBindingServerRegionJP      SekaiBindingServerRegion = "jp"
	SekaiBindingServerRegionEN      SekaiBindingServerRegion = "en"
	SekaiBindingServerRegionTW      SekaiBindingServerRegion = "tw"
	SekaiBindingServerRegionKR      SekaiBindingServerRegion = "kr"
	SekaiBindingServerRegionCN      SekaiBindingServerRegion = "cn"
	SekaiBindingServerRegionDefault SekaiBindingServerRegion = "default"
)

func (s SekaiBindingServerRegion) String() string {
	return string(s)
}

// SekaiSuiteDataType represents the type of Sekai suite data
type SekaiSuiteDataType string

const (
	SekaiSuiteDataTypeSuite   SekaiSuiteDataType = "suite"
	SekaiSuiteDataTypeMySekai SekaiSuiteDataType = "mysekai"
)

func (s SekaiSuiteDataType) String() string {
	return string(s)
}

// InstantMessengerPlatform represents the instant messenger platform
type InstantMessengerPlatform string

const (
	InstantMessengerPlatformQQ       InstantMessengerPlatform = "qq"
	InstantMessengerPlatformQQBot    InstantMessengerPlatform = "qq_bot"
	InstantMessengerPlatformDiscord  InstantMessengerPlatform = "discord"
	InstantMessengerPlatformTelegram InstantMessengerPlatform = "telegram"
)

func (i InstantMessengerPlatform) String() string {
	return string(i)
}

// PjskAliasType represents the type of PJSK alias
type PjskAliasType string

const (
	PjskAliasTypeMusic     PjskAliasType = "music"
	PjskAliasTypeCharacter PjskAliasType = "character"
)

func (p PjskAliasType) String() string {
	return string(p)
}
