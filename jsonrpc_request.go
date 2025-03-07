package jsonrpc

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/bytedance/sonic"
)

// TODO: add a method to validate the request
// TODO: add a method to marshal the request into a byte slice

// Request is a struct for JSON RPC requests.
type Request struct {
	JSONRPC string `json:"jsonrpc,omitempty"`
	ID      any    `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params"`
}

// IDString returns the ID as a string.
func (r *Request) IDString() string {
	switch id := r.ID.(type) {
	case string:
		return id
	case int64:
		return fmt.Sprintf("%d", id)
	default:
		return ""
	}
}

// IsEmpty returns whether the Request can be considered empty.
func (r *Request) IsEmpty() bool {
	if r == nil {
		return true
	}

	if r.Method == "" {
		return true
	}

	return false
}

// String returns a string representation of the JSON RPC request.
// Note: implements the fmt.Stringer interface.
func (r *Request) String() string {
	return fmt.Sprintf("ID: %v, Method: %s", r.ID, r.Method)
}

// UnmarshalJSON unmarshals a JSON RPC request using sonic. It includes two custom actions:
// - Sets the JSON RPC version to 2.0.
// - Unmarshals the ID separately, to handle both string and float64 types.
func (r *Request) UnmarshalJSON(data []byte) error {
	// Define an auxiliary struct that maps directly to the JSON RPC request structure
	// TODO: is this auxiliary struct necessary?
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
		return fmt.Errorf("invalid jsonrpc version: expected \"2.0\", got %q", aux.JSONRPC)
	}
	r.JSONRPC = aux.JSONRPC

	if aux.Method == "" {
		return errors.New("missing required field: method")
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
				r.ID = int64(v)
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
	}
	if r.ID == nil {
		// Set an ID to treat this as a call even though it might be a notification,
		// as the ID field is required for some downstream services.
		r.ID = randomJSONRPCID()
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
		default:
			return errors.New("params field must be either an array, an object, or nil")
		}
	} else {
		// You may choose to set this to nil or an empty value.
		r.Params = nil
	}

	return nil
}

// RequestFromBytes creates a JSON RPC request from a byte slice.
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
