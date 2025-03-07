package jsonrpc

import (
	"bytes"
	"errors"
	"io"
	"math/rand"
)

const ServerSideException = -32603

// randomJSONRPCID returns a value appropriate for a JSON-RPC ID field. This is an int with a
// 32-bit range, as per the JSON-RPC specification.
func randomJSONRPCID() int64 {
	return int64(rand.Int31())
}

// str2Mem safely converts a string to a byte slice without copying the underlying data.
// The resulting byte slice should only be used for read operations to avoid violating
// Go's string immutability guarantee.
func str2Mem(s string) []byte {
	return []byte(s)
}

// ReadAll reads all data from the given reader and returns it as a byte slice.
func ReadAll(reader io.Reader, chunkSize int64, expectedSize int) ([]byte, error) {
	if reader == nil {
		return nil, errors.New("cannot read from nil reader")
	}

	// 16KB buffer by default
	buffer := bytes.NewBuffer(make([]byte, 0, 16*1024))

	upperSizeLimit := 50 * 1024 * 1024 // Max limit of 50MB
	if expectedSize > 0 && expectedSize < upperSizeLimit {
		n := expectedSize - buffer.Cap()
		if n > 0 {
			buffer.Grow(n)
		}
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
