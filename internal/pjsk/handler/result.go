package handler

type CommandResultDataType string

const (
	CommandResultDataTypeImagePNG CommandResultDataType = "image/png"
	CommandResultDataTypeImageURL CommandResultDataType = "image/url"
	CommandResultDataTypeText     CommandResultDataType = "text/plain"
)
