// Package jsonrpc provides a Go implementation of the JSON-RPC 2.0 specification, as well as tools
// to parse and work with JSON-RPC requests and responses.
package jsonrpc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestError_Equals(t *testing.T) {
	t.Run("Both nil", func(t *testing.T) {
		var e1, e2 *Error
		assert.True(t, e1.Equals(e2))
	})

	t.Run("One nil", func(t *testing.T) {
		var e1 *Error
		e2 := &Error{}
		assert.False(t, e1.Equals(e2))
		assert.False(t, e2.Equals(e1))
	})

	t.Run("Both empty", func(t *testing.T) {
		e1 := &Error{}
		e2 := &Error{}
		assert.True(t, e1.Equals(e2))
	})

	t.Run("Different codes", func(t *testing.T) {
		e1 := &Error{Code: -32000}
		e2 := &Error{Code: -32001}
		assert.False(t, e1.Equals(e2))
	})

	t.Run("Different messages", func(t *testing.T) {
		e1 := &Error{Message: "error1"}
		e2 := &Error{Message: "error2"}
		assert.False(t, e1.Equals(e2))
	})

	t.Run("Same code and message", func(t *testing.T) {
		e1 := &Error{Code: -32000, Message: "some error"}
		e2 := &Error{Code: -32000, Message: "some error"}
		assert.True(t, e1.Equals(e2))
	})

	t.Run("Different data, same code and message", func(t *testing.T) {
		e1 := &Error{Code: -32000, Message: "some error", Data: "data1"}
		e2 := &Error{Code: -32000, Message: "some error", Data: "data2"}
		assert.True(t, e1.Equals(e2))
	})
}

func TestError_IsEmpty(t *testing.T) {
	t.Run("Nil error", func(t *testing.T) {
		var e *Error
		assert.True(t, e.IsEmpty())
	})

	t.Run("Empty error", func(t *testing.T) {
		e := &Error{}
		assert.True(t, e.IsEmpty())
	})

	t.Run("Non-empty code", func(t *testing.T) {
		e := &Error{Code: -32000}
		assert.False(t, e.IsEmpty())
	})

	t.Run("Non-empty message", func(t *testing.T) {
		e := &Error{Message: "some error"}
		assert.False(t, e.IsEmpty())
	})

	t.Run("Non-empty code and message", func(t *testing.T) {
		e := &Error{Code: -32000, Message: "some error"}
		assert.False(t, e.IsEmpty())
	})

	t.Run("Only Data populated", func(t *testing.T) {
		e := &Error{Data: "some data"}
		assert.True(t, e.IsEmpty())
	})

	t.Run("Zero code with message is not empty", func(t *testing.T) {
		e := &Error{Code: 0, Message: "some error"}
		assert.False(t, e.IsEmpty(), "zero code with message should not be empty")
	})

	t.Run("Zero code without message is empty", func(t *testing.T) {
		e := &Error{Code: 0, Message: ""}
		assert.True(t, e.IsEmpty(), "zero code without message should be empty")
	})
}

func TestError_UnmarshalJSON(t *testing.T) {
	t.Run("Empty error", func(t *testing.T) {
		e := &Error{}
		err := e.UnmarshalJSON([]byte(""))
		require.NoError(t, err)
		require.NotNil(t, e)
		assert.Equal(t, ServerSideException, e.Code)
		assert.Contains(t, e.Message, "empty error")
	})

	t.Run("Null error", func(t *testing.T) {
		e := &Error{}
		err := e.UnmarshalJSON([]byte("null"))
		require.NoError(t, err)
		assert.NotNil(t, e)
		assert.Equal(t, -32603, e.Code)
	})

	t.Run("Well-formed JSON-RPC error", func(t *testing.T) {
		raw := []byte(`{"code": -32000, "message": "some error", "data": "details"}`)
		e := &Error{}
		err := e.UnmarshalJSON(raw)
		require.NoError(t, err)
		require.NotNil(t, e)
		assert.Equal(t, -32000, e.Code)
		assert.Equal(t, "some error", e.Message)
		assert.Equal(t, "details", e.Data)
	})

	t.Run("Numeric error", func(t *testing.T) {
		raw := []byte(`{"code":123,"message":"test msg"}`)
		e := &Error{}
		err := e.UnmarshalJSON(raw)
		require.NoError(t, err)
		require.NotNil(t, e)
		assert.Equal(t, 123, e.Code)
		assert.Equal(t, "test msg", e.Message)
		assert.Nil(t, e.Data) // not provided => nil
	})

	t.Run("Case with only 'message' and 'data'", func(t *testing.T) {
		raw := []byte(`{"message": "this is a message","data": "this is data"}`)
		e := &Error{}
		err := e.UnmarshalJSON(raw)
		require.NoError(t, err)
		assert.NotNil(t, e)
		assert.Equal(t, ServerSideException, e.Code)
		assert.Contains(t, e.Message, "this is a message")
		assert.Equal(t, "this is data", e.Data)
	})

	t.Run("Case with only 'error' field", func(t *testing.T) {
		raw := []byte(`{"error": "this is an error string"}`)
		e := &Error{}
		err := e.UnmarshalJSON(raw)
		require.NoError(t, err)
		assert.NotNil(t, e)
		assert.Equal(t, ServerSideException, e.Code)
		assert.Equal(t, "this is an error string", e.Message)
	})

	t.Run("Case fallback", func(t *testing.T) {
		raw := []byte(`some-non-json-or-other`)
		e := &Error{}
		err := e.UnmarshalJSON(raw)
		require.NoError(t, err)
		assert.NotNil(t, e)
		assert.Equal(t, ServerSideException, e.Code)
		assert.Equal(t, "some-non-json-or-other", e.Message)
	})
}

func TestError_Validate(t *testing.T) {
	t.Run("Valid error", func(t *testing.T) {
		e := &Error{
			Code:    -32000,
			Message: "some error",
		}
		err := e.Validate()
		require.NoError(t, err)
	})

	t.Run("Missing code", func(t *testing.T) {
		e := &Error{
			Message: "some error",
		}
		err := e.Validate()
		// With updated validation, a message alone is sufficient
		require.NoError(t, err)
	})

	t.Run("Empty error", func(t *testing.T) {
		e := &Error{}
		err := e.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "either a non-zero code or a message")
	})

	t.Run("Zero code with message is valid", func(t *testing.T) {
		e := &Error{Code: 0, Message: "some error"}
		err := e.Validate()
		require.NoError(t, err,
			"zero codes are allowed per JSON-RPC 2.0 spec when message is present")
	})

	t.Run("Nil error", func(t *testing.T) {
		var e *Error
		err := e.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "nil")
	})
}
