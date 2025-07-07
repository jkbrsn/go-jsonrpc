// Package jsonrpc provides a Go implementation of the JSON-RPC 2.0 specification, as well as tools
// to parse and work with JSON-RPC requests and responses.
package jsonrpc

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helpers

// errReader is a simple io.Reader that always returns an error
type errReadCloser string

func (e errReadCloser) Read(p []byte) (n int, err error) {
	return 0, fmt.Errorf("%s", string(e))
}
func (e errReadCloser) Close() error {
	return nil
}

// readCloser is a simple io.ReadCloser that wraps a bytes.Reader
type readCloser struct {
	*bytes.Reader
}

func (rc *readCloser) Close() error {
	return nil
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
			name:      "Differen jsonrpc",
			this:      &Response{JSONRPC: "2.0"},
			other:     &Response{JSONRPC: "1.0"},
			assertion: false,
		},
		{
			name:      "Different ID:s",
			this:      &Response{ID: "id1"},
			other:     &Response{ID: "id2"},
			assertion: false,
		},
		{
			name:      "Different ID types",
			this:      &Response{ID: "some-id"},
			other:     &Response{ID: 24},
			assertion: false,
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

func TestResponse_IDString(t *testing.T) {
	t.Run("No ID set => returns empty string", func(t *testing.T) {
		resp := &Response{}
		assert.Equal(t, "", resp.IDString())
	})

	t.Run("ID is string text", func(t *testing.T) {
		resp := &Response{}
		resp.ID = "my-unique-id"
		assert.Equal(t, "my-unique-id", resp.IDString())
	})

	t.Run("ID is string integer", func(t *testing.T) {
		resp := &Response{}
		resp.ID = "15"
		assert.Equal(t, "15", resp.IDString())
	})

	t.Run("ID is string float", func(t *testing.T) {
		resp := &Response{}
		resp.ID = "33.75"
		assert.Equal(t, "33.75", resp.IDString())
	})

	t.Run("ID is int64", func(t *testing.T) {
		resp := &Response{}
		resp.ID = int64(12345)
		assert.Equal(t, "12345", resp.IDString())
	})

	t.Run("ID is float64", func(t *testing.T) {
		resp := &Response{}
		resp.ID = float64(12345.67)
		assert.Equal(t, "12345.67", resp.IDString())
	})

	t.Run("ID is float64 integer value", func(t *testing.T) {
		resp := &Response{}
		resp.ID = float64(25.0)
		assert.Equal(t, "25.0", resp.IDString())
	})

	t.Run("ID is other type => returns empty string", func(t *testing.T) {
		resp := &Response{}
		resp.ID = []int{1, 2, 3}
		assert.Equal(t, "", resp.IDString())
	})
}

func TestResponse_IsEmpty(t *testing.T) {
	t.Run("Results considered empty", func(t *testing.T) {
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

	t.Run("Empty Response", func(t *testing.T) {
		cases := []struct {
			name string
			resp *Response
		}{
			{
				name: "Nil receiver",
				resp: nil,
			},
			{
				name: "Empty response",
				resp: &Response{},
			},
			{
				name: "Result is empty",
				resp: &Response{Result: []byte{}},
			},
			{
				name: "Error is empty",
				resp: &Response{Error: &Error{}},
			},
			{
				name: "Result and Error are empty",
				resp: &Response{Result: []byte{}, Error: &Error{}},
			},
			{
				name: "Error without Code or Message",
				resp: &Response{Error: &Error{Data: "some data"}},
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				assert.True(t, tc.resp.IsEmpty())
			})
		}
	})

	t.Run("Non-empty Response", func(t *testing.T) {
		cases := []struct {
			name string
			resp *Response
		}{
			{
				name: "Result only",
				resp: &Response{Result: []byte(`"some-value"`)},
			},
			{
				name: "Error only",
				resp: &Response{Error: &Error{Code: 123, Message: "some error"}},
			},
			{
				name: "Result and error",
				resp: &Response{Result: []byte(`"some-value"`), Error: &Error{Code: 123, Message: "some error"}},
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				assert.False(t, tc.resp.IsEmpty())
			})
		}
	})
}

func TestResponse_MarshalJSON(t *testing.T) {
	cases := []struct {
		name       string
		resp       *Response
		runtimeErr bool
		json       []byte
	}{
		{
			name: "Response with result",
			resp: &Response{JSONRPC: "2.0", ID: int64(1), Result: []byte(`{"foo":"bar"}`)},
			json: []byte(`{"jsonrpc":"2.0","id":1,"result":{"foo":"bar"}}`),
		},
		{
			name: "Response with Error",
			resp: &Response{JSONRPC: "2.0", ID: "first", Error: &Error{Code: 123, Message: "test msg"}},
			json: []byte(`{"jsonrpc":"2.0","id":"first","error":{"code":123,"message":"test msg"}}`),
		},
		{
			name: "Response with rawError and nil ID",
			resp: &Response{JSONRPC: "2.0", ID: nil, rawError: []byte(`{"code":123,"message":"test msg"}`)},
			json: []byte(`{"jsonrpc":"2.0","id":null,"error":{"code":123,"message":"test msg"}}`),
		},
		{
			name: "Invalid: both result and error",
			resp: &Response{
				JSONRPC: "2.0",
				ID:      "first",
				Result:  []byte(`{"foo":"bar"}`),
				Error:   &Error{Code: 123, Message: "test msg"},
			},
			runtimeErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			marshalled, err := tc.resp.MarshalJSON()
			if tc.runtimeErr {
				assert.Error(t, err)
				assert.Nil(t, marshalled)
			} else {
				assert.NoError(t, err)
				assert.JSONEq(t, string(tc.json), string(marshalled))
			}
		})
	}
}

