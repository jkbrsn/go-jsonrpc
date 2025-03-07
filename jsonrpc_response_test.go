package jsonrpc

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helpers

// errReader is a simple io.Reader that always returns an error
type errReader string

func (e errReader) Read(p []byte) (n int, err error) {
	return 0, fmt.Errorf("%s", string(e))
}

func TestResponse_Equals(t *testing.T) {
	testCases := []struct {
		name      string
		this      *Response
		other     *Response
		assertion bool
	}{
		{
			name:      "Both nil",
			this:      nil,
			other:     nil,
			assertion: false,
		},
		{
			name:      "One nil",
			this:      &Response{},
			other:     nil,
			assertion: false,
		},
		{
			name:      "Both empty",
			this:      &Response{},
			other:     &Response{},
			assertion: true,
		},
		{
			name:      "Different errors",
			this:      &Response{Error: &Error{Code: 123, Message: "error1"}},
			other:     &Response{Error: &Error{Code: 456, Message: "error2"}},
			assertion: false,
		},
		{
			name:      "Same errors",
			this:      &Response{Error: &Error{Code: 234, Message: "error3"}},
			other:     &Response{Error: &Error{Code: 234, Message: "error3"}},
			assertion: true,
		},
		{
			name:      "Different results",
			this:      &Response{Result: []byte(`"result1"`)},
			other:     &Response{Result: []byte(`"result2"`)},
			assertion: false,
		},
		{
			name:      "Same results",
			this:      &Response{Result: []byte(`"result3"`)},
			other:     &Response{Result: []byte(`"result3"`)},
			assertion: true,
		},
		{
			name:      "Different errors, same results",
			this:      &Response{Error: &Error{Code: 123, Message: "error"}, Result: []byte(`"result"`)},
			other:     &Response{Error: &Error{Code: 456, Message: "error"}, Result: []byte(`"result"`)},
			assertion: false,
		},
		{
			name:      "Same errors, different results",
			this:      &Response{Error: &Error{Code: 123, Message: "error"}, Result: []byte(`"result1"`)},
			other:     &Response{Error: &Error{Code: 123, Message: "error"}, Result: []byte(`"result2"`)},
			assertion: false,
		},
		{
			name:      "Same errors and results",
			this:      &Response{Error: &Error{Code: 123, Message: "error"}, Result: []byte(`"result"`)},
			other:     &Response{Error: &Error{Code: 123, Message: "error"}, Result: []byte(`"result"`)},
			assertion: true,
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.assertion, tt.this.Equals(tt.other))
			assert.Equal(t, tt.assertion, tt.other.Equals(tt.this))
		})
	}
}

func TestResponse_ID(t *testing.T) {
	t.Run("No ID set => returns nil", func(t *testing.T) {
		resp := &Response{}
		assert.Nil(t, resp.ID())
	})

	t.Run("SetID => returns correct ID and caches bytes", func(t *testing.T) {
		resp := &Response{}
		err := resp.SetID("my-unique-id")
		require.NoError(t, err)

		// Reading the ID should return the same 'any' value
		id := resp.ID()
		require.NotNil(t, id)
		idStr, ok := id.(string)
		require.True(t, ok)
		assert.Equal(t, "my-unique-id", idStr)
	})

	t.Run("ID loaded from idBytes if not cached", func(t *testing.T) {
		resp := &Response{
			idBytes: []byte(`123`), // Set directly
		}

		// The first call to ID() should unmarshal idBytes
		id := resp.ID()
		require.NotNil(t, id)
		idVal, ok := id.(float64)
		require.True(t, ok)
		assert.EqualValues(t, 123, idVal)
	})

	t.Run("ID unmarshal error => logs error, returns nil", func(t *testing.T) {
		resp := &Response{
			idBytes: []byte(`{invalid json`),
		}
		got := resp.ID()
		assert.Nil(t, got, "on parse error, ID should end up nil")
	})
}

func TestResponse_IDString(t *testing.T) {
	t.Run("No ID set => returns empty string", func(t *testing.T) {
		resp := &Response{}
		assert.Equal(t, "", resp.IDString())
	})

	t.Run("ID is string => returns same string", func(t *testing.T) {
		resp := &Response{}
		err := resp.SetID("my-unique-id")
		require.NoError(t, err, "SetID should succeed for string")
		assert.Equal(t, "my-unique-id", resp.IDString())
	})

	t.Run("ID is int64 => returns string representation", func(t *testing.T) {
		resp := &Response{}
		err := resp.SetID(int64(12345))
		require.NoError(t, err, "SetID should succeed for int64")
		assert.Equal(t, "12345", resp.IDString())
	})

	t.Run("ID is float64 => returns string representation", func(t *testing.T) {
		resp := &Response{}
		err := resp.SetID(float64(12345.67))
		require.NoError(t, err, "SetID should succeed for float64")
		assert.Equal(t, "12345.67", resp.IDString())
	})

	t.Run("ID is other type => returns empty string", func(t *testing.T) {
		resp := &Response{}
		err := resp.SetID([]int{1, 2, 3})
		require.NoError(t, err, "SetID should succeed for slice")
		assert.Equal(t, "", resp.IDString())
	})
}

