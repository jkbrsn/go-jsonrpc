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
	JSONRPC string

	ID    any // TODO: type assertion for nil, int64, float64, string?
	rawID json.RawMessage
	muID  sync.RWMutex

	Error    *Error
	errBytes json.RawMessage
	muErr    sync.RWMutex

	Result   json.RawMessage
	muResult sync.RWMutex
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

// IDString returns the ID as a string.
func (r *Response) IDString() string {
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

// IsEmpty returns whether the JSON-RPC response can be considered empty.
// TODO: add validation + error return?
func (r *Response) IsEmpty() bool {
	if r == nil {
		return true
	}

	r.muErr.RLock()
	defer r.muErr.RUnlock()
	r.muResult.RLock()
	defer r.muResult.RUnlock()

	// A JSON-RPC response is considered empty if it has no error and an empty result
	if r.Error == nil && len(r.Result) == 0 {
		return true
	}

	// A JSON-RPC response error is considered empty if it's empty or contains a zero code,
	// a null message, or an empty data field.
	var emptyError bool
	// TODO: breakout into Error method
	if r.Error != nil {
		emptyError = r.Error.Code == 0 && r.Error.Message == ""
	} else {
		// If the error field is nil, it is considered empty if it has a zero code and an empty message
		emptyError = true
	}

	// A JSON-RPC response result id considered empty if it's empty or contains a zero hex value,
	// a null value, an empty string, an empty array, or an empty object.
	var emptyResult bool
	resultBytes := len(r.Result)
	if resultBytes == 0 ||
		(resultBytes == 4 && r.Result[0] == '"' && r.Result[1] == '0' && r.Result[2] == 'x' && r.Result[3] == '"') ||
		(resultBytes == 4 && r.Result[0] == 'n' && r.Result[1] == 'u' && r.Result[2] == 'l' && r.Result[3] == 'l') ||
		(resultBytes == 2 && r.Result[0] == '"' && r.Result[1] == '"') ||
		(resultBytes == 2 && r.Result[0] == '[' && r.Result[1] == ']') ||
		(resultBytes == 2 && r.Result[0] == '{' && r.Result[1] == '}') {
		emptyResult = true
	} else {
		emptyResult = false
	}

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
	r.muID.RLock()
	var id any
	if r.ID != nil {
		id = r.ID
	} else if r.rawID != nil {
		id = r.rawID
	} else {
		id = nil
	}
	r.muID.RUnlock()

	// Retrieve the error value.
	if len(r.errBytes) > 0 && r.Error == nil {
		if err := r.UnmarshalError(r.errBytes); err != nil {
			return nil, fmt.Errorf("faild to unmarshal JSON-RPC error: %w", err)
		}
	}
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
	output := jsonRPCResponse{
		JSONRPC: r.JSONRPC,
		ID:      id,
		Error:   errVal,
		Result:  result,
	}

	marshalled, err := sonic.Marshal(output)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal JSON-RPC response: %w", err)
	}

	return marshalled, nil
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

// ParseFromBytes parses a JSON-RPC response from a byte slice. This function does not unmarshal
// any of the []byte data, it only stores the raw slices in the Response, to allow for any
// unmarshalling to occur at the caller's discretion.
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
		return errors.New("invalid jsonrpc version: " + aux.JSONRPC)
	}
	r.JSONRPC = aux.JSONRPC

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
	r.muID.Lock()
	r.rawID = aux.ID
	r.muID.Unlock()

	// Assign result or error accordingly
	if aux.Result != nil {
		r.muResult.Lock()
		r.Result = aux.Result
		r.muResult.Unlock()
	} else {
		r.muErr.Lock()
		r.errBytes = aux.Error
		r.muErr.Unlock()
	}

	// TODO: validate ?

	return nil
}

// String returns a string representation of the JSON-RPC response.
func (r *Response) String() string {
	return fmt.Sprintf("ID: %v, Error: %v, Result byte size: %d", r.ID, r.Error, len(r.Result))
}

// UnmarshalError unmarshals an error from a raw JSON-RPC response.
// TODO: move to Error
func (r *Response) UnmarshalError(raw json.RawMessage) error {
	r.muErr.Lock()
	defer r.muErr.Unlock()

	strMsg := string(raw)

	// Trim whitespace and check for null
	trimmed := strings.TrimSpace(strMsg)
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
	if err := sonic.UnmarshalString(strMsg, &rpcErr); err == nil {
		// If at least one of Code or Message is set, consider a valid error
		if rpcErr.Code != 0 || rpcErr.Message != "" {
			r.Error = &rpcErr
			return nil
		}
	}

	// 2. Unmarshal an error with numeric code, message, and data fields
	numericError := struct {
		Code    int    `json:"code,omitempty"`
		Message string `json:"message,omitempty"`
		Data    string `json:"data,omitempty"`
	}{}
	if err := sonic.UnmarshalString(strMsg, &numericError); err == nil {
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
	if err := sonic.UnmarshalString(strMsg, &errorStrWrapper); err == nil && errorStrWrapper.Error != "" {
		r.Error = &Error{
			Code:    ServerSideException,
			Message: errorStrWrapper.Error,
		}
		return nil
	}

	// 4. Fallback: if none of the above cases match, set the raw message as the error message
	r.Error = &Error{
		Code:    ServerSideException,
		Message: strMsg,
	}

	return nil
}

// UnmarshalJSON unmarshals the input data into the members of Response. Note that the Result field
// is stored as raw JSON bytes, and will not be unmarshalled.
func (r *Response) UnmarshalJSON(data []byte) error {
	err := r.ParseFromBytes(data)
	if err != nil {
		return fmt.Errorf("failed to unmarshal JSON-RPC response: %w", err)
	}

	// Unmarshal the ID field
	r.muID.Lock()
	if len(r.rawID) > 0 {
		var id any
		if err := sonic.Unmarshal(r.rawID, &id); err != nil {
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
	r.muID.Unlock()

	// Unmarshal the Error field
	r.muResult.RLock()
	if len(r.Result) == 0 {
		if err := r.UnmarshalError(r.errBytes); err != nil {
			return err
		}
	}
	r.muResult.RUnlock()

	return nil
}

// Validate checks if the JSON-RPC response conforms to the JSON-RPC specification.
// TODO: finish implementation
func (r *Response) Validate() error {
	if r == nil {
		return errors.New("response is nil")
	}

	if r.JSONRPC != "2.0" {
		return errors.New("jsonrpc field is required to be exactly \"2.0\"")
	}

	switch r.ID.(type) {
	case nil, string, int64, float64:
	default:
		return errors.New("id field must be a string or a number")
	}

	if r.Error != nil && r.Result != nil {
		return errors.New("response must not contain both result and error")
	}
	if r.Error == nil && len(r.errBytes) == 0 && r.Result == nil {
		return errors.New("response must contain either result or error")
	}

	return nil
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