func TestResponse_parseFromReader(t *testing.T) {
	// Only basic tests here, since the internal call of parseFromBytes is tested separately

	t.Run("Nil reader", func(t *testing.T) {
		resp := &Response{}
		err := resp.parseFromReader(nil, 12)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot read from nil reader")
	})

	t.Run("Reader error", func(t *testing.T) {
		resp := &Response{}
		err := resp.parseFromReader(errReadCloser("some read error"), 100)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "some read error")
	})

	t.Run("Valid JSON with result", func(t *testing.T) {
		raw := []byte(`{"jsonrpc":"2.0","id":42,"result":"OK"}`)
		resp := &Response{}
		err := resp.parseFromReader(bytes.NewReader(raw), len(raw))
		require.NoError(t, err)
		assert.Nil(t, resp.rawError)
		assert.Nil(t, resp.Error)
		assert.NotNil(t, resp.Result)

		// Unmarshal the result to check if it's correct
		var resultStr string
		err = json.Unmarshal(resp.Result, &resultStr)
		require.NoError(t, err)
		assert.Equal(t, "OK", resultStr)
	})

	t.Run("Valid JSON with error", func(t *testing.T) {
		raw := []byte(`{"jsonrpc":"2.0","id":42,"error":{"code":-32000}}`)
		resp := &Response{}
		err := resp.parseFromReader(bytes.NewReader(raw), len(raw))
		require.NoError(t, err)
		assert.NotNil(t, resp.rawError)
		assert.Nil(t, resp.Result)
	})

	t.Run("Invalid JSON", func(t *testing.T) {
		raw := []byte(`{invalid-json`)
		resp := &Response{}
		err := resp.parseFromReader(bytes.NewReader(raw), len(raw))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "{invalid-json")
		assert.Nil(t, resp.rawError)
		assert.Nil(t, resp.Error)
		assert.Nil(t, resp.Result)
	})

	t.Run("Large JSON to test chunked reading", func(t *testing.T) {
		// Create a JSON response larger than the 16KB chunk size
		largeBytes := bytes.Repeat([]byte("a"), 16*1024+1)
		raw := []byte(`{"jsonrpc":"2.0","id":42,"result":"`)
		raw = append(raw, largeBytes...)
		raw = append(raw, []byte(`"}`)...)

		resp := &Response{}
		err := resp.parseFromReader(bytes.NewReader(raw), len(raw))
		require.NoError(t, err)
		assert.Nil(t, resp.rawError)
		assert.Nil(t, resp.Error)
		require.NotNil(t, resp.Result)

		// Unmarshal the result to check if it's correct
		var resultStr string
		err = json.Unmarshal(resp.Result, &resultStr)
		require.NoError(t, err)
		assert.Equal(t, string(largeBytes), resultStr)
	})
}