func TestResponse_IsEmpty(t *testing.T) {
	t.Run("Nil receiver => true", func(t *testing.T) {
		var resp *Response
		assert.True(t, resp.IsEmpty())
	})

	t.Run("Empty => true", func(t *testing.T) {
		resp := &Response{}
		assert.True(t, resp.IsEmpty())
	})

	t.Run("Various special results => true", func(t *testing.T) {
		cases := [][]byte{
			[]byte(`"0x"`),
			[]byte(`null`),
			[]byte(`""`),
			[]byte(`[]`),
			[]byte(`{}`),
		}
		for _, c := range cases {
			resp := &Response{Result: c}
			assert.True(t, resp.IsEmpty(), "expected %q to be IsEmpty == true", c)
		}
	})

	t.Run("Non-empty => false", func(t *testing.T) {
		resp := &Response{Result: []byte(`"some-value"`)}
		assert.False(t, resp.IsEmpty())
	})
}

func TestResponse_IsNull(t *testing.T) {
	t.Run("Nil receiver => true", func(t *testing.T) {
		var resp *Response
		assert.True(t, resp.IsNull())
	})

	t.Run("Empty everything => true", func(t *testing.T) {
		resp := &Response{}
		assert.True(t, resp.IsNull())
	})

	t.Run("If ID is non-zero => false", func(t *testing.T) {
		resp := &Response{}
		_ = resp.SetID(1)
		assert.False(t, resp.IsNull(), "ID is set => not null")
	})

	t.Run("If Error is non-nil => false", func(t *testing.T) {
		resp := &Response{Error: &Error{Code: 123}}
		assert.False(t, resp.IsNull(), "Error => not null")
	})

	t.Run("If Result is non-empty => false", func(t *testing.T) {
		resp := &Response{Result: []byte(`"hello"`)}
		assert.False(t, resp.IsNull(), "non-empty result => not null")
	})
}

func TestResponse_ParseError(t *testing.T) {
	t.Run("Empty or 'null' => sets generic error", func(t *testing.T) {
		resp := &Response{}
		err := resp.ParseError("")
		require.NoError(t, err)
		require.NotNil(t, resp.Error)
		assert.Equal(t, ServerSideException, resp.Error.Code)
		assert.Contains(t, resp.Error.Message, "empty error")

		resp2 := &Response{}
		err = resp2.ParseError("null")
		require.NoError(t, err)
		assert.NotNil(t, resp2.Error)
		assert.Equal(t, -32603, resp2.Error.Code)
	})

	t.Run("Well-formed JSON-RPC error => sets fields", func(t *testing.T) {
		raw := `{"code": -32000, "message": "some error", "data": "details"}`
		resp := &Response{}
		err := resp.ParseError(raw)
		require.NoError(t, err)
		require.NotNil(t, resp.Error)
		assert.Equal(t, -32000, resp.Error.Code)
		assert.Equal(t, "some error", resp.Error.Message)
		assert.Equal(t, "details", resp.Error.Data)
	})

	t.Run("Case numerics => code, message, data from partial JSON", func(t *testing.T) {
		raw := `{"code":123,"message":"test msg"}`
		resp := &Response{}
		err := resp.ParseError(raw)
		require.NoError(t, err)
		require.NotNil(t, resp.Error)
		assert.Equal(t, 123, resp.Error.Code)
		assert.Equal(t, "test msg", resp.Error.Message)
		assert.Nil(t, resp.Error.Data) // not provided => nil
	})

	t.Run("Case with only 'error' field => sets code -32603, message from error field", func(t *testing.T) {
		raw := `{"error": "this is an error string"}`
		resp := &Response{}
		err := resp.ParseError(raw)
		require.NoError(t, err)
		assert.NotNil(t, resp.Error)
		assert.Equal(t, ServerSideException, resp.Error.Code)
		assert.Equal(t, "this is an error string", resp.Error.Message)
	})

	t.Run("Fallback => raw is message, code -32603", func(t *testing.T) {
		raw := `some-non-json-or-other`
		resp := &Response{}
		err := resp.ParseError(raw)
		require.NoError(t, err)
		assert.NotNil(t, resp.Error)
		assert.Equal(t, ServerSideException, resp.Error.Code)
		assert.Equal(t, "some-non-json-or-other", resp.Error.Message)
	})
}

