package jsonrpc

import (
	"bytes"
	"errors"
	"io"
	"math/rand/v2"
	"strconv"
	"strings"
	"sync"
)

// revive:disable:add-constant makes sense here

// bufferPool is a sync.Pool for reusing byte buffers during stream reading. Purpose is to reduce
// GC pressure in high-throughput scenarios by reusing buffers.
var bufferPool = sync.Pool{
	New: func() any {
		// Pre-allocate 16KB buffers (typical response size)
		buf := make([]byte, 0, 16*1024)
		return &buf
	},
}

// getBuffer retrieves a buffer from the pool.
func getBuffer() *[]byte {
	buf, ok := bufferPool.Get().(*[]byte)
	if !ok {
		// This should never happen, but satisfy the linter
		newBuf := make([]byte, 0, 16*1024)
		return &newBuf
	}
	return buf
}

// putBuffer returns a buffer to the pool after clearing it.
func putBuffer(buf *[]byte) {
	if buf == nil {
		return
	}
	// Reset the buffer but keep capacity for reuse
	*buf = (*buf)[:0]
	bufferPool.Put(buf)
}

// formatFloat64ID formats a float64 ID as a string, removing trailing zeroes while
// preserving ".0" for whole numbers.
//
// This function supports fractional JSON-RPC IDs, which is a deviation from the
// JSON-RPC 2.0 specification. The spec states that ID numbers "SHOULD NOT contain
// fractional parts" (Section 5), but this library allows them for flexibility
// and compatibility.
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
// Uses buffer pooling to reduce GC pressure in high-throughput scenarios.
// The returned byte slice is a copy to avoid retaining the pooled buffer.
func readAll(reader io.Reader, chunkSize int64, expectedSize int) ([]byte, error) {
	if reader == nil {
		return nil, errors.New("cannot read from nil reader")
	}

	// Get a buffer from the pool
	buf := getBuffer()
	defer putBuffer(buf)

	// Adaptive initial capacity based on expectedSize
	const upperSizeLimit = 50 * 1024 * 1024 // Max limit of 50MB

	// Scale and use bytes.Buffer wrapper for efficient append operations
	if expectedSize > 0 && expectedSize < upperSizeLimit {
		// Grow buffer cap if required by expectedSize
		if expectedSize > cap(*buf) {
			*buf = make([]byte, 0, expectedSize)
		}
	}
	buffer := bytes.NewBuffer(*buf)

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

	// Prevent memory leaks by making a copy of the data to avoid retaining the pooled buffer.
	// This avoids cases where the upstream buffer is retained by the response object.
	data := buffer.Bytes()
	result := make([]byte, len(data))
	copy(result, data)

	return result, nil
}
