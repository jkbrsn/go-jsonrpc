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

func (e errReadCloser) Read(_ []byte) (n int, err error) {
	return 0, fmt.Errorf("%s", string(e))
}
func (errReadCloser) Close() error {
	return nil
}

// readCloser is a simple io.ReadCloser that wraps a bytes.Reader
type readCloser struct {
	*bytes.Reader
}

func (*readCloser) Close() error {
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
			this:      &Response{jsonrpc: "2.0"},
			other:     &Response{jsonrpc: "1.0"},
			assertion: false,
		},
		{
			name:      "Different id: s",
			this:      &Response{id: "id1"},
			other:     &Response{id: "id2"},
			assertion: false,
		},
		{
			name:      "Different ID types",
			this:      &Response{id: "some-id"},
			other:     &Response{id: 24},
			assertion: false,
		},
		{
			name:      "Different errors",
			this:      &Response{err: &Error{Code: 123, Message: "error1"}},
			other:     &Response{err: &Error{Code: 456, Message: "error2"}},
			assertion: false,
		},
		{
			name:      "Same errors",
			this:      &Response{err: &Error{Code: 234, Message: "error3"}},
			other:     &Response{err: &Error{Code: 234, Message: "error3"}},
			assertion: true,
		},
		{
			name:      "Different results",
			this:      &Response{result: []byte(`"result1"`)},
			other:     &Response{result: []byte(`"result2"`)},
			assertion: false,
		},
		{
			name:      "Same results",
			this:      &Response{result: []byte(`"result3"`)},
			other:     &Response{result: []byte(`"result3"`)},
			assertion: true,
		},
		{
			name: "Different errors, same results",
			this: &Response{
				err:    &Error{Code: 123, Message: "error"},
				result: []byte(`"result"`),
			},
			other: &Response{
				err:    &Error{Code: 456, Message: "error"},
				result: []byte(`"result"`),
			},
			assertion: false,
		},
		{
			name: "Same errors, different results",
			this: &Response{
				err:    &Error{Code: 123, Message: "error"},
				result: []byte(`"result1"`),
			},
			other: &Response{
				err:    &Error{Code: 123, Message: "error"},
				result: []byte(`"result2"`),
			},
			assertion: false,
		},
		{
			name: "Same errors and results",
			this: &Response{
				err:    &Error{Code: 123, Message: "error"},
				result: []byte(`"result"`),
			},
			other: &Response{
				err:    &Error{Code: 123, Message: "error"},
				result: []byte(`"result"`),
			},
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

// TestResponse_EqualsMixedLazyEager tests comparison between
// eagerly and lazily unmarshaled responses
func TestResponse_EqualsMixedLazyEager(t *testing.T) {
	t.Run("Mixed ID - eager vs lazy with same ID", func(t *testing.T) {
		// Create one response via DecodeResponse (eager unmarshaling)
		data := []byte(`{"jsonrpc":"2.0","id":42,"result":"success"}`)
		eager, err := DecodeResponse(data)
		require.NoError(t, err)

		// Create another response with raw ID still unparsed
		lazy := &Response{
			jsonrpc: "2.0",
			rawID:   json.RawMessage(`42`),
			result:  json.RawMessage(`"success"`),
		}

		// They should be equal even though one has ID unmarshaled and other doesn't
		assert.True(t, eager.Equals(lazy))
		assert.True(t, lazy.Equals(eager))
	})

	t.Run("Mixed ID - eager vs lazy with different ID", func(t *testing.T) {
		// Create one response via DecodeResponse (eager unmarshaling)
		data := []byte(`{"jsonrpc":"2.0","id":42,"result":"success"}`)
		eager, err := DecodeResponse(data)
		require.NoError(t, err)

		// Create another response with different raw ID
		lazy := &Response{
			jsonrpc: "2.0",
			rawID:   json.RawMessage(`99`),
			result:  json.RawMessage(`"success"`),
		}

		// They should NOT be equal
		assert.False(t, eager.Equals(lazy))
		assert.False(t, lazy.Equals(eager))
	})

	t.Run("Mixed Error - eager vs lazy with same error", func(t *testing.T) {
		// Create one response via DecodeResponse (eager error unmarshaling)
		data := []byte(`{"jsonrpc":"2.0","id":1,"error":{"code":-32000,"message":"test error"}}`)
		eager, err := DecodeResponse(data)
		require.NoError(t, err)

		// Create another response with raw error still unparsed
		lazy := &Response{
			jsonrpc:  "2.0",
			rawID:    json.RawMessage(`1`),
			rawError: json.RawMessage(`{"code":-32000,"message":"test error"}`),
		}

		// They should be equal even though one has Error unmarshaled and other doesn't
		assert.True(t, eager.Equals(lazy))
		assert.True(t, lazy.Equals(eager))
	})

	t.Run("Mixed Error - eager vs lazy with different error", func(t *testing.T) {
		// Create one response via DecodeResponse (eager error unmarshaling)
		data := []byte(`{"jsonrpc":"2.0","id":1,"error":{"code":-32000,"message":"test error"}}`)
		eager, err := DecodeResponse(data)
		require.NoError(t, err)

		// Create another response with different raw error
		lazy := &Response{
			jsonrpc:  "2.0",
			rawID:    json.RawMessage(`1`),
			rawError: json.RawMessage(`{"code":-32001,"message":"different error"}`),
		}

		// They should NOT be equal
		assert.False(t, eager.Equals(lazy))
		assert.False(t, lazy.Equals(eager))
	})

	t.Run("Both lazy - same values", func(t *testing.T) {
		lazy1 := &Response{
			jsonrpc: "2.0",
			rawID:   json.RawMessage(`"test-id"`),
			result:  json.RawMessage(`"result"`),
		}

		lazy2 := &Response{
			jsonrpc: "2.0",
			rawID:   json.RawMessage(`"test-id"`),
			result:  json.RawMessage(`"result"`),
		}

		// Both lazy, same semantic values
		assert.True(t, lazy1.Equals(lazy2))
		assert.True(t, lazy2.Equals(lazy1))
	})
}

func TestResponse_IDString(t *testing.T) {
	t.Run("No ID set => returns empty string", func(t *testing.T) {
		resp := &Response{}
		assert.Equal(t, "", resp.IDString())
	})

	t.Run("ID is string text", func(t *testing.T) {
		resp := &Response{id: "my-unique-id"}
		assert.Equal(t, "my-unique-id", resp.IDString())
	})

	t.Run("ID is string integer", func(t *testing.T) {
		resp := &Response{id: "15"}
		assert.Equal(t, "15", resp.IDString())
	})

	t.Run("ID is string float", func(t *testing.T) {
		resp := &Response{id: "33.75"}
		assert.Equal(t, "33.75", resp.IDString())
	})

	t.Run("ID is int64", func(t *testing.T) {
		resp := &Response{id: int64(12345)}
		assert.Equal(t, "12345", resp.IDString())
	})

	t.Run("ID is float64", func(t *testing.T) {
		resp := &Response{id: float64(12345.67)}
		assert.Equal(t, "12345.67", resp.IDString())
	})

	t.Run("ID is float64 integer value", func(t *testing.T) {
		resp := &Response{id: float64(25.0)}
		assert.Equal(t, "25.0", resp.IDString())
	})

	t.Run("ID is other type => returns empty string", func(t *testing.T) {
		resp := &Response{id: []int{1, 2, 3}}
		assert.Equal(t, "", resp.IDString())
	})
}

func TestResponse_IsEmpty(t *testing.T) {
	t.Run("Results considered empty", func(t *testing.T) {
		cases := []struct {
			name   string
			result []byte
		}{
			{name: "hex zero", result: []byte(`"0x"`)},
			{name: "null", result: []byte(`null`)},
			{name: "empty string", result: []byte(`""`)},
			{name: "empty array", result: []byte(`[]`)},
			{name: "empty object", result: []byte(`{}`)},
			{name: "empty byte slice", result: []byte{}},
		}
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				resp := &Response{result: c.result}
				assert.True(t, resp.IsEmpty(), "expected %q to be IsEmpty == true", c.result)
			})
		}
	})

	t.Run("Results NOT considered empty", func(t *testing.T) {
		cases := []struct {
			name   string
			result []byte
		}{
			{name: "non-zero hex", result: []byte(`"0x1"`)},
			{name: "number", result: []byte(`42`)},
			{name: "non-empty string", result: []byte(`"hello"`)},
			{name: "array with elements", result: []byte(`[1,2,3]`)},
			{name: "object with fields", result: []byte(`{"key":"value"}`)},
			{name: "boolean true", result: []byte(`true`)},
			{name: "boolean false", result: []byte(`false`)},
			{name: "zero number", result: []byte(`0`)},
		}
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				// With non-empty result and empty error, should NOT be empty
				resp := &Response{result: c.result}
				assert.False(t, resp.IsEmpty(), "expected %q to be IsEmpty == false", c.result)
			})
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
				resp: &Response{result: []byte{}},
			},
			{
				name: "Error is empty",
				resp: &Response{err: &Error{}},
			},
			{
				name: "Result and Error are empty",
				resp: &Response{result: []byte{}, err: &Error{}},
			},
			{
				name: "Error without Code or Message",
				resp: &Response{err: &Error{Data: "some data"}},
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
				resp: &Response{result: []byte(`"some-value"`)},
			},
			{
				name: "Error only",
				resp: &Response{err: &Error{Code: 123, Message: "some error"}},
			},
			{
				name: "Result and error",
				resp: &Response{
					result: []byte(`"some-value"`),
					err:    &Error{Code: 123, Message: "some error"},
				},
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
			resp: &Response{
				jsonrpc: "2.0",
				id:      int64(1),
				result:  []byte(`{"foo":"bar"}`),
			},
			json: []byte(`{"jsonrpc":"2.0","id":1,"result":{"foo":"bar"}}`),
		},
		{
			name: "Response with Error",
			resp: &Response{
				jsonrpc: "2.0",
				id:      "first",
				err:     &Error{Code: 123, Message: "test msg"},
			},
			json: []byte(
				`{"jsonrpc":"2.0","id":"first","error":{"code":123,"message":"test msg"}}`,
			),
		},
		{
			name: "Response with rawError and nil ID",
			resp: &Response{
				jsonrpc:  "2.0",
				id:       nil,
				rawError: []byte(`{"code":123,"message":"test msg"}`),
			},
			json: []byte(
				`{"jsonrpc":"2.0","id":null,"error":{"code":123,"message":"test msg"}}`,
			),
		},
		{
			name: "Invalid: both result and error",
			resp: &Response{
				jsonrpc: "2.0",
				id:      "first",
				result:  []byte(`{"foo":"bar"}`),
				err:     &Error{Code: 123, Message: "test msg"},
			},
			runtimeErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			marshaled, err := tc.resp.MarshalJSON()
			if tc.runtimeErr {
				assert.Error(t, err)
				assert.Nil(t, marshaled)
			} else {
				assert.NoError(t, err)
				assert.JSONEq(t, string(tc.json), string(marshaled))
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
		assert.Nil(t, resp.Err())
		assert.NotNil(t, resp.RawResult())

		// Unmarshal the result to check if it's correct
		var resultStr string
		err = json.Unmarshal(resp.RawResult(), &resultStr)
		require.NoError(t, err)
		assert.Equal(t, "OK", resultStr)
	})

	t.Run("Valid JSON with error", func(t *testing.T) {
		raw := []byte(`{"jsonrpc":"2.0","id":42,"error":{"code":-32000}}`)
		resp := &Response{}
		err := resp.parseFromReader(bytes.NewReader(raw), len(raw))
		require.NoError(t, err)
		assert.NotNil(t, resp.rawError)
		assert.Nil(t, resp.RawResult())
	})

	t.Run("Invalid JSON", func(t *testing.T) {
		raw := []byte(`{invalid-json`)
		resp := &Response{}
		err := resp.parseFromReader(bytes.NewReader(raw), len(raw))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "{invalid-json")
		assert.Nil(t, resp.rawError)
		assert.Nil(t, resp.Err())
		assert.Nil(t, resp.RawResult())
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
		assert.Nil(t, resp.Err())
		require.NotNil(t, resp.RawResult())

		// Unmarshal the result to check if it's correct
		var resultStr string
		err = json.Unmarshal(resp.RawResult(), &resultStr)
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
		assert.NotNil(t, resp.RawResult())
		assert.Nil(t, resp.Err())
		assert.Nil(t, resp.rawError)
	})

	t.Run("Valid response with error", func(t *testing.T) {
		raw := []byte(`{"jsonrpc":"2.0","id":1,"error":{"code":-32000}}`)
		resp := &Response{}
		err := resp.parseFromBytes(raw)
		require.NoError(t, err)
		assert.NotNil(t, resp.rawID)
		assert.Nil(t, resp.RawResult())
		// Err() triggers lazy unmarshaling, so it should return the error
		assert.NotNil(t, resp.Err())
		assert.Equal(t, -32000, resp.Err().Code)
		assert.NotNil(t, resp.rawError)
	})

	t.Run("Valid response with extra fields", func(t *testing.T) {
		raw := []byte(`{"jsonrpc":"2.0","id":1,"result":"OK","something":"extra"}`)
		resp := &Response{}
		err := resp.parseFromBytes(raw)
		require.NoError(t, err)
		assert.NotNil(t, resp.rawID)
		assert.NotNil(t, resp.RawResult())
		assert.Nil(t, resp.Err())
		assert.Nil(t, resp.rawError)
	})

	t.Run("Invalid response: both result and error", func(t *testing.T) {
		raw := []byte(`{"jsonrpc":"2.0","id":1,"result":"OK","error":{"core": -32000}}`)
		resp := &Response{}
		err := resp.parseFromBytes(raw)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "response must not contain both result and error")
		assert.Nil(t, resp.rawID)
		assert.Nil(t, resp.RawResult())
		assert.Nil(t, resp.Err())
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
			assert.Nil(t, resp.Err())
			assert.Nil(t, resp.RawResult())
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
			name: "Has id and correctly formed error 1",
			bytes: []byte(
				`{"jsonrpc":"2.0","id":"abc","error":{"code":-123,"message":"some msg"}}`,
			),
			runtimeErr: false,
			respErr: &Error{
				Code:    -123,
				Message: "some msg",
			},
			respID: "abc",
		},
		{
			name: "Has id and correctly formed error 2",
			bytes: []byte(
				`{"jsonrpc":"2.0","id":5,"error":{"code":-1234,"data":"some data"}}`,
			),
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
			name: "Both error and result",
			bytes: []byte(
				`{"jsonrpc":"2.0","id":2,` +
					`"error":{"code":-123,"message":"some msg"},"result":{"foo":"bar"}}`,
			),
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
				assert.Equal(t, c.respErr, resp.Err())
				assert.Equal(t, c.respID, resp.IDOrNil())
				assert.Equal(t, c.respRes, resp.RawResult())
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
			name: "Valid response with result",
			resp: &Response{
				jsonrpc: "2.0",
				id:      int64(1),
				result:  []byte(`{"foo":"bar"}`),
			},
			runtimeErr: false,
		},
		{
			name: "Valid response with error",
			resp: &Response{
				jsonrpc: "2.0",
				id:      "first",
				err:     &Error{Code: 123, Message: "test msg"},
			},
			runtimeErr: false,
		},
		{
			name:       "Invalid JSON-RPC version",
			resp:       &Response{jsonrpc: "1.0", id: int64(1), result: []byte(`{"foo":"bar"}`)},
			runtimeErr: true,
			errMessage: "invalid jsonrpc version",
		},
		{
			name: "Invalid ID type",
			resp: &Response{
				jsonrpc: "2.0",
				id:      []int{1, 2, 3},
				result:  []byte(`{"foo":"bar"}`),
			},
			runtimeErr: true,
			errMessage: "id field must be a string or a number",
		},
		{
			name: "Both result and error",
			resp: &Response{
				jsonrpc: "2.0",
				id:      "first",
				result:  []byte(`{"foo":"bar"}`),
				err:     &Error{Code: 123, Message: "test msg"},
			},
			runtimeErr: true,
			errMessage: "response must not contain both result and error",
		},
		{
			name:       "Neither result nor error",
			resp:       &Response{jsonrpc: "2.0", id: "first"},
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
		assert.Nil(t, resp.Err())
		assert.NotNil(t, resp.RawResult())

		// Unmarshal the result to check if it's correct
		var resultStr string
		err = json.Unmarshal(resp.RawResult(), &resultStr)
		require.NoError(t, err)
		assert.Equal(t, "OK", resultStr)
	})

	t.Run("Valid JSON with error", func(t *testing.T) {
		raw := []byte(`{"jsonrpc":"2.0","id":42,"error":{"code":-32000}}`)
		resp, err := DecodeResponse(raw)
		require.NoError(t, err)
		assert.NotNil(t, resp.rawError)
		assert.Nil(t, resp.RawResult())
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
		assert.Nil(t, resp.Err())
		assert.NotNil(t, resp.RawResult())
	})

	t.Run("Valid JSON with error", func(t *testing.T) {
		raw := []byte(`{"jsonrpc":"2.0","id":42,"error":{"code":-32000}}`)
		resp, err := DecodeResponseFromReader(&readCloser{bytes.NewReader(raw)}, len(raw))
		require.NoError(t, err)
		assert.NotNil(t, resp.rawError)
		assert.Nil(t, resp.RawResult())
	})

	t.Run("Invalid JSON", func(t *testing.T) {
		raw := []byte(`{invalid-json`)
		resp, err := DecodeResponseFromReader(&readCloser{bytes.NewReader(raw)}, len(raw))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "{invalid-json")
		assert.Nil(t, resp)
	})
}

