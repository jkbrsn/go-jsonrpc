package jsonrpc

import (
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRandomJSONRPCID(t *testing.T) {
	id1 := RandomJSONRPCID()
	id2 := RandomJSONRPCID()

	assert.NotEqual(t, id1, id2)
	assert.Greater(t, id1, int64(0))
	assert.Greater(t, id2, int64(0))
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
