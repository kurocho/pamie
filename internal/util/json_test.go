// SPDX-License-Identifier: AGPL-3.0-only

package util

import (
	"encoding/json"
	"testing"
)

func TestDecodeJSONObject(t *testing.T) {
	tests := []struct {
		name    string
		raw     json.RawMessage
		want    string
		wantErr bool
	}{
		{name: "empty", raw: nil, want: ""},
		{name: "null", raw: json.RawMessage(`null`), want: ""},
		{name: "valid object", raw: json.RawMessage(`{"name":"pamie"}`), want: "pamie"},
		{name: "unknown field", raw: json.RawMessage(`{"unknown":true}`), wantErr: true},
		{name: "trailing value", raw: json.RawMessage(`{"name":"pamie"} {"name":"extra"}`), wantErr: true},
		{name: "invalid json", raw: json.RawMessage(`{"name":`), wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var got struct {
				Name string `json:"name"`
			}
			err := DecodeJSONObject(test.raw, &got)
			if test.wantErr {
				if err == nil {
					t.Fatal("DecodeJSONObject() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("DecodeJSONObject() error = %v", err)
			}
			if got.Name != test.want {
				t.Fatalf("DecodeJSONObject() name = %q, want %q", got.Name, test.want)
			}
		})
	}
}
