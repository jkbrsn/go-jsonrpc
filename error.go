package jsonrpc

import (
	"errors"
	"fmt"
	"strings"

	"github.com/bytedance/sonic" // Primary JSON parser for performance
)

// JSON-RPC error codes
const (
	InvalidRequest      = -32600
	MethodNotFound      = -32601
	InvalidParams       = -32602
	ServerSideException = -32603
	ParseError          = -32700
)

// Error represents a JSON-RPC error.
type Error struct {
	Code    int    `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
	Data    any    `json:"data,omitempty"` // Optional data field
}

// Equals compares the contents of two JSON-RPC errors for equality.
// Returns true if both errors have the same Code and Message.
//
// Note: The Data field is intentionally excluded from comparison because:
//  1. Data has type `any`, making deep comparison complex and expensive
//  2. Error equality is typically determined by code + message alone
//  3. Data is optional and used for supplementary information
//
// If you need to compare Data fields, do so separately after calling Equals().
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

// IsEmpty returns true if the error is empty, which is if the error is nil or both code and message
// are empty.
//
// Note: Zero error codes are valid per JSON-RPC 2.0 spec, but are treated as "empty" when
// both code=0 and message="". This helps identify placeholder or uninitialized errors.
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

// UnmarshalJSON unmarshals an error from a raw JSON-RPC response.
// The unmarshal logic uses several fallbacks to ensure an error is produced.
func (e *Error) UnmarshalJSON(data []byte) error {
	// Check for null
	strData := string(data)
	trimmed := strings.TrimSpace(strData)
	if trimmed == "" || trimmed == "null" {
		e.Code = ServerSideException
		e.Message = "empty error"
		return nil
	}

	// 1. Unmarshal the error as a standard JSON-RPC error
	type alias Error // Avoid infinite recursion by using an alias
	if err := sonic.Unmarshal(data, (*alias)(e)); err == nil {
		// If Code and Message are set, consider a valid error
		if e.Code != 0 {
			return nil
		}
	}

	// 2. Try to catch common error formats
	errorStrWrapper := struct {
		Error string `json:"error"`
	}{}
	if err := sonic.Unmarshal(data, &errorStrWrapper); err == nil && errorStrWrapper.Error != "" {
		e.Code = ServerSideException
		e.Message = errorStrWrapper.Error
		return nil
	}

	// 3. Fallback: if none of the above cases match, set the raw message as the error message
	e.Code = ServerSideException
	e.Message = strData

	// 4. Validate the error
	if err := e.Validate(); err != nil {
		return fmt.Errorf("failed to unmarshal JSON-RPC error: %w", err)
	}

	return nil
}

// Validate checks if the error is valid according to the JSON-RPC specification.
// An error is valid if it has at least one of: a non-zero code or a non-empty message.
// Zero error codes are allowed per JSON-RPC 2.0 spec, though they're treated as "empty"
// by IsEmpty() when combined with an empty message.
func (e *Error) Validate() error {
	if e == nil {
		return errors.New("error is nil")
	}
	if e.Code == 0 && e.Message == "" {
		return errors.New("error must have either a non-zero code or a message")
	}
	return nil
}
