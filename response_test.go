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
			wg.Go(func() {
				idStr := resp.IDString()
				assert.Equal(t, "12345", idStr)
			})
		}
		wg.Wait()
	})

	t.Run("Concurrent IsEmpty", func(t *testing.T) {
		resp := &Response{result: []byte(`"0x"`)}

		var wg sync.WaitGroup
		for range 200 {
			wg.Go(func() {
				isEmpty := resp.IsEmpty()
				assert.True(t, isEmpty)
			})
		}
		wg.Wait()
	})

	t.Run("Concurrent Equals", func(t *testing.T) {
		resp1 := &Response{jsonrpc: "2.0", id: int64(1), result: []byte(`{"foo":"bar"}`)}
		resp2 := &Response{jsonrpc: "2.0", id: int64(1), result: []byte(`{"foo":"bar"}`)}

		var wg sync.WaitGroup
		for range 200 {
			wg.Go(func() {
				equal := resp1.Equals(resp2)
				assert.True(t, equal)
			})
		}
		wg.Wait()
	})
}

func TestResponse_Constructors(t *testing.T) {
	t.Run("NewResponse", func(t *testing.T) {
		resp, err := NewResponse("id", "result")
		require.NoError(t, err)
		assert.Equal(t, "id", resp.IDString())
		assert.Equal(t, `"result"`, string(resp.RawResult()))
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
		assert.Equal(t, `"result"`, string(resp.RawResult()))
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
	for range 100 {
		wg.Go(func() {
			assert.Equal(t, originalID, resp.IDOrNil())
			assert.Equal(t, originalResult, string(resp.RawResult()))
		})
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

	for i := range 100 {
		wg.Go(func() {
			results[i] = resp.IDOrNil()
		})
	}
	wg.Wait()

	// All goroutines should see the same result
	for i := 1; i < len(results); i++ {
		assert.Equal(t, results[0], results[i],
			"All concurrent IDOrNil calls should return the same value")
	}
	assert.Equal(t, "test-id", results[0])
}

func TestResponse_Unmarshal(t *testing.T) {
	t.Run("Unmarshal response with result", func(t *testing.T) {
		resp, err := NewResponse(1, map[string]string{"foo": "bar"})
		require.NoError(t, err)

		var out map[string]any
		err = resp.Unmarshal(&out)
		require.NoError(t, err)

		assert.Equal(t, "2.0", out["jsonrpc"])
		assert.Equal(t, float64(1), out["id"])
		assert.Equal(t, map[string]any{"foo": "bar"}, out["result"])
		assert.Nil(t, out["error"])
	})

	t.Run("Unmarshal response with error", func(t *testing.T) {
		resp := NewErrorResponse("test-id", &Error{Code: -32000, Message: "test error"})

		var out map[string]any
		err := resp.Unmarshal(&out)
		require.NoError(t, err)

		assert.Equal(t, "2.0", out["jsonrpc"])
		assert.Equal(t, "test-id", out["id"])
		assert.Nil(t, out["result"])

		errMap, ok := out["error"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, float64(-32000), errMap["code"])
		assert.Equal(t, "test error", errMap["message"])
	})

	t.Run("Unmarshal with nil destination", func(t *testing.T) {
		resp, err := NewResponse(1, "test")
		require.NoError(t, err)

		err = resp.Unmarshal(nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "destination pointer cannot be nil")
	})
}

func TestResponse_WriteTo(t *testing.T) {
	t.Run("Response with result and string ID", func(t *testing.T) {
		resp, err := NewResponse("test-id", map[string]string{"foo": "bar"})
		require.NoError(t, err)

		var buf bytes.Buffer
		n, err := resp.WriteTo(&buf)
		require.NoError(t, err)
		assert.Greater(t, n, int64(0))

		var out map[string]any
		err = json.Unmarshal(buf.Bytes(), &out)
		require.NoError(t, err)

		assert.Equal(t, "2.0", out["jsonrpc"])
		assert.Equal(t, "test-id", out["id"])
		assert.Equal(t, map[string]any{"foo": "bar"}, out["result"])
	})

	t.Run("Response with result and integer ID", func(t *testing.T) {
		resp, err := NewResponse(42, "success")
		require.NoError(t, err)

		var buf bytes.Buffer
		n, err := resp.WriteTo(&buf)
		require.NoError(t, err)
		assert.Greater(t, n, int64(0))

		var out map[string]any
		err = json.Unmarshal(buf.Bytes(), &out)
		require.NoError(t, err)

		assert.Equal(t, "2.0", out["jsonrpc"])
		assert.Equal(t, float64(42), out["id"])
		assert.Equal(t, "success", out["result"])
	})

	t.Run("Response with result and float ID", func(t *testing.T) {
		resp, err := NewResponse(float64(3.14), true)
		require.NoError(t, err)

		var buf bytes.Buffer
		n, err := resp.WriteTo(&buf)
		require.NoError(t, err)
		assert.Greater(t, n, int64(0))

		var out map[string]any
		err = json.Unmarshal(buf.Bytes(), &out)
		require.NoError(t, err)

		assert.Equal(t, "2.0", out["jsonrpc"])
		assert.Equal(t, float64(3.14), out["id"])
		assert.Equal(t, true, out["result"])
	})

	t.Run("Response with result and nil ID", func(t *testing.T) {
		resp, err := NewResponse(nil, "data")
		require.NoError(t, err)

		var buf bytes.Buffer
		n, err := resp.WriteTo(&buf)
		require.NoError(t, err)
		assert.Greater(t, n, int64(0))

		var out map[string]any
		err = json.Unmarshal(buf.Bytes(), &out)
		require.NoError(t, err)

		assert.Equal(t, "2.0", out["jsonrpc"])
		assert.Nil(t, out["id"])
		assert.Equal(t, "data", out["result"])
	})

	t.Run("Response with error", func(t *testing.T) {
		resp := NewErrorResponse("error-id", &Error{Code: -32000, Message: "test error"})

		var buf bytes.Buffer
		n, err := resp.WriteTo(&buf)
		require.NoError(t, err)
		assert.Greater(t, n, int64(0))

		var out map[string]any
		err = json.Unmarshal(buf.Bytes(), &out)
		require.NoError(t, err)

		assert.Equal(t, "2.0", out["jsonrpc"])
		assert.Equal(t, "error-id", out["id"])

		errMap, ok := out["error"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, float64(-32000), errMap["code"])
		assert.Equal(t, "test error", errMap["message"])
	})

	t.Run("Response with rawError", func(t *testing.T) {
		resp := &Response{
			jsonrpc:  "2.0",
			id:       int64(1),
			rawError: []byte(`{"code":-32601,"message":"Method not found"}`),
		}

		var buf bytes.Buffer
		n, err := resp.WriteTo(&buf)
		require.NoError(t, err)
		assert.Greater(t, n, int64(0))

		var out map[string]any
		err = json.Unmarshal(buf.Bytes(), &out)
		require.NoError(t, err)

		assert.Equal(t, "2.0", out["jsonrpc"])
		assert.Equal(t, float64(1), out["id"])

		errMap, ok := out["error"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, float64(-32601), errMap["code"])
		assert.Equal(t, "Method not found", errMap["message"])
	})

	t.Run("Response with rawID", func(t *testing.T) {
		resp := &Response{
			jsonrpc: "2.0",
			rawID:   json.RawMessage(`"raw-id"`),
			result:  json.RawMessage(`"result"`),
		}

		var buf bytes.Buffer
		n, err := resp.WriteTo(&buf)
		require.NoError(t, err)
		assert.Greater(t, n, int64(0))

		var out map[string]any
		err = json.Unmarshal(buf.Bytes(), &out)
		require.NoError(t, err)

		assert.Equal(t, "2.0", out["jsonrpc"])
		assert.Equal(t, "raw-id", out["id"])
		assert.Equal(t, "result", out["result"])
	})

	t.Run("WriteTo produces same output as MarshalJSON", func(t *testing.T) {
		testCases := []struct {
			name string
			resp *Response
		}{
			{
				name: "Response with result",
				resp: func() *Response {
					r, _ := NewResponse(1, map[string]string{"foo": "bar"})
					return r
				}(),
			},
			{
				name: "Response with error",
				resp: NewErrorResponse("error-id", &Error{Code: -32000, Message: "test"}),
			},
			{
				name: "Response with nil ID",
				resp: func() *Response {
					r, _ := NewResponse(nil, "result")
					return r
				}(),
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				var buf bytes.Buffer
				n, err := tc.resp.WriteTo(&buf)
				require.NoError(t, err)
				assert.Greater(t, n, int64(0))

				marshaled, err := tc.resp.MarshalJSON()
				require.NoError(t, err)

				assert.JSONEq(t, string(marshaled), buf.String())
			})
		}
	})

	t.Run("Large response with WriteTo", func(t *testing.T) {
		largeData := make([]map[string]string, 1000)
		for i := range largeData {
			largeData[i] = map[string]string{
				"index": fmt.Sprintf("%d", i),
				"data":  "some data here",
			}
		}

		resp, err := NewResponse(1, largeData)
		require.NoError(t, err)

		var buf bytes.Buffer
		n, err := resp.WriteTo(&buf)
		require.NoError(t, err)
		assert.Greater(t, n, int64(10000))

		var out map[string]any
		err = json.Unmarshal(buf.Bytes(), &out)
		require.NoError(t, err)

		assert.Equal(t, "2.0", out["jsonrpc"])
		assert.Equal(t, float64(1), out["id"])

		resultSlice, ok := out["result"].([]any)
		require.True(t, ok)
		assert.Len(t, resultSlice, 1000)
	})

	t.Run("Invalid response validation fails", func(t *testing.T) {
		resp := &Response{
			jsonrpc: "1.0",
			id:      int64(1),
			result:  json.RawMessage(`"test"`),
		}

		var buf bytes.Buffer
		_, err := resp.WriteTo(&buf)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid jsonrpc version")
	})

	t.Run("WriteTo returns correct byte count", func(t *testing.T) {
		resp, err := NewResponse(1, "test")
		require.NoError(t, err)

		var buf bytes.Buffer
		n, err := resp.WriteTo(&buf)
		require.NoError(t, err)

		assert.Equal(t, int64(buf.Len()), n)
	})
}

func TestResponse_PeekStringByPath(t *testing.T) {
	t.Run("Extract top-level string field", func(t *testing.T) {
		resp, err := NewResponse(1, map[string]any{
			"blockNumber": "0x1234",
			"hash":        "0xabcdef",
		})
		require.NoError(t, err)

		blockNum, err := resp.PeekStringByPath("blockNumber")
		require.NoError(t, err)
		assert.Equal(t, "0x1234", blockNum)

		hash, err := resp.PeekStringByPath("hash")
		require.NoError(t, err)
		assert.Equal(t, "0xabcdef", hash)
	})

	t.Run("Extract nested string field", func(t *testing.T) {
		resp, err := NewResponse(1, map[string]any{
			"transaction": map[string]any{
				"from": "0x123",
				"to":   "0x456",
			},
		})
		require.NoError(t, err)

		from, err := resp.PeekStringByPath("transaction", "from")
		require.NoError(t, err)
		assert.Equal(t, "0x123", from)

		to, err := resp.PeekStringByPath("transaction", "to")
		require.NoError(t, err)
		assert.Equal(t, "0x456", to)
	})

	t.Run("Extract deeply nested field", func(t *testing.T) {
		resp, err := NewResponse(1, map[string]any{
			"level1": map[string]any{
				"level2": map[string]any{
					"level3": map[string]any{
						"value": "deep",
					},
				},
			},
		})
		require.NoError(t, err)

		value, err := resp.PeekStringByPath("level1", "level2", "level3", "value")
		require.NoError(t, err)
		assert.Equal(t, "deep", value)
	})

	t.Run("No path returns entire result as string (if string)", func(t *testing.T) {
		resp, err := NewResponse(1, "simple-string")
		require.NoError(t, err)

		value, err := resp.PeekStringByPath()
		require.NoError(t, err)
		assert.Equal(t, "simple-string", value)
	})

	t.Run("Error when response has no result", func(t *testing.T) {
		resp := NewErrorResponse(1, &Error{Code: -32000, Message: "error"})

		_, err := resp.PeekStringByPath("field")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no result field")
	})

	t.Run("Error when path not found", func(t *testing.T) {
		resp, err := NewResponse(1, map[string]any{
			"field1": "value1",
		})
		require.NoError(t, err)

		_, err = resp.PeekStringByPath("nonexistent")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "path not found")
	})

	t.Run("Number value gets converted to string by sonic", func(t *testing.T) {
		resp, err := NewResponse(1, map[string]any{
			"number": 42,
		})
		require.NoError(t, err)

		// sonic's node.String() converts numbers to strings
		val, err := resp.PeekStringByPath("number")
		require.NoError(t, err)
		assert.Equal(t, "42", val)
	})

	t.Run("AST node is cached for repeated calls", func(t *testing.T) {
		resp, err := NewResponse(1, map[string]any{
			"field1": "value1",
			"field2": "value2",
			"field3": "value3",
		})
		require.NoError(t, err)

		// First call builds AST
		val1, err := resp.PeekStringByPath("field1")
		require.NoError(t, err)
		assert.Equal(t, "value1", val1)

		// Subsequent calls reuse cached AST
		val2, err := resp.PeekStringByPath("field2")
		require.NoError(t, err)
		assert.Equal(t, "value2", val2)

		val3, err := resp.PeekStringByPath("field3")
		require.NoError(t, err)
		assert.Equal(t, "value3", val3)
	})

	t.Run("Thread-safe concurrent access", func(t *testing.T) {
		resp, err := NewResponse(1, map[string]any{
			"blockNumber": "0x1234",
			"hash":        "0xabcdef",
			"timestamp":   "0x999",
		})
		require.NoError(t, err)

		var wg sync.WaitGroup
		for range 100 {
			wg.Go(func() {
				val, err := resp.PeekStringByPath("blockNumber")
				assert.NoError(t, err)
				assert.Equal(t, "0x1234", val)
			})
			wg.Go(func() {
				val, err := resp.PeekStringByPath("hash")
				assert.NoError(t, err)
				assert.Equal(t, "0xabcdef", val)
			})
			wg.Go(func() {
				val, err := resp.PeekStringByPath("timestamp")
				assert.NoError(t, err)
				assert.Equal(t, "0x999", val)
			})
		}
		wg.Wait()
	})
}

func TestResponse_PeekBytesByPath(t *testing.T) {
	t.Run("Extract nested object as bytes", func(t *testing.T) {
		resp, err := NewResponse(1, map[string]any{
			"transaction": map[string]any{
				"from":  "0x123",
				"to":    "0x456",
				"value": "1000",
			},
		})
		require.NoError(t, err)

		txBytes, err := resp.PeekBytesByPath("transaction")
		require.NoError(t, err)

		var tx map[string]string
		err = json.Unmarshal(txBytes, &tx)
		require.NoError(t, err)
		assert.Equal(t, "0x123", tx["from"])
		assert.Equal(t, "0x456", tx["to"])
		assert.Equal(t, "1000", tx["value"])
	})

	t.Run("Extract array as bytes", func(t *testing.T) {
		resp, err := NewResponse(1, map[string]any{
			"logs": []any{
				map[string]string{"event": "Transfer"},
				map[string]string{"event": "Approval"},
			},
		})
		require.NoError(t, err)

		logsBytes, err := resp.PeekBytesByPath("logs")
		require.NoError(t, err)

		var logs []map[string]string
		err = json.Unmarshal(logsBytes, &logs)
		require.NoError(t, err)
		assert.Len(t, logs, 2)
		assert.Equal(t, "Transfer", logs[0]["event"])
		assert.Equal(t, "Approval", logs[1]["event"])
	})

	t.Run("Extract primitive value as bytes", func(t *testing.T) {
		resp, err := NewResponse(1, map[string]any{
			"number": 42,
		})
		require.NoError(t, err)

		numBytes, err := resp.PeekBytesByPath("number")
		require.NoError(t, err)
		assert.Equal(t, "42", string(numBytes))

		var num int
		err = json.Unmarshal(numBytes, &num)
		require.NoError(t, err)
		assert.Equal(t, 42, num)
	})

	t.Run("No path returns entire result as bytes", func(t *testing.T) {
		resp, err := NewResponse(1, map[string]string{
			"key": "value",
		})
		require.NoError(t, err)

		resultBytes, err := resp.PeekBytesByPath()
		require.NoError(t, err)

		var result map[string]string
		err = json.Unmarshal(resultBytes, &result)
		require.NoError(t, err)
		assert.Equal(t, "value", result["key"])
	})

	t.Run("Error when response has no result", func(t *testing.T) {
		resp := NewErrorResponse(1, &Error{Code: -32000, Message: "error"})

		_, err := resp.PeekBytesByPath("field")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no result field")
	})

	t.Run("Error when path not found", func(t *testing.T) {
		resp, err := NewResponse(1, map[string]any{
			"field1": "value1",
		})
		require.NoError(t, err)

		_, err = resp.PeekBytesByPath("nonexistent")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "path not found")
	})

	t.Run("Extract and unmarshal large nested structure", func(t *testing.T) {
		largeBlock := map[string]any{
			"blockNumber": "0x1234",
			"transactions": []any{
				map[string]string{"hash": "0xaaa", "from": "0x111"},
				map[string]string{"hash": "0xbbb", "from": "0x222"},
				map[string]string{"hash": "0xccc", "from": "0x333"},
			},
			"metadata": map[string]any{
				"gasUsed":  "21000",
				"gasLimit": "8000000",
			},
		}

		resp, err := NewResponse(1, largeBlock)
		require.NoError(t, err)

		// Extract just the transactions array
		txsBytes, err := resp.PeekBytesByPath("transactions")
		require.NoError(t, err)

		var txs []map[string]string
		err = json.Unmarshal(txsBytes, &txs)
		require.NoError(t, err)
		assert.Len(t, txs, 3)
		assert.Equal(t, "0xaaa", txs[0]["hash"])

		// Extract just the metadata
		metaBytes, err := resp.PeekBytesByPath("metadata")
		require.NoError(t, err)

		var meta map[string]string
		err = json.Unmarshal(metaBytes, &meta)
		require.NoError(t, err)
		assert.Equal(t, "21000", meta["gasUsed"])
	})

	t.Run("Thread-safe concurrent access", func(t *testing.T) {
		resp, err := NewResponse(1, map[string]any{
			"obj1": map[string]string{"key": "value1"},
			"obj2": map[string]string{"key": "value2"},
		})
		require.NoError(t, err)

		var wg sync.WaitGroup
		for range 100 {
			wg.Go(func() {
				bytes, err := resp.PeekBytesByPath("obj1")
				assert.NoError(t, err)
				assert.Contains(t, string(bytes), "value1")
			})
			wg.Go(func() {
				bytes, err := resp.PeekBytesByPath("obj2")
				assert.NoError(t, err)
				assert.Contains(t, string(bytes), "value2")
			})
		}
		wg.Wait()
	})
}

