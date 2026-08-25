package jsonutil

import (
	stdjson "encoding/json"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"errors"
	"io"
)

type RawMessage = stdjson.RawMessage
type Number = stdjson.Number

// Preserve established API payload and cache-key semantics while using the v2 engine.
var defaultOptions = stdjson.DefaultOptionsV1()

var useNumberOptions = jsonv2.WithUnmarshalers(
	jsonv2.UnmarshalFromFunc(func(decoder *jsontext.Decoder, value *any) error {
		if decoder.PeekKind() != '0' {
			return errors.ErrUnsupported
		}
		raw, err := decoder.ReadValue()
		if err != nil {
			return err
		}
		*value = Number(raw)
		return nil
	}),
)

func Marshal(value any) ([]byte, error) {
	return jsonv2.Marshal(value, defaultOptions)
}

func MarshalIndent(value any, prefix, indent string) ([]byte, error) {
	return jsonv2.Marshal(value,
		defaultOptions,
		jsontext.WithIndentPrefix(prefix),
		jsontext.WithIndent(indent),
	)
}

func MarshalWrite(writer io.Writer, value any) error {
	return jsonv2.MarshalWrite(writer, value, defaultOptions)
}

func Unmarshal(data []byte, target any) error {
	return jsonv2.Unmarshal(data, target, defaultOptions)
}

func UnmarshalRead(reader io.Reader, target any) error {
	return jsonv2.UnmarshalRead(reader, target, defaultOptions)
}

type Encoder struct {
	writer io.Writer
}

func NewEncoder(writer io.Writer) *Encoder {
	return &Encoder{writer: writer}
}

func (e *Encoder) Encode(value any) error {
	if err := MarshalWrite(e.writer, value); err != nil {
		return err
	}
	_, err := e.writer.Write([]byte{'\n'})
	return err
}

type Decoder struct {
	decoder   *jsontext.Decoder
	useNumber bool
}

func NewDecoder(reader io.Reader) *Decoder {
	return &Decoder{decoder: jsontext.NewDecoder(reader, defaultOptions)}
}

func (d *Decoder) UseNumber() {
	d.useNumber = true
}

func (d *Decoder) Decode(target any) error {
	if d.useNumber {
		return jsonv2.UnmarshalDecode(d.decoder, target, defaultOptions, useNumberOptions)
	}
	return jsonv2.UnmarshalDecode(d.decoder, target, defaultOptions)
}