func TestResponse_parseFromBytes(t *testing.T) {
	t.Run("Valid response with result", func(t *testing.T) {
		raw := []byte(`{"jsonrpc":"2.0","id":1,"result":{"foo":"bar"}}`)
		resp := &Response{}
		err := resp.parseFromBytes(raw)
		require.NoError(t, err)
		assert.NotNil(t, resp.rawID)
		assert.NotNil(t, resp.Result)
		assert.Nil(t, resp.Error)
		assert.Nil(t, resp.rawError)
	})

	t.Run("Valid response with error", func(t *testing.T) {
		raw := []byte(`{"jsonrpc":"2.0","id":1,"error":{"code":-32000}}`)
		resp := &Response{}
		err := resp.parseFromBytes(raw)
		require.NoError(t, err)
		assert.NotNil(t, resp.rawID)
		assert.Nil(t, resp.Result)
		assert.Nil(t, resp.Error)
		assert.NotNil(t, resp.rawError)
	})

	t.Run("Valid response with extra fields", func(t *testing.T) {
		raw := []byte(`{"jsonrpc":"2.0","id":1,"result":"OK","something":"extra"}`)
		resp := &Response{}
		err := resp.parseFromBytes(raw)
		require.NoError(t, err)
		assert.NotNil(t, resp.rawID)
		assert.NotNil(t, resp.Result)
		assert.Nil(t, resp.Error)
		assert.Nil(t, resp.rawError)
	})

	t.Run("Invalid response: both result and error", func(t *testing.T) {
		raw := []byte(`{"jsonrpc":"2.0","id":1,"result":"OK","error":{"core": -32000}}`)
		resp := &Response{}
		err := resp.parseFromBytes(raw)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "response must not contain both result and error")
		assert.Nil(t, resp.rawID)
		assert.Nil(t, resp.Result)
		assert.Nil(t, resp.Error)
		assert.Nil(t, resp.rawError)
	})

	t.Run("Invalid JSON", func(t *testing.T) {
		cases := [][]byte{
			[]byte(`"123"`),
			[]byte(`text`),
			[]byte(`""`),
			[]byte(`{"broken`),
			[]byte(`{"key-only"}`),
		}

		for _, tc := range cases {
			resp := &Response{}
			err := resp.parseFromBytes(tc)
			require.Error(t, err)
			assert.Nil(t, resp.rawID)
			assert.Nil(t, resp.rawError)
			assert.Nil(t, resp.Error)
			assert.Nil(t, resp.Result)
		}
	})
}

func TestResponse_UnmarshalJSON(t *testing.T) {
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
			name:       "Valid response with result",
			bytes:      []byte(`{"jsonrpc":"2.0","id":1,"result":{"foo":"bar"}}`),
			runtimeErr: false,
			respID:     int64(1),
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
			respID: int64(5),
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
			name:       "Invalid ID type",
			bytes:      []byte(`{"jsonrpc":"2.0","id":[1,2,3],"result":{"foo":"bar"}}`),
			runtimeErr: true,
			errMessage: "id field must be a string or a number",
		},
		{
			name:       "Invalid JSON-RPC version",
			bytes:      []byte(`{"jsonrpc":"1.0","id":2,"result":{"foo":"bar"}}`),
			runtimeErr: true,
			errMessage: "invalid JSON-RPC version",
		},
		{
			name:       "Invalid JSON formatting",
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
			err := resp.UnmarshalJSON(c.bytes)
			if c.runtimeErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), c.errMessage)
			} else {
				require.NoError(t, err)
				require.NotNil(t, resp)
				assert.Equal(t, c.respErr, resp.Error)
				assert.Equal(t, c.respID, resp.ID)
				assert.Equal(t, c.respRes, resp.Result)
			}
		})
	}
}