func TestResponse_Clone(t *testing.T) {
	t.Run("Clone response with result", func(t *testing.T) {
		original, err := NewResponse("test-id", map[string]string{"key": "value"})
		require.NoError(t, err)

		clone, err := original.Clone()
		require.NoError(t, err)
		require.NotNil(t, clone)

		// Verify fields are equal
		assert.Equal(t, original.Version(), clone.Version())
		assert.Equal(t, original.IDOrNil(), clone.IDOrNil())
		assert.Equal(t, original.RawResult(), clone.RawResult())

		// Verify they are equal using Equals
		assert.True(t, original.Equals(clone))
	})

	t.Run("Clone response with error", func(t *testing.T) {
		original := NewErrorResponse(int64(42), &Error{Code: -32000, Message: "test error"})

		clone, err := original.Clone()
		require.NoError(t, err)
		require.NotNil(t, clone)

		// Verify fields are equal
		assert.Equal(t, original.Version(), clone.Version())
		assert.Equal(t, original.IDOrNil(), clone.IDOrNil())
		assert.Equal(t, original.Err().Code, clone.Err().Code)
		assert.Equal(t, original.Err().Message, clone.Err().Message)

		// Verify they are equal using Equals
		assert.True(t, original.Equals(clone))
	})

	t.Run("Clone with different ID types", func(t *testing.T) {
		testCases := []struct {
			name string
			id   any
		}{
			{"string ID", "string-id"},
			{"int64 ID", int64(123)},
			{"float64 ID", float64(3.14)},
			{"nil ID", nil},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				original, err := NewResponse(tc.id, "result")
				require.NoError(t, err)

				clone, err := original.Clone()
				require.NoError(t, err)

				assert.Equal(t, original.IDOrNil(), clone.IDOrNil())
				assert.True(t, original.Equals(clone))
			})
		}
	})

	t.Run("Deep copy - no shared byte slice references", func(t *testing.T) {
		original, err := NewResponse(1, "original-result")
		require.NoError(t, err)

		clone, err := original.Clone()
		require.NoError(t, err)

		// Verify result byte slices are different
		originalBytes := original.RawResult()
		cloneBytes := clone.RawResult()

		// Content should be equal
		assert.Equal(t, originalBytes, cloneBytes)

		// But slices should have different backing arrays
		// Modify clone's bytes and verify original is unchanged
		if len(cloneBytes) > 0 {
			cloneBytes[0] = 'X'
			assert.NotEqual(t, originalBytes[0], cloneBytes[0])
		}
	})

	t.Run("Deep copy with rawID", func(t *testing.T) {
		// Create response via decoding to ensure rawID is set
		data := []byte(`{"jsonrpc":"2.0","id":"test-id","result":"data"}`)
		original, err := DecodeResponse(data)
		require.NoError(t, err)

		clone, err := original.Clone()
		require.NoError(t, err)

		// Verify rawID is copied
		assert.Equal(t, original.IDOrNil(), clone.IDOrNil())

		// Modify clone and verify original is unaffected
		cloneData, err := clone.MarshalJSON()
		require.NoError(t, err)
		assert.Contains(t, string(cloneData), "test-id")
	})

	t.Run("Deep copy with rawError", func(t *testing.T) {
		// Create response via decoding to ensure rawError is set
		data := []byte(`{"jsonrpc":"2.0","id":1,"error":{"code":-32000,"message":"test"}}`)
		original, err := DecodeResponse(data)
		require.NoError(t, err)

		clone, err := original.Clone()
		require.NoError(t, err)

		// Verify error is copied
		assert.Equal(t, original.Err().Code, clone.Err().Code)
		assert.Equal(t, original.Err().Message, clone.Err().Message)
	})

	t.Run("Clone preserves large result data", func(t *testing.T) {
		largeData := make([]map[string]string, 1000)
		for i := range largeData {
			largeData[i] = map[string]string{
				"index": fmt.Sprintf("%d", i),
				"data":  "some data",
			}
		}

		original, err := NewResponse(1, largeData)
		require.NoError(t, err)

		clone, err := original.Clone()
		require.NoError(t, err)

		assert.True(t, original.Equals(clone))

		// Verify we can unmarshal both independently
		var originalResult []map[string]string
		err = original.UnmarshalResult(&originalResult)
		require.NoError(t, err)

		var cloneResult []map[string]string
		err = clone.UnmarshalResult(&cloneResult)
		require.NoError(t, err)

		assert.Equal(t, originalResult, cloneResult)
		assert.Len(t, cloneResult, 1000)
	})

	t.Run("AST node cache is not shared", func(t *testing.T) {
		original, err := NewResponse(1, map[string]string{
			"field": "value",
		})
		require.NoError(t, err)

		// Build AST node on original
		val, err := original.PeekStringByPath("field")
		require.NoError(t, err)
		assert.Equal(t, "value", val)

		// Clone should not have AST node cached
		clone, err := original.Clone()
		require.NoError(t, err)

		// Clone should be able to build its own AST node
		val, err = clone.PeekStringByPath("field")
		require.NoError(t, err)
		assert.Equal(t, "value", val)
	})

	t.Run("Error on nil response", func(t *testing.T) {
		var nilResp *Response
		clone, err := nilResp.Clone()
		require.Error(t, err)
		assert.Nil(t, clone)
		assert.Contains(t, err.Error(), "cannot clone nil response")
	})

	t.Run("Clone is independent - modifications don't affect original", func(t *testing.T) {
		original, err := NewResponse("original-id", "original-result")
		require.NoError(t, err)

		clone, err := original.Clone()
		require.NoError(t, err)

		// Get marshaled versions
		originalJSON, err := original.MarshalJSON()
		require.NoError(t, err)

		cloneJSON, err := clone.MarshalJSON()
		require.NoError(t, err)

		// They should be equal
		assert.JSONEq(t, string(originalJSON), string(cloneJSON))

		// Verify independence by checking internal state
		assert.Equal(t, original.IDOrNil(), clone.IDOrNil())
		assert.Equal(t, string(original.RawResult()), string(clone.RawResult()))
	})

	t.Run("Clone with Error.Data field", func(t *testing.T) {
		original := NewErrorResponse(1, &Error{
			Code:    -32000,
			Message: "error",
			Data:    map[string]string{"detail": "extra info"},
		})

		clone, err := original.Clone()
		require.NoError(t, err)

		// Verify error data is copied
		assert.Equal(t, original.Err().Code, clone.Err().Code)
		assert.Equal(t, original.Err().Message, clone.Err().Message)
		assert.Equal(t, original.Err().Data, clone.Err().Data)
	})

	t.Run("Clone respects immutability - concurrent cloning", func(t *testing.T) {
		original, err := NewResponse(1, "data")
		require.NoError(t, err)

		var wg sync.WaitGroup
		clones := make([]*Response, 100)

		for i := range 100 {
			wg.Go(func() {
				clone, err := original.Clone()
				assert.NoError(t, err)
				clones[i] = clone
			})
		}
		wg.Wait()

		// All clones should be equal to original
		for i, clone := range clones {
			assert.True(t, original.Equals(clone), "Clone %d should equal original", i)
		}
	})
}

