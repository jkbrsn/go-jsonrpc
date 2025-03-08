package jsonrpc

import (
	"errors"
	"fmt"
	"strings"

	"github.com/bytedance/sonic"
)

// Error represents a JSON-RPC error.
type Error struct {
	Code    int    `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
	Data    any    `json:"data,omitempty"` // Optional data field
}

// Equals compares the contents of two JSON-RPC errors. Note: the Data field is not compared.
func (e *Error) Equals(other *Error) bool {
	if e == nil && other == nil {
		return true
	}
	if e == nil || other == nil {
		return false
	}
	if e.Code != other.Code {
		return false
	}
	if e.Message != other.Message {
		return false
	}
	return true
}

// IsEmpty returns true if the error is empty, which is if the code and message are both empty.
func (e *Error) IsEmpty() bool {
	if e == nil {
		return true
	}
	return e.Code == 0 && e.Message == ""
}

// String returns a string representation of the error.
func (e *Error) String() string {
	return fmt.Sprintf("Code: %d, Message: %s", e.Code, e.Message)
}

// UnmarshalError unmarshals an error from a raw JSON-RPC response.
func (e *Error) UnmarshalJSON(data []byte) error {
	strData := string(data)

	// Check for null
	trimmed := strings.TrimSpace(strData)
	if trimmed == "" || trimmed == "null" {
		e.Code = ServerSideException
		e.Message = "empty error"
		return nil
	}

	// 1. Unmarshal the error as a standard JSON-RPC error

	type alias Error // Avoid infinite recursion by using an alias
	if err := sonic.UnmarshalString(strData, (*alias)(e)); err == nil {
		// If Code and Message are set, consider a valid error
		if e.Code != 0 && e.Message != "" {
			return nil
		}
	}

	// 2. Unmarshal an error with numeric code, message, and data fields
	numericError := struct {
		Code    int    `json:"code,omitempty"`
		Message string `json:"message,omitempty"`
		Data    string `json:"data,omitempty"`
	}{}
	if err := sonic.UnmarshalString(strData, &numericError); err == nil {
		if numericError.Code != 0 || numericError.Message != "" || numericError.Data != "" {
			e.Code = numericError.Code
			e.Message = numericError.Message
			e.Data = numericError.Data
			return nil
		}
	}

	// 3. Unmarshal an error with the error field
	errorStrWrapper := struct {
		Error string `json:"error"`
	}{}
	if err := sonic.UnmarshalString(strData, &errorStrWrapper); err == nil && errorStrWrapper.Error != "" {
		e.Code = ServerSideException
		e.Message = errorStrWrapper.Error
		return nil
	}

	// 4. Fallback: if none of the above cases match, set the raw message as the error message
	e.Code = ServerSideException
	e.Message = strData

	return nil
}

// Validate checks if the error is valid according to the JSON-RPC specification.
func (e *Error) Validate() error {
	var err error
	if e.Code == 0 {
		err = errors.Join(err, errors.New("code is required"))
	}
	if e.Message == "" {
		err = errors.Join(err, errors.New("message is required"))
	}
	return err
}
