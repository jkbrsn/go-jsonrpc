// Package jsonrpc provides a Go implementation of the JSON-RPC 2.0 specification, as well as tools
// to parse and work with JSON-RPC requests and responses.
package jsonrpc

import (
	"bytes"
	"errors"
	"io"
	"math/rand/v2"
	"strconv"
	"strings"
)

// revive:disable:add-constant makes sense here

// formatFloat64ID formats a float64 ID as a string, removing trailing zeroes while preserving ".0" for whole numbers.
//
// This function supports fractional JSON-RPC IDs, which is a deviation from the JSON-RPC 2.0 specification.
// The spec states that ID numbers "SHOULD NOT contain fractional parts" (Section 5),
// but this library allows them for flexibility and compatibility.
//
// Reference: https://www.jsonrpc.org/specification#request_object
func formatFloat64ID(id float64) string {
	str := strconv.FormatFloat(id, 'f', -1, 64)

	// Find the decimal point
	if dot := strings.IndexByte(str, '.'); dot >= 0 {
		// Trim trailing zeroes but leave one in the case of a whole number
		str = strings.TrimRight(str, "0")
		if str[len(str)-1] == '.' {
			str += "0"
		}
	} else {
		// No decimal point, so add one
		str += ".0"
	}

	return str
}

// RandomJSONRPCID returns a randomly generated value appropriate for a JSON-RPC ID field.
// Returns an int64 in the range [0, 2147483647] (int32 range) for compatibility.
// Uses math/rand/v2 which is automatically seeded and provides good randomness.
func RandomJSONRPCID() int64 {
	return int64(rand.IntN(2147483647)) // math.MaxInt32
}

// readAll reads all data from the given reader and returns it as a byte slice.
// The buffer size adapts based on expectedSize to minimize allocations:
// - For small messages (<= 1KB): starts with 512B
// - For medium messages (1KB-16KB): starts with expectedSize
// - For large messages (>16KB): starts with 16KB, grows as needed
func readAll(reader io.Reader, chunkSize int64, expectedSize int) ([]byte, error) {
	if reader == nil {
		return nil, errors.New("cannot read from nil reader")
	}

	// Adaptive initial buffer sizing
	initialSize := 512                 // Default for unknown sizes (small messages)
	upperSizeLimit := 50 * 1024 * 1024 // Max limit of 50MB

	if expectedSize > 0 && expectedSize < upperSizeLimit {
		if expectedSize <= 1024 {
			// Small messages: use compact buffer
			initialSize = 512
		} else if expectedSize <= 16*1024 {
			// Medium messages: pre-allocate exact size
			initialSize = expectedSize
		} else {
			// Large messages: start with 16KB, grow as needed
			initialSize = 16 * 1024
		}
	} else if expectedSize == 0 {
		// Unknown size: start small for typical small JSON-RPC messages
		initialSize = 512
	}

	buffer := bytes.NewBuffer(make([]byte, 0, initialSize))

	// Grow buffer if we know we'll need more space
	if expectedSize > initialSize && expectedSize < upperSizeLimit {
		buffer.Grow(expectedSize - initialSize)
	}

	// Read data in chunks
	for {
		n, err := io.CopyN(buffer, reader, chunkSize)
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		if n == 0 {
			break
		}
	}

	return buffer.Bytes(), nil
}
