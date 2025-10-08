package jsonrpc

import (
	"bytes"
	"encoding/json" // Used for json.RawMessage type
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/bytedance/sonic/ast" // AST for zero-copy JSON traversal
)

// Response is a struct for JSON-RPC responses conforming to the JSON-RPC 2.0 specification.
// Response instances are immutable after decoding and safe for concurrent reads.
// All fields are unexported to enforce immutability. Use getter methods to access field values.
//
// The Response type uses lazy unmarshaling for the ID and Error fields to optimize performance.
// These fields are unmarshaled on first access via IDOrNil() or Err() respectively.
type Response struct {
	jsonrpc string

	// Immutable fields (set once during decode, never modified)
	// Access via getter methods: IDOrNil(), Err(), RawResult()
	id     any
	err    *Error
	result json.RawMessage

	// Internal fields for lazy unmarshaling and caching
	// rawID serves dual purpose:
	// 1. Stores raw ID bytes from incoming JSON (unmarshal path)
	// 2. Caches marshaled ID bytes for outgoing JSON (marshal path)
	rawID    json.RawMessage
	rawError json.RawMessage

	// One-time initialization guards for lazy operations
	idOnce  sync.Once
	errOnce sync.Once

	// AST node caching for efficient field access
	// Lazily built on first PeekByPath call, cached for subsequent calls
	astNode  ast.Node
	astOnce  sync.Once
	astMutex sync.RWMutex
	astErr   error
}

// Version returns the JSON-RPC protocol version (always "2.0" for valid responses).
func (r *Response) Version() string {
	return r.jsonrpc
}

// Err returns the error from the response, if any.
// The error is unmarshaled lazily on first call and cached for subsequent calls.
// This method is safe for concurrent use.
func (r *Response) Err() *Error {
	_ = r.UnmarshalError()
	return r.err
}

// RawResult returns the raw JSON-encoded result bytes.
// For string results, this includes the JSON quotes (e.g., "result" not result).
// Use UnmarshalResult to decode the result into a specific type.
func (r *Response) RawResult() json.RawMessage {
	return r.result
}

// IDOrNil returns the unmarshaled ID, or nil if unmarshaling fails.
// The ID is unmarshaled lazily on first call and cached for subsequent
// calls. This method is safe for concurrent use.
func (r *Response) IDOrNil() any {
	r.idOnce.Do(func() {
		// Ignore error - validation happens during decode
		// If unmarshal fails, ID remains nil
		_ = r.unmarshalID()
	})
	return r.id
}

// IDString returns the ID as a string.
func (r *Response) IDString() string {
	switch id := r.id.(type) {
	case string:
		return id
	case int64:
		return fmt.Sprintf("%d", id)
	case float64:
		return formatFloat64ID(id)
	default:
		return ""
	}
}

// Validate checks if the JSON-RPC response conforms to the JSON-RPC specification.
func (r *Response) Validate() error {
	if r == nil {
		return errors.New("response is nil")
	}

	if r.jsonrpc != jsonRPCVersion {
		return fmt.Errorf("invalid jsonrpc version: %s", r.jsonrpc)
	}

	// Normalize int to int64 for consistency
	switch v := r.id.(type) {
	case nil, string, int64, float64:
		// Already in correct format
	case int:
		// Convert platform-dependent int to int64
		r.id = int64(v)
	default:
		return errors.New("id field must be a string or a number")
	}

	if r.err != nil && r.result != nil || r.rawError != nil && r.result != nil {
		return errors.New("response must not contain both result and error")
	}
	if r.err == nil && len(r.rawError) == 0 && r.result == nil {
		return errors.New("response must contain either result or error")
	}

	return nil
}

// Equals compares the contents of two JSON-RPC responses.
// This method handles both eagerly and lazily unmarshaled responses by ensuring
// both IDs and Errors are unmarshaled before comparison.
func (r *Response) Equals(other *Response) bool {
	if r == nil || other == nil {
		return false
	}
	if r.jsonrpc != other.jsonrpc {
		return false
	}

	// Ensure both IDs are unmarshaled before comparing (if they have rawID set)
	// IDOrNil() uses sync.Once internally to unmarshal lazily
	rID := r.IDOrNil()
	otherID := other.IDOrNil()

	if rID != otherID {
		return false
	}

	// Ensure both errors are unmarshaled before comparing
	_ = r.UnmarshalError()
	_ = other.UnmarshalError()

	if !r.err.Equals(other.err) {
		return false
	}

	if r.result != nil && other.result != nil {
		if string(r.result) != string(other.result) {
			return false
		}
	}

	return true
}

