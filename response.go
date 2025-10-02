package jsonrpc

import (
	"bytes"
	"encoding/json" // Used for json.RawMessage type
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/bytedance/sonic" // Primary JSON parser for performance
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

	// Internal fields for lazy unmarshaling
	rawID    json.RawMessage
	rawError json.RawMessage

	// One-time initialization guards for lazy operations
	idOnce  sync.Once
	errOnce sync.Once
}

// jsonRPCResponse is an internal representation of a JSON-RPC response.
// This is decoupled from the public struct to allow for custom handling of the response data,
// separately from how it is marshaled and unmarshaled.
type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Error   *Error          `json:"error,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
}

// Getter methods for Response fields

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

// parseFromReader parses a JSON-RPC response from a reader.
func (r *Response) parseFromReader(reader io.Reader, expectedSize int) error {
	// 16KB chunks by default
	chunkSize := defaultChunkSize
	data, err := readAll(reader, int64(chunkSize), expectedSize)
	if err != nil {
		return err
	}

	return r.parseFromBytes(data)
}

// parseFromBytes parses a JSON-RPC response from a byte slice. This function does not unmarshal
// the []byte data of the error or the result, it only stores the raw slices in the Response, to
// allow for any unmarshalling to occur at the caller's discretion.
func (r *Response) parseFromBytes(data []byte) error {
	// Define an auxiliary struct that maps directly to the JSON-RPC response structure
	type jsonRPCResponseAux struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Result  json.RawMessage `json:"result,omitempty"`
		Error   json.RawMessage `json:"error,omitempty"`
	}

	var aux jsonRPCResponseAux
	if err := sonic.Unmarshal(data, &aux); err != nil {
		return err
	}

	// Validate JSON-RPC version
	if aux.JSONRPC != jsonRPCVersion {
		return fmt.Errorf("invalid JSON-RPC version: %s", aux.JSONRPC)
	}
	r.jsonrpc = aux.JSONRPC

	// Validate that either result or error is present
	resultExists := len(aux.Result) > 0
	errorExists := len(aux.Error) > 0

	if !resultExists && !errorExists {
		return errors.New("response must contain either result or error")
	}
	if resultExists && errorExists {
		return errors.New("response must not contain both result and error")
	}

	// Parse the ID field
	r.rawID = aux.ID

	// Also unmarshal the ID, as the ID field is imperative for use of the
	// Response
	if err := r.unmarshalID(); err != nil {
		return fmt.Errorf("failed to unmarshal ID: %w", err)
	}

	// Assign result or error accordingly
	if aux.Result != nil {
		r.result = aux.Result
	} else {
		r.rawError = aux.Error
	}

	// Validate the response
	if err := r.Validate(); err != nil {
		return fmt.Errorf("failed to parse JSON-RPC response: %w", err)
	}

	return nil
}

// unmarshalID unmarshals the raw ID bytes into the ID field.
// This function is designed to be called via sync.Once to ensure it runs exactly once.
func (r *Response) unmarshalID() error {
	// If there's no rawID to unmarshal, leave ID field as-is (may be nil or already set)
	if len(r.rawID) == 0 {
		return nil
	}

	var id any
	if err := sonic.Unmarshal(r.rawID, &id); err != nil {
		return fmt.Errorf("invalid id field: %w", err)
	}

	// If the value is "null", id will be nil
	if id == nil {
		r.id = nil
		return nil
	}

	switch v := id.(type) {
	case float64:
		// JSON numbers are unmarshalled as float64, so an explicit integer check is needed
		if v != float64(int64(v)) {
			r.id = v
		} else {
			r.id = int64(v)
		}
	case string:
		if v == "" {
			r.id = nil
		} else {
			r.id = v
		}
	default:
		return errors.New("id field must be a string or a number")
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

// IDRaw returns the unmarshaled ID, or nil if unmarshaling fails.
// Deprecated: Use IDOrNil instead for clearer intent. See MIGRATION.md for details.
func (r *Response) IDRaw() any {
	return r.IDOrNil()
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

// MarshalJSON marshals a JSON-RPC response into a byte slice. The public members ID and Error
// will be prioritized over their raw counterparts.
func (r *Response) MarshalJSON() ([]byte, error) {
	err := r.Validate()
	if err != nil {
		return nil, err
	}

	// Retrieve the ID value
	var id any
	if r.id != nil {
		id = r.id
	} else if r.rawID != nil {
		id = r.rawID
	} else {
		id = nil
	}

	// Retrieve the error value
	// If rawError exists but Error hasn't been unmarshaled, do it now
	if len(r.rawError) > 0 && r.err == nil {
		r.err = &Error{}
		if err := r.err.UnmarshalJSON(r.rawError); err != nil {
			return nil, fmt.Errorf("failed to unmarshal JSON-RPC error: %w", err)
		}
	}
	errVal := r.err

	// Retrieve the result
	// Since it is already a JSON encoded []byte, we wrap it as json.RawMessage to prevent sonic
	// from re-encoding it.
	var result json.RawMessage
	if len(r.result) > 0 {
		result = json.RawMessage(r.result)
	}

	// Build the output struct. Fields with zero values are omitted.
	output := jsonRPCResponse{
		JSONRPC: r.jsonrpc,
		ID:      id,
		Error:   errVal,
		Result:  result,
	}

	marshaled, err := sonic.Marshal(output)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal JSON-RPC response: %w", err)
	}

	return marshaled, nil
}

// String returns a string representation of the JSON-RPC response.
func (r *Response) String() string {
	return fmt.Sprintf("ID: %v, Error: %v, Result byte size: %d", r.id, r.err, len(r.result))
}

// UnmarshalError unmarshals the raw error into the Error field.
// The error is unmarshaled lazily on first call and cached for subsequent calls.
// This method is safe for concurrent use.
func (r *Response) UnmarshalError() error {
	var unmarshalErr error

	r.errOnce.Do(func() {
		if r.err == nil && len(r.rawError) > 0 {
			r.err = &Error{}
			unmarshalErr = r.err.UnmarshalJSON(r.rawError)
		}
	})

	return unmarshalErr
}

// UnmarshalJSON unmarshals the input data into the members of Response. Note: does not unmarshal
// the Result field, but leaves that at the caller's discretion (UnmarshalResult). This is an
// optimization to prevent unnecessary unmarshalling of the Result field for very large blobs.
func (r *Response) UnmarshalJSON(data []byte) error {
	// Use the core parsing routine
	if err := r.parseFromBytes(data); err != nil {
		return fmt.Errorf("failed to unmarshal JSON-RPC response: %w", err)
	}

	// If the response carries an error (and no result), decode it eagerly so callers
	// can inspect *Response.Error without an extra step.
	if len(r.result) == 0 {
		if r.err == nil && len(r.rawError) > 0 {
			r.err = &Error{}
			if err := r.err.UnmarshalJSON(r.rawError); err != nil {
				return fmt.Errorf("failed to unmarshal JSON-RPC error: %w", err)
			}
		}
	}

	return nil
}

// UnmarshalResult decodes the raw Result field into the provided destination pointer.
func (r *Response) UnmarshalResult(dst any) error {
	if dst == nil {
		return errors.New("destination pointer cannot be nil")
	}

	if len(r.result) == 0 {
		return errors.New("response has no result field")
	}

	return sonic.Unmarshal(r.result, dst)
}

// Validate checks if the JSON-RPC response conforms to the JSON-RPC specification.
func (r *Response) Validate() error {
	if r == nil {
		return errors.New("response is nil")
	}

	if r.jsonrpc != jsonRPCVersion {
		return fmt.Errorf("invalid jsonrpc version: %s", r.jsonrpc)
	}

	switch r.id.(type) {
	case nil, string, int64, float64:
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
	resultBytes, err := sonic.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal result: %w", err)
	}

	return &Response{
		jsonrpc: jsonRPCVersion,
		id:      id,
		result:  resultBytes,
	}, nil
}

// NewResponseFromRaw creates a JSON-RPC 2.0 response with a raw result.
func NewResponseFromRaw(id any, rawResult json.RawMessage) (*Response, error) {
	return &Response{
		jsonrpc: jsonRPCVersion,
		id:      id,
		result:  rawResult,
	}, nil
}

// NewErrorResponse creates a JSON-RPC 2.0 error response.
func NewErrorResponse(id any, err *Error) *Response {
	return &Response{
		jsonrpc: jsonRPCVersion,
		id:      id,
		err:     err,
	}
}

// NewResponseFromBytes parses and returns a new Response from a byte slice.
// Deprecated: Use DecodeResponse instead. See MIGRATION.md for details. Will be removed in v2.0.
func NewResponseFromBytes(data []byte) (*Response, error) {
	return DecodeResponse(data)
}

// NewResponseFromStream parses and returns a new Response from a stream.
// Deprecated: Use DecodeResponseFromReader instead. Note that DecodeResponseFromReader
// does NOT automatically close the reader. See MIGRATION.md for details. Will be removed in v2.0.
func NewResponseFromStream(body io.ReadCloser, expectedSize int) (*Response, error) {
	if body == nil {
		return nil, errors.New("cannot read from nil reader")
	}
	defer body.Close() // nolint:errcheck

	return DecodeResponseFromReader(body, expectedSize)
}
