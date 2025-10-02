// Package jsonrpc provides a Go implementation of the JSON-RPC 2.0 specification, as well as tools
// to parse and work with JSON-RPC requests and responses.
package jsonrpc

import (
	"bytes"
	"encoding/json" // Used for json.RawMessage type, which provides interop with stdlib encoding/json
	"errors"
	"fmt"

	"github.com/bytedance/sonic" // Primary JSON parser for performance
)

// Request is a struct for a JSON-RPC request. It conforms to the JSON-RPC 2.0 specification, with
// minor exceptions. E.g. the ID field is allowed to be fractional in this implementation.
// Note: to ensure proper conformance, use the provided constructors and methods.
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
	return sonic.Marshal((*alias)(r))
}

// String returns a string representation of the JSON-RPC request.
// Note: implements the fmt.Stringer interface.
func (r *Request) String() string {
	return fmt.Sprintf("ID: %v, Method: %s", r.ID, r.Method)
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
	if err := sonic.Unmarshal(data, &aux); err != nil {
		return err
	}

	if aux.JSONRPC != "2.0" {
		return fmt.Errorf("jsonrpc field is required to be exactly \"2.0\"")
	}
	r.JSONRPC = aux.JSONRPC

	if aux.Method == "" {
		return errors.New("method field is required")
	}
	r.Method = aux.Method

	// Unmarshal and validate the id field
	if len(aux.ID) > 0 {
		var id any
		if err := sonic.Unmarshal(aux.ID, &id); err != nil {
			return fmt.Errorf("invalid id field: %w", err)
		}
		// If the value is "null", id will be nil
		if id == nil {
			r.ID = nil
		} else {
			switch v := id.(type) {
			case float64:
				// JSON numbers are unmarshalled as float64, so an explicit integer check is needed
				if v != float64(int64(v)) {
					r.ID = v
				} else {
					r.ID = int64(v)
				}
			case string:
				if v == "" {
					r.ID = nil
				} else {
					r.ID = v
				}
			default:
				return errors.New("id field must be a string or a number")
			}
		}
	} else {
		r.ID = nil
	}

	// Unmarshal the params field
	if len(aux.Params) > 0 {
		var rawParams any
		if err := sonic.Unmarshal(aux.Params, &rawParams); err != nil {
			return fmt.Errorf("invalid params field: %w", err)
		}
		// Accept only arrays or objects.
		switch rawParams.(type) {
		case []any, map[string]any, nil:
			r.Params = rawParams
		case string:
			// Treat empty strings as nil
			if rawParams == "" {
				r.Params = nil
			} else {
				return errors.New("params field must be either an array, an object, or nil")
			}
		default:
			return errors.New("params field must be either an array, an object, or nil")
		}
	} else {
		// You may choose to set this to nil or an empty value.
		r.Params = nil
	}

	return nil
}

// Validate checks if the JSON-RPC request conforms to the JSON-RPC specification.
func (r *Request) Validate() error {
	if r == nil {
		return errors.New("request is nil")
	}
	if r.JSONRPC != "2.0" {
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
		JSONRPC: "2.0",
		ID:      RandomJSONRPCID(),
		Method:  method,
		Params:  params,
	}
}

// NewRequestWithID creates a JSON-RPC 2.0 request with a specific ID.
func NewRequestWithID(method string, params any, id any) *Request {
	return &Request{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}
}

// NewNotification creates a JSON-RPC 2.0 notification (request without ID).
func NewNotification(method string, params any) *Request {
	return &Request{
		JSONRPC: "2.0",
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
	paramBytes, err := sonic.Marshal(r.Params)
	if err != nil {
		return fmt.Errorf("failed to marshal params: %w", err)
	}

	return sonic.Unmarshal(paramBytes, dst)
}

// DecodeRequest parses a JSON-RPC request from a byte slice.
func DecodeRequest(data []byte) (*Request, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, fmt.Errorf("empty data")
	}
	req := &Request{}
	err := req.UnmarshalJSON(data)
	if err != nil {
		return nil, err
	}
	return req, nil
}

// RequestFromBytes creates a JSON-RPC request from a byte slice.
// Deprecated: Use DecodeRequest instead. See MIGRATION.md for details. Will be removed in v2.0.
func RequestFromBytes(data []byte) (*Request, error) {
	return DecodeRequest(data)
}