// TestResponse_Concurrency verifies that Response is safe for concurrent reads.
// Responses are immutable after decode, so concurrent access should never race.
func TestResponse_Concurrency(t *testing.T) {
	t.Run("Concurrent IDString", func(t *testing.T) {
		resp := &Response{id: int64(12345)}

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
		resp := &Response{result: []byte(`"0x"`)}

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
		resp1 := &Response{jsonrpc: "2.0", id: int64(1), result: []byte(`{"foo":"bar"}`)}
		resp2 := &Response{jsonrpc: "2.0", id: int64(1), result: []byte(`{"foo":"bar"}`)}

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

func TestResponse_Constructors(t *testing.T) {
	t.Run("NewResponse", func(t *testing.T) {
		resp, err := NewResponse("id", "result")
		require.NoError(t, err)
		assert.Equal(t, "id", resp.IDString())
		assert.Equal(t, "result", string(resp.RawResult()))
	})

	t.Run("NewErrorResponse", func(t *testing.T) {
		resp := NewErrorResponse("id", &Error{Code: -32000, Message: "error"})
		assert.Equal(t, "id", resp.IDString())
		assert.Equal(t, -32000, resp.Err().Code)
		assert.Equal(t, "error", resp.Err().Message)
	})

	t.Run("NewResponseFromRaw", func(t *testing.T) {
		resp, err := NewResponseFromRaw("id", []byte(`"result"`))
		require.NoError(t, err)
		assert.Equal(t, "id", resp.IDString())
		assert.Equal(t, "result", string(resp.RawResult()))
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

	// Error responses are eagerly unmarshaled during DecodeResponse for convenience
	require.NotNil(t, resp.Err())
	assert.Equal(t, -32000, resp.Err().Code)
	assert.Equal(t, "oops", resp.Err().Message)

	// Calling UnmarshalError again should be idempotent and still succeed.
	err = resp.UnmarshalError()
	require.NoError(t, err)
	assert.Equal(t, -32000, resp.Err().Code)
}

// TestResponse_Immutability verifies that Response fields don't change after decode.
func TestResponse_Immutability(t *testing.T) {
	data := []byte(`{"jsonrpc":"2.0","id":123,"result":"success"}`)

	resp, err := DecodeResponse(data)
	require.NoError(t, err)

	// Capture initial state
	originalID := resp.IDOrNil()
	originalResult := string(resp.RawResult())

	// Access ID multiple times (triggers lazy unmarshal on first call)
	id1 := resp.IDOrNil()
	id2 := resp.IDOrNil()
	id3 := resp.IDOrNil()

	// All should return same value
	assert.Equal(t, id1, id2)
	assert.Equal(t, id2, id3)
	assert.Equal(t, originalID, id1)

	// Result should never change
	assert.Equal(t, originalResult, string(resp.RawResult()))

	// Concurrent reads should see consistent state
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			assert.Equal(t, originalID, resp.IDOrNil())
			assert.Equal(t, originalResult, string(resp.RawResult()))
		}()
	}
	wg.Wait()
}

// TestResponse_LazyUnmarshalOnce verifies that unmarshalID runs exactly once.
func TestResponse_LazyUnmarshalOnce(t *testing.T) {
	data := []byte(`{"jsonrpc":"2.0","id":"test-id","result":true}`)

	resp, err := DecodeResponse(data)
	require.NoError(t, err)

	// Call IDOrNil concurrently from multiple goroutines
	var wg sync.WaitGroup
	results := make([]any, 100)

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx] = resp.IDOrNil()
		}(i)
	}
	wg.Wait()

	// All goroutines should see the same result
	for i := 1; i < len(results); i++ {
		assert.Equal(t, results[0], results[i],
			"All concurrent IDOrNil calls should return the same value")
	}
	assert.Equal(t, "test-id", results[0])
}
