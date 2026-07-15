package pjsk

import (
	"fmt"

	"haruki-cloud/api"
	"haruki-cloud/internal/observability/commandtrace"

	"github.com/bytedance/sonic"
	"github.com/gofiber/fiber/v3"
	"github.com/shamaton/msgpack/v3"
)

// botResponseEnvelope is the transport-neutral form of a Bot API response.
// It contains no request-scoped Fiber or Noise state and can therefore be
// encoded before the request that will ultimately deliver it is selected.
type botResponseEnvelope struct {
	HTTPStatus int
	Message    string
	Data       any
}

// encodedBotResponse contains plaintext wire representations. Noise
// encryption, when enabled, remains the responsibility of the selected
// request's secure middleware.
type encodedBotResponse struct {
	HTTPStatus  int    `msgpack:"http_status"`
	JSONBody    []byte `msgpack:"json_body"`
	MsgPackBody []byte `msgpack:"msgpack_body"`
}

func newBotResponseEnvelope(status int, message string, data ...any) botResponseEnvelope {
	var value any
	if len(data) > 0 {
		value = data[0]
	}
	return botResponseEnvelope{
		HTTPStatus: status,
		Message:    message,
		Data:       value,
	}
}

func encodeBotResponseEnvelope(envelope botResponseEnvelope) (encodedBotResponse, error) {
	payload := api.BuildResponseMap(envelope.HTTPStatus, envelope.Message, envelope.Data)
	jsonBody, err := sonic.Marshal(payload)
	if err != nil {
		return encodedBotResponse{}, fmt.Errorf("encode bot response as JSON: %w", err)
	}
	msgPackBody, err := msgpack.Marshal(payload)
	if err != nil {
		return encodedBotResponse{}, fmt.Errorf("encode bot response as MsgPack: %w", err)
	}
	return encodedBotResponse{
		HTTPStatus:  envelope.HTTPStatus,
		JSONBody:    jsonBody,
		MsgPackBody: msgPackBody,
	}, nil
}

func writeEncodedBotResponse(c fiber.Ctx, response encodedBotResponse) error {
	finish := commandtrace.MeasurePhase(c.Context(), "response_encode")
	defer finish()
	contentType := api.ContentTypeJSON
	body := response.JSONBody
	if c.Locals("secure_noise") != nil {
		contentType = api.ContentTypeMsgPack
		body = response.MsgPackBody
	}
	c.Set(fiber.HeaderContentType, contentType)
	return c.Status(response.HTTPStatus).Send(body)
}