// IsEmpty returns whether the JSON-RPC response can be considered empty.
//
// This method is primarily used to detect responses that carry no meaningful data, such as
// responses from notification requests (which shouldn't exist per spec) or placeholder responses.
//
// A response is considered empty when BOTH the error and result are empty:
//   - Result is empty if: empty byte slice, null, empty string (""), empty array ([]),
//     empty object ({}), or hex zero value ("0x")
//   - Error is empty if: nil, or has both code=0 and message=""
//
// The specific byte pattern checks (null, "0x", etc.) handle common JSON-RPC conventions
// where these values represent "no data" semantically.
func (r *Response) IsEmpty() bool {
	if r == nil {
		return true
	}

	// Case: both error and result are empty
	if r.err == nil && len(r.result) == 0 {
		return true
	}

	emptyError := r.err.IsEmpty()
	emptyResult := isEmptyResult(r.result)

	return emptyError && emptyResult
}

// String returns a string representation of the JSON-RPC response.
func (r *Response) String() string {
	return fmt.Sprintf("ID: %v, Error: %v, Result byte size: %d", r.id, r.err, len(r.result))
}

// isEmptyResult checks if a raw JSON result is considered empty.
// Returns true for: empty slice, null, "0x", "", [], {}
func isEmptyResult(result json.RawMessage) bool {
	resultBytes := len(result)
	if resultBytes == 0 {
		return true
	}

	// Check for "0x" (hex zero value)
	if resultBytes == 4 && result[0] == '"' && result[1] == '0' &&
		result[2] == 'x' && result[3] == '"' {
		return true
	}

	// Check for null
	if resultBytes == 4 && result[0] == 'n' && result[1] == 'u' &&
		result[2] == 'l' && result[3] == 'l' {
		return true
	}

	// Check for empty string, array, or object
	if resultBytes == 2 {
		if (result[0] == '"' && result[1] == '"') ||
			(result[0] == '[' && result[1] == ']') ||
			(result[0] == '{' && result[1] == '}') {
			return true
		}
	}

	return false
}

// DecodeResponse parses and returns a new Response from a byte slice.
func DecodeResponse(data []byte) (*Response, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, errors.New(errEmptyData)
	}

	resp := &Response{}
	if err := resp.parseFromBytes(data); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// If the response carries an error (and no result), decode it eagerly so callers
	// can inspect *Response.err without an extra step.
	if len(resp.result) == 0 && len(resp.rawError) > 0 {
		resp.err = &Error{}
		if err := resp.err.UnmarshalJSON(resp.rawError); err != nil {
			return nil, fmt.Errorf("failed to unmarshal JSON-RPC error: %w", err)
		}
	}

	return resp, nil
}

// DecodeResponseFromReader parses and returns a new Response from an io.Reader.
// expectedSize is optional and used for internal buffer sizing; pass 0 if unknown.
func DecodeResponseFromReader(r io.Reader, expectedSize int) (*Response, error) {
	if r == nil {
		return nil, errors.New("cannot read from nil reader")
	}
	resp := &Response{}
	if err := resp.parseFromReader(r, expectedSize); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return resp, nil
}

// NewResponse creates a JSON-RPC 2.0 response with a result.
func NewResponse(id any, result any) (*Response, error) {
	resultBytes, err := getSonicAPI().Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal result: %w", err)
	}

	// Pre-marshal the ID to cache it for later use
	var rawID json.RawMessage
	if id != nil {
		idBytes, err := getSonicAPI().Marshal(id)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal id: %w", err)
		}
		rawID = idBytes
	}

	return &Response{
		jsonrpc: jsonRPCVersion,
		id:      id,
		rawID:   rawID,
		result:  resultBytes,
	}, nil
}

// NewResponseFromRaw creates a JSON-RPC 2.0 response with a raw result.
func NewResponseFromRaw(id any, rawResult json.RawMessage) (*Response, error) {
	// Pre-marshal the ID to cache it for later use
	var rawID json.RawMessage
	if id != nil {
		idBytes, err := getSonicAPI().Marshal(id)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal id: %w", err)
		}
		rawID = idBytes
	}

	return &Response{
		jsonrpc: jsonRPCVersion,
		id:      id,
		rawID:   rawID,
		result:  rawResult,
	}, nil
}

// NewErrorResponse creates a JSON-RPC 2.0 error response.
func NewErrorResponse(id any, err *Error) *Response {
	// Pre-marshal the ID to cache it for later use
	var rawID json.RawMessage
	if id != nil {
		idBytes, marshalErr := getSonicAPI().Marshal(id)
		if marshalErr == nil {
			rawID = idBytes
		}
		// If marshal fails, rawID remains nil and MarshalJSON will handle it
	}

	return &Response{
		jsonrpc: jsonRPCVersion,
		id:      id,
		rawID:   rawID,
		err:     err,
	}
}