func TestResponse_WithID(t *testing.T) {
	t.Run("replaces id and preserves original", func(t *testing.T) {
		data := []byte(`{"jsonrpc":"2.0","id":"orig","result":"payload"}`)
		original, err := DecodeResponse(data)
		require.NoError(t, err)

		updated, err := original.WithID(int64(99))
		require.NoError(t, err)

		// Original remains untouched
		assert.Equal(t, "orig", original.IDString())

		// Updated clone reflects new ID and marshals accordingly
		assert.Equal(t, int64(99), updated.IDOrNil())
		updatedJSON, err := updated.MarshalJSON()
		require.NoError(t, err)
		assert.JSONEq(t, `{"jsonrpc":"2.0","id":99,"result":"payload"}`, string(updatedJSON))

		originalJSON, err := original.MarshalJSON()
		require.NoError(t, err)
		assert.JSONEq(t, `{"jsonrpc":"2.0","id":"orig","result":"payload"}`, string(originalJSON))
	})

	t.Run("nil id marshals to null", func(t *testing.T) {
		resp, err := NewResponse("initial", map[string]any{"key": "value"})
		require.NoError(t, err)

		updated, err := resp.WithID(nil)
		require.NoError(t, err)
		assert.Nil(t, updated.IDOrNil())

		bytes, err := updated.MarshalJSON()
		require.NoError(t, err)
		assert.Contains(t, string(bytes), `"id":null`)
	})

	t.Run("invalid id returns error", func(t *testing.T) {
		resp, err := NewResponse(1, "ok")
		require.NoError(t, err)

		updated, err := resp.WithID(struct{}{})
		require.Error(t, err)
		assert.Nil(t, updated)
		assert.Contains(t, err.Error(), "invalid response after id update")
	})

	t.Run("nil receiver returns error", func(t *testing.T) {
		var resp *Response
		updated, err := resp.WithID("new")
		require.Error(t, err)
		assert.Nil(t, updated)
		assert.Contains(t, err.Error(), "cannot update id on nil response")
	})
}

