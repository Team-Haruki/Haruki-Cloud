package onebot11

type BotCommandResponse struct {
	Replay  bool    `json:"replay" msgpack:"replay"`
	Message Message `json:"message" msgpack:"message"`
}
