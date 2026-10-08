package snapshot

import (
	"bytes"
	"strconv"
	"strings"
)

const uploadTimeMember = "upload_time"

// payloadUploadTime returns the top-level upload_time (seconds) of a private
// data payload. The common Toolbox shape is answered by scanTopLevelUploadTime
// without decoding anything; every other shape falls back to the full decode
// of parseTopLevelUploadTime, so the result never differs from it for a
// well-formed payload.
func payloadUploadTime(data []byte) (int64, error) {
	if uploadTime, ok := trailingUploadTime(data); ok {
		return uploadTime, nil
	}
	if uploadTime, ok := scanTopLevelUploadTime(data); ok {
		return uploadTime, nil
	}
	return parseTopLevelUploadTime(data)
}

// trailingUploadTime answers in constant time when upload_time is the last
// member of the top-level object, which is where a sorted-key encoder puts it
// in a MySekai payload. In a well-formed document the tokens right before the
// final '}' are the root's last member, and a full decode keeps the last
// occurrence of a member, so this is exactly the value it would return.
func trailingUploadTime(data []byte) (int64, bool) {
	end := len(bytes.TrimRight(data, jsonSpace))
	if end == 0 || data[end-1] != '}' {
		return 0, false
	}
	valueEnd := len(bytes.TrimRight(data[:end-1], jsonSpace))
	valueStart := valueEnd
	for valueStart > 0 && (data[valueStart-1] >= '0' && data[valueStart-1] <= '9' || data[valueStart-1] == '-') {
		valueStart--
	}
	if valueStart == valueEnd {
		return 0, false
	}
	colon := len(bytes.TrimRight(data[:valueStart], jsonSpace))
	if colon == 0 || data[colon-1] != ':' {
		return 0, false
	}
	nameEnd := len(bytes.TrimRight(data[:colon-1], jsonSpace))
	const quotedName = `"` + uploadTimeMember + `"`
	if !bytes.HasSuffix(data[:nameEnd], []byte(quotedName)) {
		return 0, false
	}
	before := len(bytes.TrimRight(data[:nameEnd-len(quotedName)], jsonSpace))
	if before == 0 || (data[before-1] != ',' && data[before-1] != '{') {
		return 0, false
	}
	s := jsonScanner{data: data[:end], pos: valueStart}
	return s.integer()
}

// scanTopLevelUploadTime walks the members of the top-level object, skipping
// each value structurally, until it reaches the "upload_time" member. It only
// answers when that member is a plain integer under its exact name and no
// earlier member name could also match it (case-insensitive or escaped
// names); anything else reports ok=false. The scan does not validate the
// skipped values: Build decodes the whole payload before it is ever used.
// Toolbox encodes each top-level key once, so the first exact match is the
// member a full decode would read.
func scanTopLevelUploadTime(data []byte) (int64, bool) {
	s := jsonScanner{data: data}
	if !s.consume('{') {
		return 0, false
	}
	for {
		if !s.consume('"') {
			return 0, false
		}
		name, ok := s.memberName()
		if !ok || !s.consume(':') {
			return 0, false
		}
		if name == uploadTimeMember {
			return s.integer()
		}
		if strings.EqualFold(name, uploadTimeMember) || !s.skipValue() {
			return 0, false
		}
		if !s.consume(',') {
			return 0, false
		}
	}
}

type jsonScanner struct {
	data []byte
	pos  int
}

func (s *jsonScanner) skipSpace() {
	for s.pos < len(s.data) {
		switch s.data[s.pos] {
		case ' ', '\t', '\n', '\r':
			s.pos++
		default:
			return
		}
	}
}

// consume skips whitespace and then the expected byte.
func (s *jsonScanner) consume(want byte) bool {
	s.skipSpace()
	if s.pos >= len(s.data) || s.data[s.pos] != want {
		return false
	}
	s.pos++
	return true
}

// memberName reads a member name after its opening quote. Names with escape
// sequences are rejected so they cannot hide an alternative spelling.
func (s *jsonScanner) memberName() (string, bool) {
	end := bytes.IndexByte(s.data[s.pos:], '"')
	if end < 0 {
		return "", false
	}
	name := s.data[s.pos : s.pos+end]
	if bytes.IndexByte(name, '\\') >= 0 {
		return "", false
	}
	s.pos += end + 1
	return string(name), true
}

// skipString moves past a string whose opening quote was already consumed.
func (s *jsonScanner) skipString() bool {
	for {
		end := bytes.IndexByte(s.data[s.pos:], '"')
		if end < 0 {
			return false
		}
		quote := s.pos + end
		backslashes := 0
		for i := quote - 1; i >= s.pos && s.data[i] == '\\'; i-- {
			backslashes++
		}
		s.pos = quote + 1
		if backslashes%2 == 0 {
			return true
		}
	}
}

// skipValue moves past one value: a string, a balanced object or array, or a
// literal or number that ends at the next delimiter.
func (s *jsonScanner) skipValue() bool {
	s.skipSpace()
	if s.pos >= len(s.data) {
		return false
	}
	switch s.data[s.pos] {
	case '"':
		s.pos++
		return s.skipString()
	case '{', '[':
		return s.skipContainer()
	default:
		start := s.pos
		for s.pos < len(s.data) && !jsonScalarEnd[s.data[s.pos]] {
			s.pos++
		}
		return s.pos > start
	}
}

func (s *jsonScanner) skipContainer() bool {
	depth := 0
	data := s.data
	for s.pos < len(data) {
		for s.pos < len(data) && !jsonStructural[data[s.pos]] {
			s.pos++
		}
		if s.pos >= len(data) {
			return false
		}
		c := data[s.pos]
		s.pos++
		switch c {
		case '"':
			if !s.skipString() {
				return false
			}
		case '{', '[':
			depth++
		case '}', ']':
			depth--
			if depth == 0 {
				return true
			}
		}
	}
	return false
}

// integer reads the member value as an int64 when it is a plain integer
// literal followed by a delimiter.
func (s *jsonScanner) integer() (int64, bool) {
	s.skipSpace()
	start := s.pos
	if s.pos < len(s.data) && s.data[s.pos] == '-' {
		s.pos++
	}
	digits := s.pos
	for s.pos < len(s.data) && s.data[s.pos] >= '0' && s.data[s.pos] <= '9' {
		s.pos++
	}
	if s.pos-digits > 1 && s.data[digits] == '0' {
		return 0, false
	}
	if s.pos >= len(s.data) || !jsonScalarEnd[s.data[s.pos]] || s.data[s.pos] == '"' {
		return 0, false
	}
	value, err := strconv.ParseInt(string(s.data[start:s.pos]), 10, 64)
	if err != nil {
		return 0, false
	}
	return value, true
}

const jsonSpace = " \t\n\r"

var jsonStructural = [256]bool{'"': true, '{': true, '}': true, '[': true, ']': true}

var jsonScalarEnd = [256]bool{
	',': true, '}': true, ']': true, ':': true, '"': true,
	' ': true, '\t': true, '\n': true, '\r': true,
}
