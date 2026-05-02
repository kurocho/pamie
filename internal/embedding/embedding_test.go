// SPDX-License-Identifier: AGPL-3.0-only

package embedding

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestLocalHashProviderEmbedsDeterministically(t *testing.T) {
	provider, err := NewLocalHashProvider(16)
	if err != nil {
		t.Fatalf("NewLocalHashProvider() error = %v", err)
	}

	first, err := provider.Embed(context.Background(), "Pamie vector search")
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	second, err := provider.Embed(context.Background(), "pamie vector search")
	if err != nil {
		t.Fatalf("Embed() second error = %v", err)
	}
	if len(first) != 16 {
		t.Fatalf("len(vector) = %d, want 16", len(first))
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("Embed() not deterministic: %v != %v", first, second)
	}
}

func TestLocalHashProviderRejectsInvalidDimensions(t *testing.T) {
	if _, err := NewLocalHashProvider(0); err == nil {
		t.Fatal("NewLocalHashProvider(0) error = nil, want error")
	}
}

func TestOllamaProviderEmbedsThroughLocalAPI(t *testing.T) {
	var gotModel string
	var gotInput string
	var gotDimensions int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/embed" {
			t.Fatalf("path = %q, want /api/embed", r.URL.Path)
		}
		var request struct {
			Model      string `json:"model"`
			Input      string `json:"input"`
			Dimensions int    `json:"dimensions"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request error = %v", err)
		}
		gotModel = request.Model
		gotInput = request.Input
		gotDimensions = request.Dimensions
		_, _ = w.Write([]byte(`{"embeddings":[[3,4,0]]}`))
	}))
	defer server.Close()

	provider, err := NewOllamaProvider(OllamaOptions{
		BaseURL:    server.URL,
		Model:      "test-embed",
		Dimensions: 3,
	})
	if err != nil {
		t.Fatalf("NewOllamaProvider() error = %v", err)
	}
	vector, err := provider.Embed(context.Background(), "semantic memory")
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	if gotModel != "test-embed" || gotInput != "semantic memory" || gotDimensions != 3 {
		t.Fatalf("request = model:%q input:%q dimensions:%d", gotModel, gotInput, gotDimensions)
	}
	if len(vector) != 3 || vector[0] != 0.6 || vector[1] != 0.8 || vector[2] != 0 {
		t.Fatalf("vector = %v, want normalized [0.6 0.8 0]", vector)
	}
}

func TestOllamaProviderRejectsDimensionMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"embeddings":[[1,0]]}`))
	}))
	defer server.Close()

	provider, err := NewOllamaProvider(OllamaOptions{
		BaseURL:    server.URL,
		Model:      "test-embed",
		Dimensions: 3,
	})
	if err != nil {
		t.Fatalf("NewOllamaProvider() error = %v", err)
	}
	if _, err := provider.Embed(context.Background(), "semantic memory"); err == nil {
		t.Fatal("Embed() error = nil, want dimension mismatch")
	}
}
