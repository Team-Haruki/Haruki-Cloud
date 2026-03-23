package pjsk

type BuildResponse struct {
	Endpoint string      `json:"endpoint"`
	Method   string      `json:"method"`
	Payload  interface{} `json:"payload"`
}