func TestResponse_ParseFromBytes(t *testing.T) {
	cases := []struct {
		name       string
		bytes      []byte
		runtimeErr bool
		errMessage string
		respErr    *Error
		respID     any
		respRes    json.RawMessage
	}{
		{
			name:       "Has id and result",
			bytes:      []byte(`{"jsonrpc":"2.0","id":1,"result":{"foo":"bar"}}`),
			runtimeErr: false,
			respID:     float64(1),
			respRes:    []byte(`{"foo":"bar"}`),
		},
		{
			name:       "Has id and correctly formed error 1",
			bytes:      []byte(`{"jsonrpc":"2.0","id":"abc","error":{"code":-123,"message":"some msg"}}`),
			runtimeErr: false,
			respErr: &Error{
				Code:    -123,
				Message: "some msg",
			},
			respID: "abc",
		},
		{
			name:       "Has id and correctly formed error 2",
			bytes:      []byte(`{"jsonrpc":"2.0","id":5,"error":{"code":-1234,"data":"some data"}}`),
			runtimeErr: false,
			respErr: &Error{
				Code: -1234,
				Data: "some data",
			},
			respID: float64(5),
		},
		{
			name:       "Has id and error with error string",
			bytes:      []byte(`{"jsonrpc":"2.0","id":"abc","error":{"error":"some string"}}`),
			runtimeErr: false,
			respErr: &Error{
				Code:    ServerSideException,
				Message: "some string",
			},
			respID: "abc",
		},
		{
			name:       "Has id and malformed error",
			bytes:      []byte(`{"jsonrpc":"2.0","id":"abc","error":"just a string"}`),
			runtimeErr: false,
			respErr: &Error{
				Code:    ServerSideException,
				Message: `"just a string"`,
			},
			respID: "abc",
		},
		{
			name:       "Neither error nor result",
			bytes:      []byte(`{"jsonrpc":"2.0","id":2}`),
			runtimeErr: true,
			errMessage: "response must contain either result or error",
		},
		{
			name:       "Both error and result",
			bytes:      []byte(`{"jsonrpc":"2.0","id":2,"error":{"code":-123,"message":"some msg"},"result":{"foo":"bar"}}`),
			runtimeErr: true,
			errMessage: "response must not contain both result and error",
		},
		{
			name:       "Invalid JSON",
			bytes:      []byte(`{invalid-json`),
			runtimeErr: true,
			errMessage: "invalid char",
		},
		{
			name:       "Empty JSON",
			bytes:      []byte{},
			runtimeErr: true,
			errMessage: "input json is empty",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			resp := &Response{}
			err := resp.ParseFromBytes(c.bytes)
			if c.runtimeErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), c.errMessage)
			} else {
				require.NoError(t, err)
				require.NotNil(t, resp)
				assert.Equal(t, c.respErr, resp.Error)
				assert.Equal(t, c.respID, resp.ID())
				assert.Equal(t, c.respRes, resp.Result)
			}
		})
	}
}

func TestResponse_ParseFromStream(t *testing.T) {
	t.Run("Invalid JSON => error response", func(t *testing.T) {
		raw := []byte(`{invalid-json`)
		resp := &Response{}
		err := resp.ParseFromStream(bytes.NewReader(raw), len(raw))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "{invalid-json")
		assert.Nil(t, resp.Error)
		assert.Nil(t, resp.Result)
	})

	t.Run("Nil reader => error return", func(t *testing.T) {
		resp := &Response{}
		err := resp.ParseFromStream(nil, 12)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot read from nil reader")
	})

	t.Run("Reader error => error return", func(t *testing.T) {
		resp := &Response{}
		err := resp.ParseFromStream(errReader("some read error"), 100)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "some read error")
	})
}

func TestResponseFromStream(t *testing.T) {
	t.Run("Nil reader => error", func(t *testing.T) {
		resp, err := ResponseFromStream(nil, 0)
		require.Error(t, err)
		require.Nil(t, resp)
	})

	// Content based test cases
	// TODO: more cases, and make sure the parsing actually denies malformed JSON RPC responses
	cases := []struct {
		name       string
		bytes      []byte
		runtimeErr bool
		expectErr  bool
		expectRes  string
	}{
		{
			name:       "nil bytes",
			bytes:      nil,
			runtimeErr: true,
			expectErr:  true,
			expectRes:  "",
		}, {
			name:       "valid string",
			bytes:      []byte(`{"jsonrpc":"2.0","id":42,"result":"OK"}`),
			runtimeErr: false,
			expectErr:  false,
			expectRes:  `"OK"`,
		}, {
			name:       "valid object",
			bytes:      []byte(`{"jsonrpc":"2.0","id":42,"result":{"key":"value"}}`),
			runtimeErr: false,
			expectErr:  false,
			expectRes:  `{"key":"value"}`,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := io.NopCloser(bytes.NewReader(c.bytes))
			resp, err := ResponseFromStream(r, len(c.bytes))
			if c.runtimeErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.NotNil(t, resp)
				assert.Equal(t, c.expectErr, resp.Error != nil)
				assert.Equal(t, c.expectRes, string(resp.Result))
			}
		})
	}
}

func TestResponse_SetID(t *testing.T) {
	resp := &Response{}
	err := resp.SetID(1234)
	require.NoError(t, err)
	// Confirm that both id and idBytes were set
	assert.Equal(t, 1234, resp.ID())
	assert.Equal(t, []byte(`1234`), resp.idBytes)
}
