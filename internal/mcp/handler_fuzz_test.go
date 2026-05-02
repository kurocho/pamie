// SPDX-License-Identifier: AGPL-3.0-only

package mcp

import (
	"encoding/json"
	"testing"
)

func FuzzParseRequest(f *testing.F) {
	for _, seed := range [][]byte{
		[]byte(`{"jsonrpc":"2.0","id":1,"method":"ping"}`),
		[]byte(`{"jsonrpc":"2.0","method":"notifications/initialized"}`),
		[]byte(`{"jsonrpc":"2.0","id":null,"method":"tools/list","params":{}}`),
		[]byte(`{`),
		[]byte(`[]`),
		[]byte(`{"jsonrpc":2,"id":1,"method":false}`),
		[]byte(`{"jsonrpc":"2.0","id":1,"method":"ping"} {"jsonrpc":"2.0"}`),
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, body []byte) {
		req, hasID, err := parseRequest(body)
		if err != nil {
			return
		}
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(body, &raw); err != nil {
			t.Fatalf("parseRequest succeeded but json.Unmarshal failed: %v", err)
		}
		_, wantID := raw["id"]
		if hasID != wantID {
			t.Fatalf("hasID = %t, want %t for %q", hasID, wantID, body)
		}
		if req.Method != "" {
			encoded, err := json.Marshal(req.Method)
			if err != nil || !json.Valid(encoded) {
				t.Fatalf("method contains invalid JSON string data: %q", req.Method)
			}
		}
	})
}
