package jsonrpc

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/bytedance/sonic"
)

// Response is a struct for JSON-RPC responses.
type Response struct {
	id      any
	idBytes []byte
	muID    sync.RWMutex

	Error    *Error
	errBytes []byte
	muErr    sync.RWMutex

	Result   json.RawMessage
	muResult sync.RWMutex
}

// jsonRPCResponse is an internal representation of a JSON-RPC response.
// This is decoupled from the public struct to allow for custom handling of the response data,
// separately from how it is marshaled and unmarshaled.
type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Error   *Error          `json:"error,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
}

// Equals compares the contents of two JSON-RPC responses.
func (r *Response) Equals(other *Response) bool {
	if r == nil || other == nil {
		return false
	}

	if r.Error != nil && other.Error != nil {
		if r.Error.Code != other.Error.Code || r.Error.Message != other.Error.Message {
			return false
		}
	}

	if r.Result != nil && other.Result != nil {
		if string(r.Result) != string(other.Result) {
			return false
		}
	}

	return true
}

// ID returns the ID of the JSON-RPC response.
func (r *Response) ID() any {
	r.muID.RLock()

	if r.id != nil {
		r.muID.RUnlock()
		return r.id
	}
	r.muID.RUnlock()

	r.muID.Lock()
	defer r.muID.Unlock()

	if len(r.idBytes) == 0 {
		return nil
	}

	err := sonic.Unmarshal(r.idBytes, &r.id)
	if err != nil {
		return nil
	}

	return r.id
}

// IDString returns the ID as a string.
func (r *Response) IDString() string {
	switch id := r.ID().(type) {
	case string:
		return id
	case int64:
		return fmt.Sprintf("%d", id)
	case float64:
		return strings.Trim(fmt.Sprintf("%f", id), "0")
	default:
		return ""
	}
}

// IsEmpty returns whether the JSON-RPC response can be considered empty.
func (r *Response) IsEmpty() bool {
	if r == nil {
		return true
	}

	r.muResult.RLock()
	defer r.muResult.RUnlock()

	lnr := len(r.Result)
	if lnr == 0 ||
		(lnr == 4 && r.Result[0] == '"' && r.Result[1] == '0' && r.Result[2] == 'x' && r.Result[3] == '"') ||
		(lnr == 4 && r.Result[0] == 'n' && r.Result[1] == 'u' && r.Result[2] == 'l' && r.Result[3] == 'l') ||
		(lnr == 2 && r.Result[0] == '"' && r.Result[1] == '"') ||
		(lnr == 2 && r.Result[0] == '[' && r.Result[1] == ']') ||
		(lnr == 2 && r.Result[0] == '{' && r.Result[1] == '}') {
		return true
	}

	return false
}

// IsNull determines if the JSON-RPC response is null.
func (r *Response) IsNull() bool {
	if r == nil {
		return true
	}

	r.muResult.RLock()
	defer r.muResult.RUnlock()

	r.muErr.RLock()
	defer r.muErr.RUnlock()

	if len(r.Result) == 0 && r.Error == nil && r.ID() == nil {
		return true
	}

	return false
}

// MarshalJSON marshals a JSON-RPC response into a byte slice.
func (r *Response) MarshalJSON() ([]byte, error) {
	// Retrieve the id value.
	r.muID.RLock()
	id := r.id
	r.muID.RUnlock()

	// Retrieve the error value.
	r.muErr.RLock()
	errVal := r.Error
	r.muErr.RUnlock()

	// Retrieve the result. Since it is already a JSON encoded []byte,
	// we wrap it as json.RawMessage to prevent sonic from re-encoding it.
	r.muResult.RLock()
	var result json.RawMessage
	if len(r.Result) > 0 {
		result = json.RawMessage(r.Result)
	}
	r.muResult.RUnlock()

	// Build the output struct. Fields with zero values are omitted.
	out := jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   errVal,
		Result:  result,
	}

	return sonic.Marshal(out)
}

// ParseError parses an error from a raw JSON-RPC response.
func (r *Response) ParseError(raw string) error {
	r.muErr.Lock()
	defer r.muErr.Unlock()

	// Clear previously stored error
	r.errBytes = nil

	// Trim whitespace and check for null
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "null" {
		r.Error = &Error{
			Code:    ServerSideException,
			Message: "empty error",
			Data:    "",
		}
		return nil
	}

	// 1. Unmarshal the error as a standard JSON-RPC error
	var rpcErr Error
	if err := sonic.UnmarshalString(raw, &rpcErr); err == nil {
		// If at least one of Code or Message is set, consider a valid error
		if rpcErr.Code != 0 || rpcErr.Message != "" {
			r.Error = &rpcErr
			r.errBytes = str2Mem(raw)
			return nil
		}
	}

	// 2. Unmarshal an error with numeric code, message, and data fields
	numericError := struct {
		Code    int    `json:"code,omitempty"`
		Message string `json:"message,omitempty"`
		Data    string `json:"data,omitempty"`
	}{}
	if err := sonic.UnmarshalString(raw, &numericError); err == nil {
		if numericError.Code != 0 || numericError.Message != "" || numericError.Data != "" {
			r.Error = &Error{
				Code:    numericError.Code,
				Message: numericError.Message,
				Data:    numericError.Data,
			}
			return nil
		}
	}

	// 3. Unmarshal an error with the error field
	errorStrWrapper := struct {
		Error string `json:"error"`
	}{}
	if err := sonic.UnmarshalString(raw, &errorStrWrapper); err == nil && errorStrWrapper.Error != "" {
		r.Error = &Error{
			Code:    ServerSideException,
			Message: errorStrWrapper.Error,
		}
		return nil
	}

	// 4. Fallback: if none of the above cases match, set the raw message as the error message
	r.Error = &Error{
		Code:    ServerSideException,
		Message: raw,
	}

	return nil
}

// ParseFromStream parses a JSON-RPC response from a stream.
func (r *Response) ParseFromStream(reader io.Reader, expectedSize int) error {
	// 16KB chunks by default
	chunkSize := 16 * 1024
	data, err := ReadAll(reader, int64(chunkSize), expectedSize)
	if err != nil {
		return err
	}

	return r.ParseFromBytes(data)
}

// ParseFromBytes parses a JSON-RPC response from a byte slice.
func (r *Response) ParseFromBytes(data []byte) error {
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

	// Validate jsonrpc version
	if aux.JSONRPC != "2.0" {
		return errors.New("invalid jsonrpc version")
	}

	// Validate that either result or error is present
	resultExists := len(aux.Result) > 0
	errorExists := len(aux.Error) > 0

	if !resultExists && !errorExists {
		return errors.New("response must contain either result or error")
	}
	if resultExists && errorExists {
		return errors.New("response must not contain both result and error")
	}

	// Process the id field
	r.muID.Lock()
	r.idBytes = aux.ID
	r.muID.Unlock()

	// Assign result or error accordingly
	if aux.Result != nil {
		r.muResult.Lock()
		r.Result = aux.Result
		r.muResult.Unlock()
	} else {
		if err := r.ParseError(string(aux.Error)); err != nil {
			return err
		}
	}

	return nil
}

// SetID sets the ID of the JSON-RPC response. Both ID fields are updated.
func (r *Response) SetID(id any) error {
	r.muID.Lock()
	defer r.muID.Unlock()

	r.id = id

	bytes, err := sonic.Marshal(id)
	if err != nil {
		return err
	}
	r.idBytes = bytes

	return nil
}

// String returns a string representation of the JSON-RPC response.
func (r *Response) String() string {
	return fmt.Sprintf("ID: %v, Error: %v, Result bytes: %d", r.id, r.Error, len(r.Result))
}

// ResponseFromStream creates a JSON-RPC response from a stream.
func ResponseFromStream(body io.ReadCloser, expectedSize int) (*Response, error) {
	resp := &Response{}

	if body != nil {
		err := resp.ParseFromStream(body, expectedSize)
		if err != nil {
			return nil, err
		}
		return resp, nil
	}

	return nil, fmt.Errorf("empty body")
}

// Validate checks if the JSON-RPC response conforms to the JSON-RPC specification.
// TODO: finish implementation
func (r *Response) Validate() error {
	if r == nil {
		return errors.New("reponse is nil")
	}

	/* if r.JSONRPC != "2.0" {
		return errors.New("jsonrpc field is required to be exactly \"2.0\"")
	} */

	switch r.id.(type) {
	case nil, string, int64, float64:
	default:
		return errors.New("id field must be a string or a number")
	}

	if r.Error != nil && r.Result != nil {
		return errors.New("response must not contain both result and error")
	}
	if r.Error == nil && r.Result == nil {
		return errors.New("response must contain either result or error")
	}

	return nil
}
