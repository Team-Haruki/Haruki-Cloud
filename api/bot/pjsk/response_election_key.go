package pjsk

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"haruki-cloud/internal/onebot11"
	renderregion "haruki-cloud/internal/pjsk/region"
)

const responseElectionRedisKeyPrefix = "haruki:bot:response-election:"

// responseElectionIdentity returns the transport-independent identity of one
// command event. Bot-specific fields are intentionally excluded so every Bot
// that observes the same platform event joins the same election.
func responseElectionIdentity(req BotCommandRequest) string {
	h := sha256.New()
	writeResponseElectionHashField(h, []byte("v1"))
	writeResponseElectionHashField(h, []byte(req.Platform))
	writeResponseElectionHashField(h, []byte(req.PlatformGroupID))
	writeResponseElectionHashField(h, []byte(req.PlatformUserID))
	writeResponseElectionHashField(h, []byte(req.MatchedCommand))
	writeResponseElectionHashField(h, []byte(canonicalResponseElectionServer(req.Server)))
	writeResponseElectionHashField(h, canonicalResponseElectionBool(req.EnableParamEcho))

	var message canonicalResponseElectionEncoder
	message.writeValue(reflect.ValueOf(responseElectionCanonicalMessage(req.Message, req.SelfID)))
	writeResponseElectionHashField(h, message.Bytes())
	return hex.EncodeToString(h.Sum(nil))
}

func canonicalResponseElectionServer(server string) string {
	server = strings.TrimSpace(server)
	if normalized := renderregion.Normalize(server); !normalized.IsZero() {
		return normalized.String()
	}
	return server
}

func responseElectionCanonicalMessage(message onebot11.Message, selfID string) onebot11.Message {
	selfID = strings.TrimSpace(selfID)
	if selfID == "" {
		return message
	}
	canonical := append(onebot11.Message(nil), message...)
	for index, segment := range canonical {
		if segment.Type != onebot11.TypeAt {
			continue
		}
		mentionedID := ""
		switch data := segment.Data.(type) {
		case onebot11.AtData:
			mentionedID = data.QQ
		case map[string]any:
			mentionedID = fmt.Sprint(data[onebot11.KeyQQ])
		case map[string]string:
			mentionedID = data[onebot11.KeyQQ]
		case map[any]any:
			mentionedID = fmt.Sprint(data[onebot11.KeyQQ])
		}
		if strings.TrimSpace(mentionedID) == selfID {
			canonical[index].Data = onebot11.AtData{QQ: "<self>"}
		}
	}
	return canonical
}

func responseElectionStateKey(identity string) string {
	return responseElectionRedisKeyPrefix + "{" + identity + "}:state"
}

func responseElectionCandidatesKey(identity string) string {
	return responseElectionRedisKeyPrefix + "{" + identity + "}:candidates"
}

type responseElectionHashWriter interface {
	Write([]byte) (int, error)
}

func writeResponseElectionHashField(dst responseElectionHashWriter, value []byte) {
	var size [8]byte
	binary.BigEndian.PutUint64(size[:], uint64(len(value)))
	_, _ = dst.Write(size[:])
	_, _ = dst.Write(value)
}

func canonicalResponseElectionBool(value bool) []byte {
	if value {
		return []byte{1}
	}
	return []byte{0}
}

type canonicalResponseElectionEncoder struct {
	bytes.Buffer
}

func (e *canonicalResponseElectionEncoder) writeValue(value reflect.Value) {
	if !value.IsValid() {
		e.WriteByte('0')
		return
	}
	for value.Kind() == reflect.Interface || value.Kind() == reflect.Pointer {
		if value.IsNil() {
			e.WriteByte('0')
			return
		}
		value = value.Elem()
	}

	if value.CanInterface() {
		if number, ok := value.Interface().(json.Number); ok {
			e.writeAtom('n', canonicalResponseElectionNumber(number.String()))
			return
		}
	}

	switch value.Kind() {
	case reflect.Bool:
		e.WriteByte('b')
		if value.Bool() {
			e.WriteByte(1)
		} else {
			e.WriteByte(0)
		}
	case reflect.String:
		e.writeAtom('s', value.String())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		e.writeAtom('n', strconv.FormatInt(value.Int(), 10))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		e.writeAtom('n', strconv.FormatUint(value.Uint(), 10))
	case reflect.Float32, reflect.Float64:
		e.writeAtom('n', canonicalResponseElectionFloat(value.Float(), value.Type().Bits()))
	case reflect.Slice, reflect.Array:
		e.WriteByte('a')
		e.writeCount(value.Len())
		for index := 0; index < value.Len(); index++ {
			var item canonicalResponseElectionEncoder
			item.writeValue(value.Index(index))
			e.writeBytes(item.Bytes())
		}
	case reflect.Map:
		e.writeMap(value)
	case reflect.Struct:
		e.writeStruct(value)
	default:
		e.writeAtom('x', value.Type().String())
	}
}

