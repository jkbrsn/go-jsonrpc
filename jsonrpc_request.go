package jsonrpc

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/bytedance/sonic"
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

// RequestFromBytes creates a JSON-RPC request from a byte slice.
func RequestFromBytes(data []byte) (*Request, error) {
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
