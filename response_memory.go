package jsonrpc

import (
	"encoding/json"
	"errors"

	"github.com/bytedance/sonic/ast"
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

// Clone creates a copy of the response with minimal shared state.
//
// Deep copies:
//   - rawID, rawError, result byte slices
//   - Error struct
//
// Shallow copies type any fields:
//   - id field
//   - Error.Data field
//
// Not copied:
//   - AST cache (clone starts with empty cache)
//   - Sync primitives (fresh Once and Mutex created)
func (r *Response) Clone() (*Response, error) {
	if r == nil {
		return nil, errors.New("cannot clone nil response")
	}

	clone := &Response{
		jsonrpc: r.jsonrpc,
	}

	// Shallow copy ID (safe for primitives, pointers will be shared)
	clone.id = r.id

	// Deep copy rawID byte slice
	if len(r.rawID) > 0 {
		clone.rawID = make(json.RawMessage, len(r.rawID))
		copy(clone.rawID, r.rawID)
	}

	// Copy Error
	if r.err != nil {
		clone.err = &Error{
			Code:    r.err.Code,
			Message: r.err.Message,
			Data:    r.err.Data, // Shallow copy
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

// Free releases memory-retaining fields. Only use after consuming the response.
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

	// Note: we keep r.id and r.err for logging purposes (typically small values)
}

// Size returns the approximate serialized size of the response in bytes.
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