func TestResponse_Size(t *testing.T) {
	t.Run("Size of response with string result", func(t *testing.T) {
		resp, err := NewResponse("test-id", "result-value")
		require.NoError(t, err)

		size := resp.Size()
		assert.Greater(t, size, 0)

		// Verify size is close to marshaled size
		marshaled, err := resp.MarshalJSON()
		require.NoError(t, err)
		actualSize := len(marshaled)

		// Size should be within reasonable range (±10% of actual)
		diff := actualSize - size
		if diff < 0 {
			diff = -diff
		}
		tolerance := actualSize / 10
		assert.LessOrEqual(t, diff, tolerance,
			"Size estimate %d should be within 10%% of actual size %d", size, actualSize)
	})

	t.Run("Size of response with integer ID", func(t *testing.T) {
		resp, err := NewResponse(42, "data")
		require.NoError(t, err)

		size := resp.Size()
		marshaled, err := resp.MarshalJSON()
		require.NoError(t, err)

		assert.InDelta(t, len(marshaled), size, float64(len(marshaled))*0.15)
	})

	t.Run("Size of response with float ID", func(t *testing.T) {
		resp, err := NewResponse(float64(3.14), "data")
		require.NoError(t, err)

		size := resp.Size()
		assert.Greater(t, size, 0)
	})

	t.Run("Size of response with nil ID", func(t *testing.T) {
		resp, err := NewResponse(nil, "data")
		require.NoError(t, err)

		size := resp.Size()
		marshaled, err := resp.MarshalJSON()
		require.NoError(t, err)

		assert.InDelta(t, len(marshaled), size, float64(len(marshaled))*0.15)
	})

	t.Run("Size of error response", func(t *testing.T) {
		resp := NewErrorResponse(1, &Error{Code: -32000, Message: "Method not found"})

		size := resp.Size()
		marshaled, err := resp.MarshalJSON()
		require.NoError(t, err)

		assert.InDelta(t, len(marshaled), size, float64(len(marshaled))*0.15)
	})

	t.Run("Size of large response", func(t *testing.T) {
		largeData := make([]map[string]string, 1000)
		for i := range largeData {
			largeData[i] = map[string]string{
				"index": fmt.Sprintf("%d", i),
				"data":  "some data here that adds to size",
			}
		}

		resp, err := NewResponse(1, largeData)
		require.NoError(t, err)

		size := resp.Size()
		marshaled, err := resp.MarshalJSON()
		require.NoError(t, err)
		actualSize := len(marshaled)

		// For large responses, size should be fairly accurate
		assert.Greater(t, size, 10000, "Large response should have size > 10KB")
		assert.InDelta(t, actualSize, size, float64(actualSize)*0.15)
	})

	t.Run("Size of nil response", func(t *testing.T) {
		var nilResp *Response
		size := nilResp.Size()
		assert.Equal(t, 0, size)
	})

	t.Run("Size with different ID types", func(t *testing.T) {
		testCases := []struct {
			name string
			id   any
		}{
			{"small int", int64(1)},
			{"large int", int64(9223372036854775807)},
			{"negative int", int64(-32000)},
			{"zero int", int64(0)},
			{"short string", "id"},
			{"long string", "very-long-identifier-with-many-characters"},
			{"float", float64(123.456)},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				resp, err := NewResponse(tc.id, "result")
				require.NoError(t, err)

				size := resp.Size()
				marshaled, err := resp.MarshalJSON()
				require.NoError(t, err)

				assert.InDelta(t, len(marshaled), size, float64(len(marshaled))*0.2)
			})
		}
	})

	t.Run("Size with error containing Data", func(t *testing.T) {
		resp := NewErrorResponse(1, &Error{
			Code:    -32000,
			Message: "Server error",
			Data:    map[string]string{"detail": "additional information"},
		})

		size := resp.Size()
		assert.Greater(t, size, 0)
	})

	t.Run("Size accuracy for typical responses", func(t *testing.T) {
		// Test several typical JSON-RPC responses
		testCases := []struct {
			name    string
			resp    *Response
			minSize int
			maxSize int
		}{
			{
				name: "eth_blockNumber response",
				resp: func() *Response {
					r, _ := NewResponse(1, "0x1234567")
					return r
				}(),
				minSize: 40,
				maxSize: 100,
			},
			{
				name: "error response",
				resp: NewErrorResponse("abc", &Error{
					Code:    -32601,
					Message: "Method not found",
				}),
				minSize: 60,
				maxSize: 120,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				size := tc.resp.Size()
				assert.GreaterOrEqual(t, size, tc.minSize)
				assert.LessOrEqual(t, size, tc.maxSize)

				// Compare with actual marshaled size
				marshaled, err := tc.resp.MarshalJSON()
				require.NoError(t, err)
				actualSize := len(marshaled)

				assert.InDelta(t, actualSize, size, float64(actualSize)*0.2)
			})
		}
	})

	t.Run("Size with rawID from decoding", func(t *testing.T) {
		data := []byte(`{"jsonrpc":"2.0","id":"decoded-id","result":"data"}`)
		resp, err := DecodeResponse(data)
		require.NoError(t, err)

		size := resp.Size()
		assert.Greater(t, size, 0)

		marshaled, err := resp.MarshalJSON()
		require.NoError(t, err)
		assert.InDelta(t, len(marshaled), size, float64(len(marshaled))*0.15)
	})

	t.Run("Size with rawError from decoding", func(t *testing.T) {
		data := []byte(`{"jsonrpc":"2.0","id":1,"error":{"code":-32000,"message":"test error"}}`)
		resp, err := DecodeResponse(data)
		require.NoError(t, err)

		size := resp.Size()
		marshaled, err := resp.MarshalJSON()
		require.NoError(t, err)
		assert.InDelta(t, len(marshaled), size, float64(len(marshaled))*0.15)
	})

	t.Run("Size is consistent across multiple calls", func(t *testing.T) {
		resp, err := NewResponse(1, "data")
		require.NoError(t, err)

		size1 := resp.Size()
		size2 := resp.Size()
		size3 := resp.Size()

		assert.Equal(t, size1, size2)
		assert.Equal(t, size2, size3)
	})
}