func TestResponse_Validate(t *testing.T) {
	cases := []struct {
		name       string
		resp       *Response
		runtimeErr bool
		errMessage string
	}{
		{
			name:       "Valid response with result",
			resp:       &Response{JSONRPC: "2.0", ID: int64(1), Result: []byte(`{"foo":"bar"}`)},
			runtimeErr: false,
		},
		{
			name:       "Valid response with error",
			resp:       &Response{JSONRPC: "2.0", ID: "first", Error: &Error{Code: 123, Message: "test msg"}},
			runtimeErr: false,
		},
		{
			name:       "Invalid JSON-RPC version",
			resp:       &Response{JSONRPC: "1.0", ID: int64(1), Result: []byte(`{"foo":"bar"}`)},
			runtimeErr: true,
			errMessage: "invalid jsonrpc version",
		},
		{
			name:       "Invalid ID type",
			resp:       &Response{JSONRPC: "2.0", ID: []int{1, 2, 3}, Result: []byte(`{"foo":"bar"}`)},
			runtimeErr: true,
			errMessage: "id field must be a string or a number",
		},
		{
			name:       "Both result and error",
			resp:       &Response{JSONRPC: "2.0", ID: "first", Result: []byte(`{"foo":"bar"}`), Error: &Error{Code: 123, Message: "test msg"}},
			runtimeErr: true,
			errMessage: "response must not contain both result and error",
		},
		{
			name:       "Neither result nor error",
			resp:       &Response{JSONRPC: "2.0", ID: "first"},
			runtimeErr: true,
			errMessage: "response must contain either result or error",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.resp.Validate()
			if c.runtimeErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), c.errMessage)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestDecodeResponse(t *testing.T) {
	t.Run("Nil data", func(t *testing.T) {
		resp, err := DecodeResponse(nil)
		require.Error(t, err)
		require.Nil(t, resp)
	})

	t.Run("Empty data", func(t *testing.T) {
		resp, err := DecodeResponse([]byte{})
		require.Error(t, err)
		require.Nil(t, resp)
	})

	t.Run("Valid JSON with result", func(t *testing.T) {
		raw := []byte(`{"jsonrpc":"2.0","id":42,"result":"OK"}`)
		resp, err := DecodeResponse(raw)
		require.NoError(t, err)
		assert.Nil(t, resp.rawError)
		assert.Nil(t, resp.Error)
		assert.NotNil(t, resp.Result)

		// Unmarshal the result to check if it's correct
		var resultStr string
		err = json.Unmarshal(resp.Result, &resultStr)
		require.NoError(t, err)
		assert.Equal(t, "OK", resultStr)
	})

	t.Run("Valid JSON with error", func(t *testing.T) {
		raw := []byte(`{"jsonrpc":"2.0","id":42,"error":{"code":-32000}}`)
		resp, err := DecodeResponse(raw)
		require.NoError(t, err)
		assert.NotNil(t, resp.rawError)
		assert.Nil(t, resp.Result)
	})

	t.Run("Invalid JSON", func(t *testing.T) {
		raw := []byte(`{invalid-json`)
		resp, err := DecodeResponse(raw)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "{invalid-json")
		assert.Nil(t, resp)
	})
}

