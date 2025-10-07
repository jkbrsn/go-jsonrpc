// Package jsonrpc provides a Go implementation of the JSON-RPC 2.0 specification, as well as tools
// to parse and work with JSON-RPC requests and responses.
package jsonrpc

import (
	"bytes"
	// Used for json.RawMessage type, which provides interop with stdlib encoding/json
	"encoding/json"
	"errors"
	"fmt"
)

// Request is a struct for a JSON-RPC request. It conforms to the JSON-RPC 2.0 specification, with
// minor exceptions. E.g. the ID field is allowed to be fractional in this implementation.
type Request struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

// IDString returns the ID as a string.
func (r *Request) IDString() string {
	switch id := r.ID.(type) {
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

// IsEmpty returns whether the Request can be considered empty. A request is considered empty if
// the method field is empty.
func (r *Request) IsEmpty() bool {
	if r == nil {
		return true
	}

	if r.Method == "" {
		return true
	}

	return false
}

// MarshalJSON marshals a JSON-RPC request.
func (r *Request) MarshalJSON() ([]byte, error) {
	err := r.Validate()
	if err != nil {
		return nil, err
	}

	type alias Request // Avoid infinite recursion by using an alias
	return getSonicAPI().Marshal((*alias)(r))
}

// String returns a string representation of the JSON-RPC request.
// Note: implements the fmt.Stringer interface.
func (r *Request) String() string {
	return fmt.Sprintf("ID: %v, Method: %s", r.ID, r.Method)
}

// unmarshalRequestID unmarshals and normalizes the ID field from raw JSON.
func unmarshalRequestID(rawID json.RawMessage) (any, error) {
	if len(rawID) == 0 {
		return nil, nil
	}

	var id any
	if err := getSonicAPI().Unmarshal(rawID, &id); err != nil {
		return nil, fmt.Errorf("invalid id field: %w", err)
	}

	// If the value is "null", id will be nil
	if id == nil {
		return nil, nil
	}

	switch v := id.(type) {
	case float64:
		// JSON numbers are unmarshalled as float64, so an explicit integer check is needed
		if v != float64(int64(v)) {
			return v, nil
		}
		return int64(v), nil
	case string:
		if v == "" {
			return nil, nil
		}
		return v, nil
	default:
		return nil, errors.New("id field must be a string or a number")
	}
}

// unmarshalRequestParams unmarshals and validates the params field from raw JSON.
func unmarshalRequestParams(rawParams json.RawMessage) (any, error) {
	if len(rawParams) == 0 {
		return nil, nil
	}

	var params any
	if err := getSonicAPI().Unmarshal(rawParams, &params); err != nil {
		return nil, fmt.Errorf("invalid params field: %w", err)
	}

	// Accept only arrays or objects.
	switch params.(type) {
	case []any, map[string]any, nil:
		return params, nil
	case string:
		// Treat empty strings as nil
		if params != "" {
			return nil, errors.New("params field must be either an array, an object, or nil")
		}
		return nil, nil
	default:
		return nil, errors.New("params field must be either an array, an object, or nil")
	}
}

// UnmarshalJSON unmarshals a JSON-RPC request. The function takes two custom actions; sets the
// JSON-RPC version to 2.0 and unmarshals the ID separately, to handle both string and float64 IDs.
func (r *Request) UnmarshalJSON(data []byte) error {
	// Define an auxiliary type that maps to the JSON-RPC request structure, but with raw fields
	type requestAux struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Method  string          `json:"method"`
		Params  json.RawMessage `json:"params,omitempty"`
	}

	var aux requestAux
	if err := getSonicAPI().Unmarshal(data, &aux); err != nil {
		return err
	}

	if aux.JSONRPC != jsonRPCVersion {
		return errors.New("jsonrpc field is required to be exactly \"2.0\"")
	}
	r.JSONRPC = aux.JSONRPC

	if aux.Method == "" {
		return errors.New("method field is required")
	}
	r.Method = aux.Method

	// Unmarshal and validate the id field
	id, err := unmarshalRequestID(aux.ID)
	if err != nil {
		return err
	}
	r.ID = id

	// Unmarshal and validate the params field
	params, err := unmarshalRequestParams(aux.Params)
	if err != nil {
		return err
	}
	r.Params = params

	return nil
}

// Validate checks if the JSON-RPC request conforms to the JSON-RPC specification.
func (r *Request) Validate() error {
	if r == nil {
		return errors.New("request is nil")
	}
	if r.JSONRPC != jsonRPCVersion {
		return errors.New("jsonrpc field is required to be exactly \"2.0\"")
	}
	if r.Method == "" {
		return errors.New("method field is required")
	}

	// Check for reserved "rpc." prefix (JSON-RPC 2.0 spec)
	if len(r.Method) >= 4 && r.Method[:4] == "rpc." {
		return errors.New("method names starting with 'rpc.' are reserved by JSON-RPC 2.0 spec")
	}

	switch r.ID.(type) {
	case nil, string, int64, float64:
	default:
		return errors.New("id field must be a string or a number")
	}
	switch r.Params.(type) {
	case nil, []any, map[string]any:
	default:
		return errors.New("params field must be either an array, an object, or nil")
	}

	return nil
}

// NewRequest creates a JSON-RPC 2.0 request with an auto-generated ID.
func NewRequest(method string, params any) *Request {
	return &Request{
		JSONRPC: jsonRPCVersion,
		ID:      RandomJSONRPCID(),
		Method:  method,
		Params:  params,
	}
}

// NewRequestWithID creates a JSON-RPC 2.0 request with a specific ID.
func NewRequestWithID(method string, params any, id any) *Request {
	return &Request{
		JSONRPC: jsonRPCVersion,
		ID:      id,
		Method:  method,
		Params:  params,
	}
}

// NewNotification creates a JSON-RPC 2.0 notification (request without ID).
func NewNotification(method string, params any) *Request {
	return &Request{
		JSONRPC: jsonRPCVersion,
		Method:  method,
		Params:  params,
	}
}

// IsNotification returns true if this is a notification (no ID expected).
func (r *Request) IsNotification() bool {
	return r.ID == nil
}

// UnmarshalParams decodes the Params field into the provided destination pointer.
// This is a convenience method for unmarshaling structured parameters.
func (r *Request) UnmarshalParams(dst any) error {
	if dst == nil {
		return errors.New("destination pointer cannot be nil")
	}

	if r.Params == nil {
		return errors.New("request has no params field")
	}

	// Marshal params back to JSON, then unmarshal into destination
	// This handles the conversion from any ([]any or map[string]any) to the target type
	paramBytes, err := getSonicAPI().Marshal(r.Params)
	if err != nil {
		return fmt.Errorf("failed to marshal params: %w", err)
	}

	return getSonicAPI().Unmarshal(paramBytes, dst)
}

// DecodeRequest parses a JSON-RPC request from a byte slice.
func DecodeRequest(data []byte) (*Request, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, errors.New(errEmptyData)
	}
	req := &Request{}
	err := req.UnmarshalJSON(data)
	if err != nil {
		return nil, err
	}
	return req, nil
}
