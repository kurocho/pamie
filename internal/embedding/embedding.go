// SPDX-License-Identifier: AGPL-3.0-only

// Package embedding defines local embedding providers used by optional vector search.
package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"
)

const (
	LocalHashProviderName = "local-hash"
	LocalHashModelName    = "local-hash-v1"
	OllamaProviderName    = "ollama"
	DefaultOllamaURL      = "http://127.0.0.1:11434"
	DefaultOllamaModel    = "embeddinggemma"
	DefaultDimensions     = 384
	defaultHTTPTimeout    = 30 * time.Second
)

// Provider embeds untrusted memory text into a numeric vector.
type Provider interface {
	Name() string
	Model() string
	Dimensions() int
	Embed(ctx context.Context, text string) ([]float64, error)
}

// LocalHashProvider is a deterministic local-only lexical embedding provider.
type LocalHashProvider struct {
	dimensions int
}

type OllamaOptions struct {
	BaseURL    string
	Model      string
	Dimensions int
	KeepAlive  string
	Client     *http.Client
}

// OllamaProvider calls a locally running Ollama embedding model.
type OllamaProvider struct {
	endpoint   string
	model      string
	dimensions int
	keepAlive  string
	client     *http.Client
}

// NewLocalHashProvider creates a local provider with fixed vector dimensions.
func NewLocalHashProvider(dimensions int) (*LocalHashProvider, error) {
	if dimensions <= 0 {
		return nil, errors.New("embedding dimensions must be positive")
	}
	return &LocalHashProvider{dimensions: dimensions}, nil
}

func (p *LocalHashProvider) Name() string {
	return LocalHashProviderName
}

func (p *LocalHashProvider) Model() string {
	return fmt.Sprintf("%s-%d", LocalHashModelName, p.dimensions)
}

func (p *LocalHashProvider) Dimensions() int {
	return p.dimensions
}

func (p *LocalHashProvider) Embed(ctx context.Context, text string) ([]float64, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	vector := make([]float64, p.dimensions)
	for _, token := range tokenize(text) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		hash := hashToken(token)
		index := int(hash % uint64(p.dimensions))
		weight := 1.0
		if (hash>>63)&1 == 1 {
			weight = -1
		}
		vector[index] += weight
	}
	normalize(vector)
	return vector, nil
}

func tokenize(text string) []string {
	fields := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	tokens := fields[:0]
	for _, field := range fields {
		if field != "" {
			tokens = append(tokens, field)
		}
	}
	return tokens
}

func hashToken(token string) uint64 {
	hash := fnv.New64a()
	_, _ = hash.Write([]byte(token))
	return hash.Sum64()
}

func normalize(vector []float64) {
	var norm float64
	for _, value := range vector {
		norm += value * value
	}
	if norm == 0 {
		return
	}
	norm = math.Sqrt(norm)
	for i := range vector {
		vector[i] /= norm
	}
}

// NewOllamaProvider creates a local semantic embedding provider backed by Ollama.
func NewOllamaProvider(opts OllamaOptions) (*OllamaProvider, error) {
	baseURL := strings.TrimSpace(opts.BaseURL)
	if baseURL == "" {
		baseURL = DefaultOllamaURL
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("ollama URL must be absolute: %q", baseURL)
	}
	model := strings.TrimSpace(opts.Model)
	if model == "" {
		model = DefaultOllamaModel
	}
	if opts.Dimensions <= 0 {
		return nil, errors.New("ollama embedding dimensions must be positive")
	}
	client := opts.Client
	if client == nil {
		client = &http.Client{Timeout: defaultHTTPTimeout}
	}
	return &OllamaProvider{
		endpoint:   strings.TrimRight(baseURL, "/") + "/api/embed",
		model:      model,
		dimensions: opts.Dimensions,
		keepAlive:  opts.KeepAlive,
		client:     client,
	}, nil
}

func (p *OllamaProvider) Name() string {
	return OllamaProviderName
}

func (p *OllamaProvider) Model() string {
	return fmt.Sprintf("%s-%d", p.model, p.dimensions)
}

func (p *OllamaProvider) Dimensions() int {
	return p.dimensions
}

func (p *OllamaProvider) Embed(ctx context.Context, text string) ([]float64, error) {
	request := ollamaEmbedRequest{
		Model:      p.model,
		Input:      text,
		Truncate:   true,
		Dimensions: p.dimensions,
		KeepAlive:  p.keepAlive,
	}
	body, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("encode ollama request: %w", err)
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create ollama request: %w", err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")

	response, err := p.client.Do(httpRequest)
	if err != nil {
		return nil, fmt.Errorf("call ollama embed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return nil, fmt.Errorf("ollama embed status %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}

	var decoded ollamaEmbedResponse
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		return nil, fmt.Errorf("decode ollama response: %w", err)
	}
	vector := decoded.FirstEmbedding()
	if len(vector) == 0 {
		return nil, errors.New("ollama response did not include an embedding")
	}
	if len(vector) != p.dimensions {
		return nil, fmt.Errorf("ollama returned %d dimensions, want %d", len(vector), p.dimensions)
	}
	normalize(vector)
	return vector, nil
}

type ollamaEmbedRequest struct {
	Model      string `json:"model"`
	Input      string `json:"input"`
	Truncate   bool   `json:"truncate"`
	Dimensions int    `json:"dimensions,omitempty"`
	KeepAlive  string `json:"keep_alive,omitempty"`
}

type ollamaEmbedResponse struct {
	Embeddings [][]float64 `json:"embeddings"`
	Embedding  []float64   `json:"embedding"`
}

func (r ollamaEmbedResponse) FirstEmbedding() []float64 {
	if len(r.Embeddings) > 0 {
		return r.Embeddings[0]
	}
	return r.Embedding
}
