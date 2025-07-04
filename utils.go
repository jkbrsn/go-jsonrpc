// Package jsonrpc provides a Go implementation of the JSON-RPC 2.0 specification, as well as tools
// to parse and work with JSON-RPC requests and responses.
package jsonrpc

import (
	"bytes"
	"errors"
	"io"
	"math/rand"
	"strconv"
	"strings"
)

// formatFloat64ID formats a float64 ID as a string, removing trailing zeroes.
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

// RandomJSONRPCID returns a value appropriate for a JSON-RPC ID field. This is an int with a
// 32-bit range, as per the JSON-RPC specification.
func RandomJSONRPCID() int64 {
	return int64(rand.Int31())
}

// readAll reads all data from the given reader and returns it as a byte slice.
func readAll(reader io.Reader, chunkSize int64, expectedSize int) ([]byte, error) {
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
