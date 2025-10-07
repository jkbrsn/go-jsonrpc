package jsonrpc

import (
	"bytes"
	"encoding/json" // Used for json.RawMessage type
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/bytedance/sonic"     // Primary JSON parser for performance
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
	if len(r.rawID) > 0 {
		id = r.rawID
	} else if r.id != nil {
		id = r.id
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

// Unmarshal decodes the entire JSON-RPC response into the provided destination pointer.
// This includes all fields: jsonrpc, id, result (if present), and error (if present).
//
// Note: This method is not optimized. It performs a marshal-then-unmarshal round trip. For
// performance-critical code, consider using the individual getter methods.
func (r *Response) Unmarshal(dst any) error {
	if dst == nil {
		return errors.New("destination pointer cannot be nil")
	}

	data, err := r.MarshalJSON()
	if err != nil {
		return err
	}

	return sonic.Unmarshal(data, dst)
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

// WriteTo implements io.WriterTo for efficient streaming serialization without buffering
// the entire response in memory. This is particularly beneficial for large responses
// (e.g., getLogs with 10k+ events) as it significantly reduces memory pressure.
//
// The method writes the JSON-RPC response directly to the provided writer, streaming
// each field without intermediate allocations. For large result payloads, this avoids
// the memory overhead of MarshalJSON which allocates a full buffer before writing.
//
// Example usage:
//
//	var buf bytes.Buffer
//	n, err := response.WriteTo(&buf)
//	// or directly to http.ResponseWriter
//	n, err := response.WriteTo(w)
func (r *Response) WriteTo(w io.Writer) (n int64, err error) {
	if err := r.Validate(); err != nil {
		return 0, err
	}

	var total int64

	// Write opening brace and jsonrpc field
	if err = writeString(w, `{"jsonrpc":"2.0","id":`, &total); err != nil {
		return total, err
	}

	// Write ID field
	idBytes, err := r.getIDBytes()
	if err != nil {
		return total, err
	}
	if err = writeBytes(w, idBytes, &total); err != nil {
		return total, err
	}

	// Write either error or result field
	if r.err != nil || len(r.rawError) > 0 {
		if err = r.writeErrorField(w, &total); err != nil {
			return total, err
		}
	} else {
		if err = r.writeResultField(w, &total); err != nil {
			return total, err
		}
	}

	// Write closing brace
	if err = writeString(w, `}`, &total); err != nil {
		return total, err
	}

	return total, nil
}

// writeString writes a string to the writer and updates the total byte count
func writeString(w io.Writer, s string, total *int64) error {
	return writeBytes(w, []byte(s), total)
}

// writeBytes writes bytes to the writer and updates the total byte count
func writeBytes(w io.Writer, b []byte, total *int64) error {
	written, err := w.Write(b)
	*total += int64(written)
	return err
}

// getIDBytes returns the marshaled ID bytes. Uses cached rawID if available to avoid re-marshaling.
func (r *Response) getIDBytes() ([]byte, error) {
	if len(r.rawID) > 0 {
		return r.rawID, nil
	}
	if r.id != nil {
		return sonic.Marshal(r.id)
	}
	return []byte("null"), nil
}

// writeErrorField writes the error field to the writer
func (r *Response) writeErrorField(w io.Writer, total *int64) error {
	if err := writeString(w, `,"error":`, total); err != nil {
		return err
	}

	errorBytes, err := r.getErrorBytes()
	if err != nil {
		return err
	}

	return writeBytes(w, errorBytes, total)
}

// getErrorBytes returns the marshaled error bytes
func (r *Response) getErrorBytes() ([]byte, error) {
	if r.err != nil {
		return sonic.Marshal(r.err)
	}
	return r.rawError, nil
}

// writeResultField writes the result field to the writer
func (r *Response) writeResultField(w io.Writer, total *int64) error {
	if err := writeString(w, `,"result":`, total); err != nil {
		return err
	}
	return writeBytes(w, r.result, total)
}

// buildASTNode lazily builds the AST node for the result field.
// This is called via sync.Once to ensure it runs exactly once.
func (r *Response) buildASTNode() {
	if len(r.result) == 0 {
		r.astErr = errors.New("response has no result field")
		return
	}

	// Parse the result field into an AST node
	// The result field has already been validated during decode
	node, err := ast.NewSearcher(string(r.result)).GetByPath()
	if err != nil {
		r.astErr = fmt.Errorf("failed to build AST node: %w", err)
		return
	}
	r.astNode = node
}

// getASTNode returns the cached AST node, building it if necessary.
// Thread-safe via sync.Once and RWMutex.
func (r *Response) getASTNode() (ast.Node, error) {
	r.astOnce.Do(r.buildASTNode)

	r.astMutex.RLock()
	defer r.astMutex.RUnlock()

	if r.astErr != nil {
		return ast.Node{}, r.astErr
	}

	return r.astNode, nil
}

// PeekStringByPath traverses the result JSON using sonic's AST to extract a string field without
// unmarshaling the entire result. This is valuable for large responses where you only need to
// access specific nested fields.
//
// The path is specified as a sequence of keys for nested objects. For example, to extract
// the "blockNumber" field from a result like {"blockNumber": "0x1234", ...}, use:
//
//	blockNum, err := response.PeekStringByPath("blockNumber")
//
// For nested paths, provide multiple arguments:
//
//	from, err := response.PeekStringByPath("transaction", "from")
//
// The AST node is lazily built on first call and cached for subsequent calls, making
// repeated field access very efficient.
//
// Returns an error if:
//   - The response has no result field (only error)
//   - The specified path does not exist
//   - The value at the path is not a string
//   - AST parsing fails
func (r *Response) PeekStringByPath(path ...any) (string, error) {
	node, err := r.getASTNode()
	if err != nil {
		return "", err
	}

	// Navigate to the requested path
	if len(path) > 0 {
		targetNode := node.GetByPath(path...)
		if targetNode == nil || !targetNode.Valid() {
			return "", errors.New("path not found")
		}
		node = *targetNode
	}

	// Extract string value
	str, err := node.String()
	if err != nil {
		return "", fmt.Errorf("value at path is not a string: %w", err)
	}

	return str, nil
}

// PeekBytesByPath returns raw JSON bytes for a nested field without unmarshaling the entire result.
// This is useful when you want to extract a sub-object or array and unmarshal it separately.
//
// Example usage to extract a nested transaction object:
//
//	txBytes, err := response.PeekBytesByPath("transaction")
//	if err != nil {
//	    return err
//	}
//	var tx Transaction
//	err = sonic.Unmarshal(txBytes, &tx)
//
// The returned bytes are valid JSON that can be unmarshaled into any type.
//
// Returns an error if:
//   - The response has no result field (only error)
//   - The specified path does not exist
//   - AST parsing fails
func (r *Response) PeekBytesByPath(path ...any) ([]byte, error) {
	node, err := r.getASTNode()
	if err != nil {
		return nil, err
	}

	// Navigate to the requested path
	if len(path) > 0 {
		targetNode := node.GetByPath(path...)
		if targetNode == nil || !targetNode.Valid() {
			return nil, errors.New("path not found")
		}
		node = *targetNode
	}

	// Get raw JSON bytes
	raw, err := node.Raw()
	if err != nil {
		return nil, fmt.Errorf("failed to get raw bytes: %w", err)
	}

	return []byte(raw), nil
}

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

const (
	// Size estimation constants
	jsonStructureOverhead  = 35 // {"jsonrpc":"2.0","id":,"result":}
	errorStructureOverhead = 20 // {"code":,"message":""}
	errorDataEstimate      = 50 // rough estimate for error data field
	float64SizeEstimate    = 12 // float64 average size (range: 5-23 bytes)
	nullSize               = 4  // "null"
	decimalBase            = 10 // base 10 for digit counting
)

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

	// Pre-marshal the ID to cache it for later use
	var rawID json.RawMessage
	if id != nil {
		idBytes, err := sonic.Marshal(id)
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
		idBytes, err := sonic.Marshal(id)
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
		idBytes, marshalErr := sonic.Marshal(id)
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
