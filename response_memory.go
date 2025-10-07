package jsonrpc

import (
	"encoding/json" // Used for json.RawMessage type
	"errors"

	"github.com/bytedance/sonic/ast" // AST for zero-copy JSON traversal
)

const (
	// Size estimation constants
	jsonStructureOverhead  = 35 // {"jsonrpc":"2.0","id":,"result":}
	errorStructureOverhead = 20 // {"code":,"message":""}
	errorDataEstimate      = 50 // rough estimate for error data field
	float64SizeEstimate    = 12 // float64 average size (range: 5-23 bytes)
	nullSize               = 4  // "null"
	decimalBase            = 10 // base 10 for digit counting
)

// Clone creates a deep copy of the response, ensuring no shared references between the original
// and the clone. This is useful when deriving new responses or when middleware needs to modify
// responses without affecting the original.
//
// The clone includes:
//   - Deep copies of all byte slices (rawID, rawError, result)
//   - Copies of parsed values (id, err)
//   - A fresh jsonrpc version string
//
// The clone does NOT include:
//   - Cached AST nodes
//   - Sync primitives (fresh Once guards are created)
//
// Example usage:
//
//	original, _ := NewResponse(1, "data")
//	clone, err := original.Clone()
//	if err != nil {
//	    return err
//	}
func (r *Response) Clone() (*Response, error) {
	if r == nil {
		return nil, errors.New("cannot clone nil response")
	}

	clone := &Response{
		jsonrpc: r.jsonrpc,
	}

	// Deep copy ID
	// For primitive types (string, int64, float64), direct assignment is sufficient
	// as these are value types or immutable
	clone.id = r.id

	// Deep copy rawID byte slice
	if len(r.rawID) > 0 {
		clone.rawID = make(json.RawMessage, len(r.rawID))
		copy(clone.rawID, r.rawID)
	}

	// Deep copy Error
	if r.err != nil {
		clone.err = &Error{
			Code:    r.err.Code,
			Message: r.err.Message,
			Data:    r.err.Data, // Data is any, shallow copy
		}
	}

	// Deep copy rawError byte slice
	if len(r.rawError) > 0 {
		clone.rawError = make(json.RawMessage, len(r.rawError))
		copy(clone.rawError, r.rawError)
	}

	// Deep copy result byte slice
	if len(r.result) > 0 {
		clone.result = make(json.RawMessage, len(r.result))
		copy(clone.result, r.result)
	}

	return clone, nil
}

// Free releases heavy memory-retaining fields after the response has been consumed.
// This is useful for long-running services, to prevent memory leaks from retained buffers.
//
// After calling Free:
//   - All byte slices (rawID, rawError, result) are released
//   - Cached AST nodes are released
//   - Parsed values (id, err) are kept for logging purposes
//   - The response should not be used for marshaling or field access
//   - Concurrent use after Free is unsafe
//
// Example usage:
//
//	response, _ := DecodeResponse(data)
//	// Use response
//	json.NewEncoder(w).Encode(response)
//	// Explicitly free when done
//	response.Free()
func (r *Response) Free() {
	if r == nil {
		return
	}

	r.rawID = nil
	r.rawError = nil
	r.result = nil

	r.astMutex.Lock()
	r.astNode = ast.Node{}
	r.astErr = nil
	r.astMutex.Unlock()

	// Note: We keep r.id and r.err for logging purposes (typically small values)
}

// Size returns the approximate serialized size of the response in bytes.
// This is useful for metrics, logging, and deciding whether to buffer or stream responses.
//
// The calculation includes:
//   - JSON structure overhead (opening/closing braces, field names, colons, commas)
//   - ID field size (or "null" if not present)
//   - Error field size (if present)
//   - Result field size (if present)
//
// The returned size is an approximation and may differ slightly from the actual
// marshaled size due to formatting differences, but is accurate enough for
// practical purposes like logging and metrics.
//
// Example usage:
//
//	if response.Size() > 1024*1024 {
//	    log.Warn("Large response detected", "size", response.Size())
//	}
func (r *Response) Size() int {
	if r == nil {
		return 0
	}

	size := jsonStructureOverhead
	size += r.idSize()
	size += r.errorSize()
	size += r.resultSize()

	return size
}

// idSize estimates the size of the ID field
func (r *Response) idSize() int {
	if r.id != nil {
		switch v := r.id.(type) {
		case string:
			return len(v) + 2 // +2 for quotes
		case int64:
			return intDigits(int(v))
		case float64:
			return float64SizeEstimate
		default:
			return nullSize
		}
	}
	if len(r.rawID) > 0 {
		return len(r.rawID)
	}
	return nullSize
}

// errorSize estimates the size of the error field
func (r *Response) errorSize() int {
	if r.err != nil {
		size := errorStructureOverhead
		size += intDigits(r.err.Code)
		size += len(r.err.Message)
		if r.err.Data != nil {
			size += errorDataEstimate
		}
		return size
	}
	if len(r.rawError) > 0 {
		return len(r.rawError)
	}
	return 0
}

// resultSize returns the size of the result field
func (r *Response) resultSize() int {
	return len(r.result)
}

// intDigits calculates the number of digits needed to represent an integer
func intDigits(n int) int {
	if n == 0 {
		return 1
	}
	digits := 0
	absVal := n
	if absVal < 0 {
		digits++ // for minus sign
		absVal = -absVal
	}
	for absVal > 0 {
		digits++
		absVal /= decimalBase
	}
	return digits
}
