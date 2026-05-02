// SPDX-License-Identifier: AGPL-3.0-only

package db

import "testing"

func FuzzValidateMetadataFilter(f *testing.F) {
	for _, seed := range []struct {
		key       string
		value     string
		number    int
		useNumber bool
	}{
		{key: "project", value: "pamie"},
		{key: "project.name", value: "pamie"},
		{key: "release_stage", value: "acceptance"},
		{key: "", value: "empty"},
		{key: "priority", number: 3, useNumber: true},
	} {
		f.Add(seed.key, seed.value, seed.number, seed.useNumber)
	}

	f.Fuzz(func(t *testing.T, key string, value string, number int, useNumber bool) {
		var candidate any = value
		if useNumber {
			candidate = number
		}
		_ = validateMetadataFilter(key, candidate)
	})
}

func FuzzBuildFTSQuery(f *testing.F) {
	for _, seed := range []string{
		"pamie acceptance",
		`"quoted" OR token`,
		"alpha:beta NEAR gamma",
		"emoji \U0001f9ea punctuation !@#$",
		"___ ---",
		"",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, query string) {
		fts, err := buildFTSQuery(query)
		if err != nil {
			return
		}
		if fts == "" {
			t.Fatal("buildFTSQuery returned empty query without error")
		}
		for _, r := range fts {
			switch {
			case r >= 'a' && r <= 'z':
			case r >= 'A' && r <= 'Z':
			case r >= '0' && r <= '9':
			case r == '_' || r == '-' || r == '"' || r == ' ':
			default:
				t.Fatalf("buildFTSQuery(%q) = %q, contains unsupported rune %q", query, fts, r)
			}
		}
	})
}