func TestResponse_Free(t *testing.T) {
	t.Run("Free releases byte slices", func(t *testing.T) {
		resp, err := NewResponse(1, map[string]string{"key": "value"})
		require.NoError(t, err)

		// Verify fields are populated before Free
		assert.NotNil(t, resp.RawResult())
		assert.Greater(t, len(resp.RawResult()), 0)

		// Call Free
		resp.Free()

		// Verify byte slices are nil
		assert.Nil(t, resp.RawResult())
	})

	t.Run("Free keeps parsed values for logging", func(t *testing.T) {
		resp, err := NewResponse(42, "test-data")
		require.NoError(t, err)

		// Force validation to normalize int to int64
		_, err = resp.MarshalJSON()
		require.NoError(t, err)

		// Get ID after normalization
		idBefore := resp.IDOrNil()
		assert.Equal(t, int64(42), idBefore)

		// Call Free
		resp.Free()

		// Verify parsed ID is still accessible
		idAfter := resp.IDOrNil()
		assert.Equal(t, int64(42), idAfter)
	})

	t.Run("Free on error response", func(t *testing.T) {
		resp := NewErrorResponse("test-id", &Error{
			Code:    -32000,
			Message: "test error",
			Data:    map[string]string{"detail": "extra"},
		})

		// Verify error is accessible before Free
		assert.NotNil(t, resp.Err())
		assert.Equal(t, -32000, resp.Err().Code)

		// Call Free
		resp.Free()

		// Verify error is still accessible for logging
		assert.NotNil(t, resp.Err())
		assert.Equal(t, -32000, resp.Err().Code)
		assert.Equal(t, "test error", resp.Err().Message)
	})

	t.Run("Free releases AST cache", func(t *testing.T) {
		resp, err := NewResponse(1, map[string]string{"field": "value"})
		require.NoError(t, err)

		// Build AST cache
		val, err := resp.PeekStringByPath("field")
		require.NoError(t, err)
		assert.Equal(t, "value", val)

		// Call Free
		resp.Free()

		// AST node should be released (zero value)
		// After Free, result is nil, so PeekStringByPath should fail
		// or return an error due to invalid AST
		_, err = resp.PeekStringByPath("field")
		require.Error(t, err)
		// Error could be "no result field" or "path not found" depending on AST state
	})

	t.Run("Free on nil response is safe", func(t *testing.T) {
		var resp *Response
		assert.NotPanics(t, func() {
			resp.Free()
		})
	})

	t.Run("Free with rawID and rawError fields", func(t *testing.T) {
		// Create a response by unmarshaling (which populates rawID, rawError)
		data := []byte(`{"jsonrpc":"2.0","id":"test-id","error":{"code":-32000,"message":"error"}}`)
		resp, err := DecodeResponse(data)
		require.NoError(t, err)

		// IDRaw() triggers unmarshal, which populates r.id from r.rawID
		// So IDRaw() returns the parsed ID, not the raw bytes
		idBefore := resp.IDOrNil()
		assert.Equal(t, "test-id", idBefore)

		// Call Free - releases rawID but keeps r.id
		resp.Free()

		// IDOrNil still returns the parsed value (kept for logging)
		idAfter := resp.IDOrNil()
		assert.Equal(t, "test-id", idAfter)
	})

	t.Run("Marshal fails after Free", func(t *testing.T) {
		resp, err := NewResponse(1, "data")
		require.NoError(t, err)

		// Marshal should work before Free
		_, err = resp.MarshalJSON()
		require.NoError(t, err)

		// Call Free
		resp.Free()

		// Marshal should fail after Free (no result field)
		_, err = resp.MarshalJSON()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "response must contain either result or error")
	})

	t.Run("WriteTo fails after Free", func(t *testing.T) {
		resp, err := NewResponse(1, "test")
		require.NoError(t, err)

		// WriteTo should work before Free
		var buf bytes.Buffer
		_, err = resp.WriteTo(&buf)
		require.NoError(t, err)

		// Call Free
		resp.Free()

		// WriteTo should fail after Free
		buf.Reset()
		_, err = resp.WriteTo(&buf)
		require.Error(t, err)
	})

	t.Run("Free releases memory for large responses", func(t *testing.T) {
		// Create a large response
		largeData := make([]map[string]string, 1000)
		for i := range largeData {
			largeData[i] = map[string]string{
				"index": fmt.Sprintf("%d", i),
				"data":  "some large data here that uses significant memory",
			}
		}

		resp, err := NewResponse(1, largeData)
		require.NoError(t, err)

		// Verify result is large
		resultSize := len(resp.RawResult())
		assert.Greater(t, resultSize, 10000)

		// Call Free
		resp.Free()

		// Verify result is released
		assert.Nil(t, resp.RawResult())
	})

	t.Run("Multiple Free calls are safe", func(t *testing.T) {
		resp, err := NewResponse(1, "data")
		require.NoError(t, err)

		// Call Free multiple times
		assert.NotPanics(t, func() {
			resp.Free()
			resp.Free()
			resp.Free()
		})
	})
}

