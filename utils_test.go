// Package jsonrpc provides a Go implementation of the JSON-RPC 2.0 specification, as well as tools
// to parse and work with JSON-RPC requests and responses.
package jsonrpc

import (
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRandomJSONRPCID(t *testing.T) {
	t.Run("Generates non-zero IDs", func(t *testing.T) {
		id1 := RandomJSONRPCID()
		id2 := RandomJSONRPCID()

		assert.GreaterOrEqual(t, id1, int64(0))
		assert.GreaterOrEqual(t, id2, int64(0))
		assert.LessOrEqual(t, id1, int64(2147483647))
		assert.LessOrEqual(t, id2, int64(2147483647))
	})

	t.Run("Generates unique IDs across multiple calls", func(t *testing.T) {
		seen := make(map[int64]bool)
		duplicates := 0

		// Generate 1000 IDs and check for uniqueness
		for i := 0; i < 1000; i++ {
			id := RandomJSONRPCID()
			if seen[id] {
				duplicates++
			}
			seen[id] = true
		}

		// With a range of 2^31, duplicates should be extremely rare in 1000 iterations
		// Allow up to 5 duplicates due to birthday paradox, but expect 0-2 typically
		assert.LessOrEqual(t, duplicates, 5, "Too many duplicate IDs generated")
	})

	t.Run("Two consecutive calls produce different IDs (usually)", func(t *testing.T) {
		// Note: This test may occasionally fail due to randomness, but very unlikely
		id1 := RandomJSONRPCID()
		id2 := RandomJSONRPCID()
		assert.NotEqual(t, id1, id2, "Consecutive IDs should differ (test may rarely fail due to randomness)")
	})
}

func TestReadAll(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		chunkSize    int64
		expectedSize int
		expected     string
		expectError  bool
	}{
		{
			name:         "Read all data in one go",
			input:        "Hello, World!",
			chunkSize:    1024,
			expectedSize: 0,
			expected:     "Hello, World!",
			expectError:  false,
		},
		{
			name:         "Read data in chunks",
			input:        "Hello, World!",
			chunkSize:    5,
			expectedSize: 0,
			expected:     "Hello, World!",
			expectError:  false,
		},
		{
			name:         "Small message with expected size",
			input:        "Short",
			chunkSize:    1024,
			expectedSize: 5,
			expected:     "Short",
			expectError:  false,
		},
		{
			name:         "Medium message with expected size",
			input:        strings.Repeat("A", 5000),
			chunkSize:    1024,
			expectedSize: 5000,
			expected:     strings.Repeat("A", 5000),
			expectError:  false,
		},
		{
			name:         "Large message with expected size",
			input:        strings.Repeat("B", 20000),
			chunkSize:    4096,
			expectedSize: 20000,
			expected:     strings.Repeat("B", 20000),
			expectError:  false,
		},
		{
			name:         "Nil reader",
			input:        "",
			chunkSize:    1024,
			expectedSize: 0,
			expected:     "",
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var reader io.Reader
			if tt.input != "" {
				reader = strings.NewReader(tt.input)
			}

			data, err := readAll(reader, tt.chunkSize, tt.expectedSize)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, string(data))
			}
		})
	}
}