func (e *canonicalResponseElectionEncoder) writeMap(value reflect.Value) {
	if value.IsNil() {
		e.WriteByte('0')
		return
	}
	entries := make([]canonicalResponseElectionMapEntry, 0, value.Len())
	iterator := value.MapRange()
	for iterator.Next() {
		var key canonicalResponseElectionEncoder
		key.writeValue(iterator.Key())
		var item canonicalResponseElectionEncoder
		item.writeValue(iterator.Value())
		entries = append(entries, canonicalResponseElectionMapEntry{key: key.Bytes(), value: item.Bytes()})
	}
	sort.Slice(entries, func(i, j int) bool {
		if compared := bytes.Compare(entries[i].key, entries[j].key); compared != 0 {
			return compared < 0
		}
		return bytes.Compare(entries[i].value, entries[j].value) < 0
	})
	e.writeMapEntries(entries)
}

func (e *canonicalResponseElectionEncoder) writeStruct(value reflect.Value) {
	entries := make([]canonicalResponseElectionMapEntry, 0, value.NumField())
	typeInfo := value.Type()
	for index := 0; index < value.NumField(); index++ {
		fieldInfo := typeInfo.Field(index)
		if fieldInfo.PkgPath != "" {
			continue
		}
		name, omitEmpty, skip := responseElectionFieldName(fieldInfo)
		if skip || omitEmpty && value.Field(index).IsZero() {
			continue
		}
		var key canonicalResponseElectionEncoder
		key.writeValue(reflect.ValueOf(name))
		var item canonicalResponseElectionEncoder
		item.writeValue(value.Field(index))
		entries = append(entries, canonicalResponseElectionMapEntry{key: key.Bytes(), value: item.Bytes()})
	}
	sort.Slice(entries, func(i, j int) bool {
		return bytes.Compare(entries[i].key, entries[j].key) < 0
	})
	e.writeMapEntries(entries)
}

func (e *canonicalResponseElectionEncoder) writeMapEntries(entries []canonicalResponseElectionMapEntry) {
	e.WriteByte('m')
	e.writeCount(len(entries))
	for _, entry := range entries {
		e.writeBytes(entry.key)
		e.writeBytes(entry.value)
	}
}

func (e *canonicalResponseElectionEncoder) writeAtom(tag byte, value string) {
	e.WriteByte(tag)
	e.writeBytes([]byte(value))
}

func (e *canonicalResponseElectionEncoder) writeCount(count int) {
	var size [8]byte
	binary.BigEndian.PutUint64(size[:], uint64(count))
	_, _ = e.Write(size[:])
}

func (e *canonicalResponseElectionEncoder) writeBytes(value []byte) {
	e.writeCount(len(value))
	_, _ = e.Write(value)
}

type canonicalResponseElectionMapEntry struct {
	key   []byte
	value []byte
}

func responseElectionFieldName(field reflect.StructField) (name string, omitEmpty bool, skip bool) {
	tag := field.Tag.Get("json")
	if tag == "" {
		tag = field.Tag.Get("msgpack")
	}
	parts := strings.Split(tag, ",")
	if len(parts) > 0 && parts[0] == "-" {
		return "", false, true
	}
	name = field.Name
	if len(parts) > 0 && parts[0] != "" {
		name = parts[0]
	}
	for _, option := range parts[1:] {
		if option == "omitempty" {
			omitEmpty = true
		}
	}
	return name, omitEmpty, false
}

func canonicalResponseElectionFloat(value float64, bits int) string {
	switch {
	case math.IsNaN(value):
		return "nan"
	case math.IsInf(value, 1):
		return "+inf"
	case math.IsInf(value, -1):
		return "-inf"
	case value == 0:
		return "0"
	case math.Trunc(value) == value:
		return strconv.FormatFloat(value, 'f', -1, bits)
	default:
		return strconv.FormatFloat(value, 'g', -1, bits)
	}
}

func canonicalResponseElectionNumber(value string) string {
	if integer, err := strconv.ParseInt(value, 10, 64); err == nil {
		return strconv.FormatInt(integer, 10)
	}
	if unsigned, err := strconv.ParseUint(value, 10, 64); err == nil {
		return strconv.FormatUint(unsigned, 10)
	}
	if floating, err := strconv.ParseFloat(value, 64); err == nil {
		return canonicalResponseElectionFloat(floating, 64)
	}
	return value
}
