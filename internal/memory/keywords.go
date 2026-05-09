// SPDX-License-Identifier: AGPL-3.0-only

package memory

import (
	"fmt"
	"strings"
	"unicode"
)

const (
	MaxKeywordsPerMemory              = 64
	MaxKeywordRunes                   = 128
	MaxTitleKeywordsDocumentRunes     = 4096
	titleKeywordsEmbeddingScope       = "title_keywords"
	titleKeywordsEmbeddingDocumentTag = "Title:"
)

func normalizeKeywords(keywords []string) ([]string, error) {
	if len(keywords) > MaxKeywordsPerMemory {
		return nil, fmt.Errorf("%w: at most %d keywords are allowed", ErrInvalid, MaxKeywordsPerMemory)
	}
	normalized := make([]string, 0, len(keywords))
	seen := make(map[string]struct{}, len(keywords))
	for _, keyword := range keywords {
		trimmed := strings.TrimSpace(keyword)
		if trimmed == "" {
			continue
		}
		if runeLen(trimmed) > MaxKeywordRunes {
			return nil, fmt.Errorf("%w: keyword %q is longer than %d characters", ErrInvalid, trimmed, MaxKeywordRunes)
		}
		for _, r := range trimmed {
			if unicode.IsControl(r) {
				return nil, fmt.Errorf("%w: keyword %q contains control characters", ErrInvalid, trimmed)
			}
		}
		key := strings.ToLower(trimmed)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, trimmed)
	}
	return normalized, nil
}

func normalizedKeywordValue(keyword string) string {
	return strings.ToLower(strings.TrimSpace(keyword))
}

func buildTitleKeywordsEmbeddingDocument(title string, keywords []string) (string, bool) {
	title = strings.TrimSpace(title)
	filtered := make([]string, 0, len(keywords))
	for _, keyword := range keywords {
		trimmed := strings.TrimSpace(keyword)
		if trimmed != "" {
			filtered = append(filtered, trimmed)
		}
	}
	if title == "" && len(filtered) == 0 {
		return "", false
	}
	var builder strings.Builder
	if title != "" {
		builder.WriteString(titleKeywordsEmbeddingDocumentTag)
		builder.WriteByte(' ')
		builder.WriteString(title)
	}
	if len(filtered) > 0 {
		if builder.Len() > 0 {
			builder.WriteByte('\n')
		}
		builder.WriteString("Keywords:")
		for _, keyword := range filtered {
			builder.WriteString("\n- ")
			builder.WriteString(keyword)
		}
	}
	return builder.String(), true
}

func validateTitleKeywordsDocumentLength(title string, keywords []string) error {
	document, ok := buildTitleKeywordsEmbeddingDocument(title, keywords)
	if !ok {
		return nil
	}
	if runeLen(document) > MaxTitleKeywordsDocumentRunes {
		return fmt.Errorf("%w: title and keywords embedding document is longer than %d characters", ErrInvalid, MaxTitleKeywordsDocumentRunes)
	}
	return nil
}

func runeLen(value string) int {
	return len([]rune(value))
}