func TestResponse_IntToInt64Normalization(t *testing.T) {
	t.Run("int ID is normalized to int64", func(t *testing.T) {
		// Use plain int literal
		resp, err := NewResponse(42, "data")
		require.NoError(t, err)

		// Should be normalized to int64 after validation
		marshaled, err := resp.MarshalJSON()
		require.NoError(t, err)

		// Verify it marshals correctly
		assert.Contains(t, string(marshaled), `"id":42`)

		// Verify IDOrNil returns int64
		id := resp.IDOrNil()
		assert.Equal(t, int64(42), id)
		assert.IsType(t, int64(0), id)
	})

	t.Run("int ID in NewErrorResponse", func(t *testing.T) {
		resp := NewErrorResponse(123, &Error{Code: -32000, Message: "error"})

		marshaled, err := resp.MarshalJSON()
		require.NoError(t, err)
		assert.Contains(t, string(marshaled), `"id":123`)

		id := resp.IDOrNil()
		assert.Equal(t, int64(123), id)
		assert.IsType(t, int64(0), id)
	})

	t.Run("WriteTo works with int ID", func(t *testing.T) {
		resp, err := NewResponse(999, "test")
		require.NoError(t, err)

		var buf bytes.Buffer
		n, err := resp.WriteTo(&buf)
		require.NoError(t, err)
		assert.Greater(t, n, int64(0))

		assert.Contains(t, buf.String(), `"id":999`)
	})
}
