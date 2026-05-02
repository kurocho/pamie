// SPDX-License-Identifier: AGPL-3.0-only

package util

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

// DecodeJSONObject decodes one JSON object into dst, rejecting unknown fields and trailing values.
// Empty and null inputs are treated as an empty object for optional JSON-RPC params and tool arguments.
func DecodeJSONObject(raw json.RawMessage, dst any) error {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		raw = []byte("{}")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	var extra struct{}
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("unexpected extra JSON value")
	}
	return nil
}
