package deck

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/klauspost/compress/zstd"
)

func (r *RemoteDeckRecommender) postJSON(path string, requestBody interface{}, responseBody interface{}) error {
	body, err := json.Marshal(requestBody)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, r.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		var remoteErr remoteErrorResponse
		if json.Unmarshal(payload, &remoteErr) == nil && strings.TrimSpace(remoteErr.Error) != "" {
			return fmt.Errorf("%s", remoteErr.Error)
		}
		if trimmed := strings.TrimSpace(string(payload)); trimmed != "" {
			return fmt.Errorf("deck-service returned HTTP %d: %s", resp.StatusCode, trimmed)
		}
		return fmt.Errorf("deck-service returned HTTP %d", resp.StatusCode)
	}
	if responseBody == nil || len(payload) == 0 {
		return nil
	}
	return json.Unmarshal(payload, responseBody)
}

func (r *RemoteDeckRecommender) postBinary(path string, payload []byte, responseBody interface{}) error {
	req, err := http.NewRequest(http.MethodPost, r.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := r.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		var remoteErr remoteErrorResponse
		if json.Unmarshal(body, &remoteErr) == nil && strings.TrimSpace(remoteErr.Error) != "" {
			return fmt.Errorf("%s", remoteErr.Error)
		}
		if trimmed := strings.TrimSpace(string(body)); trimmed != "" {
			return fmt.Errorf("deck-service returned HTTP %d: %s", resp.StatusCode, trimmed)
		}
		return fmt.Errorf("deck-service returned HTTP %d", resp.StatusCode)
	}
	if responseBody == nil || len(body) == 0 {
		return nil
	}
	return json.Unmarshal(body, responseBody)
}

func buildMultipartPayload(segments ...[]byte) []byte {
	var raw bytes.Buffer
	for _, segment := range segments {
		if err := binary.Write(&raw, binary.BigEndian, uint32(len(segment))); err != nil {
			return nil
		}
		if _, err := raw.Write(segment); err != nil {
			return nil
		}
	}

	var compressed bytes.Buffer
	writer, err := zstd.NewWriter(&compressed)
	if err != nil {
		return nil
	}
	if _, err := writer.Write(raw.Bytes()); err != nil {
		_ = writer.Close()
		return nil
	}
	if err := writer.Close(); err != nil {
		return nil
	}
	return compressed.Bytes()
}