func TestDecodeResponseFromReader(t *testing.T) {
	// Only basic tests here, since the internal call of parseFromReader is tested separately

	t.Run("Nil reader", func(t *testing.T) {
		resp, err := DecodeResponseFromReader(nil, 0)
		require.Error(t, err)
		require.Nil(t, resp)
	})

	t.Run("Reader error", func(t *testing.T) {
		resp, err := DecodeResponseFromReader(errReadCloser("some read error"), 100)
		require.Error(t, err)
		require.Nil(t, resp)
		assert.Contains(t, err.Error(), "some read error")
	})

	t.Run("Valid JSON with result", func(t *testing.T) {
		raw := []byte(`{"jsonrpc":"2.0","id":42,"result":"OK"}`)
		resp, err := DecodeResponseFromReader(&readCloser{bytes.NewReader(raw)}, len(raw))
		require.NoError(t, err)
		assert.Nil(t, resp.rawError)
		assert.Nil(t, resp.Error)
		assert.NotNil(t, resp.Result)
	})

	t.Run("Valid JSON with error", func(t *testing.T) {
		raw := []byte(`{"jsonrpc":"2.0","id":42,"error":{"code":-32000}}`)
		resp, err := DecodeResponseFromReader(&readCloser{bytes.NewReader(raw)}, len(raw))
		require.NoError(t, err)
		assert.NotNil(t, resp.rawError)
		assert.Nil(t, resp.Result)
	})

	t.Run("Invalid JSON", func(t *testing.T) {
		raw := []byte(`{invalid-json`)
		resp, err := DecodeResponseFromReader(&readCloser{bytes.NewReader(raw)}, len(raw))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "{invalid-json")
		assert.Nil(t, resp)
	})
}

func TestResponse_Concurrency(t *testing.T) {
	t.Run("Concurrent IDString", func(t *testing.T) {
		resp := &Response{ID: int64(12345)}

		var wg sync.WaitGroup
		for range 200 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				idStr := resp.IDString()
				assert.Equal(t, "12345", idStr)
			}()
		}
		wg.Wait()
	})

	t.Run("Concurrent IsEmpty", func(t *testing.T) {
		resp := &Response{Result: []byte(`"0x"`)}

		var wg sync.WaitGroup
		for range 200 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				isEmpty := resp.IsEmpty()
				assert.True(t, isEmpty)
			}()
		}
		wg.Wait()
	})

	t.Run("Concurrent Equals", func(t *testing.T) {
		resp1 := &Response{JSONRPC: "2.0", ID: int64(1), Result: []byte(`{"foo":"bar"}`)}
		resp2 := &Response{JSONRPC: "2.0", ID: int64(1), Result: []byte(`{"foo":"bar"}`)}

		var wg sync.WaitGroup
		for range 200 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				equal := resp1.Equals(resp2)
				assert.True(t, equal)
			}()
		}
		wg.Wait()
	})
}

func TestResponse_UnmarshalResult(t *testing.T) {
	type myRes struct {
		Foo string `json:"foo"`
	}

	t.Run("Success path", func(t *testing.T) {
		raw := []byte(`{"jsonrpc":"2.0","id":1,"result":{"foo":"bar"}}`)
		resp, err := DecodeResponse(raw)
		require.NoError(t, err)

		var out myRes
		err = resp.UnmarshalResult(&out)
		require.NoError(t, err)
		assert.Equal(t, "bar", out.Foo)
	})

	t.Run("Missing result returns error", func(t *testing.T) {
		raw := []byte(`{"jsonrpc":"2.0","id":1,"error":{"code":-32000,"message":"oops"}}`)
		resp, err := DecodeResponse(raw)
		require.NoError(t, err)

		var dst any
		err = resp.UnmarshalResult(&dst)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no result field")
	})
}

func TestResponse_UnmarshalError(t *testing.T) {
	raw := []byte(`{"jsonrpc":"2.0","id":1,"error":{"code":-32000,"message":"oops"}}`)
	resp, err := DecodeResponse(raw)
	require.NoError(t, err)

	// Error should still be nil until explicitly decoded.
	assert.Nil(t, resp.Error)

	err = resp.UnmarshalError()
	require.NoError(t, err)
	require.NotNil(t, resp.Error)
	assert.Equal(t, -32000, resp.Error.Code)
	assert.Equal(t, "oops", resp.Error.Message)

	// Calling again should be idempotent and still succeed.
	err = resp.UnmarshalError()
	require.NoError(t, err)
}
