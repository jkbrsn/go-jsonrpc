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

// TODO: extend with more cases
func TestResponse_MarshalJSON(t *testing.T) {
	cases := []struct {
		name       string
		resp       *Response
		runtimeErr bool
		json       []byte
	}{
		{
			name: "Valid Response with result",
			resp: &Response{JSONRPC: "2.0", ID: int64(1), Result: []byte(`{"foo":"bar"}`)},
			json: []byte(`{"jsonrpc":"2.0","id":1,"result":{"foo":"bar"}}`),
		},
		{
			name: "Valid Response with Error",
			resp: &Response{JSONRPC: "2.0", ID: "first", Error: &Error{Code: 123, Message: "test msg"}},
			json: []byte(`{"jsonrpc":"2.0","id":"first","error":{"code":123,"message":"test msg"}}`),
		},
		{
			name: "Valid Response with errBytes",
			resp: &Response{JSONRPC: "2.0", ID: nil, errBytes: []byte(`{"code":123,"message":"test msg"}`)},
			json: []byte(`{"jsonrpc":"2.0","id":null,"error":{"code":123,"message":"test msg"}}`),
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

func TestResponse_ParseFromStream(t *testing.T) {
	t.Run("Invalid JSON", func(t *testing.T) {
		raw := []byte(`{invalid-json`)
		resp := &Response{}
		err := resp.ParseFromStream(bytes.NewReader(raw), len(raw))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "{invalid-json")
		assert.Nil(t, resp.Error)
		assert.Nil(t, resp.Result)
	})

	t.Run("Nil reader", func(t *testing.T) {
		resp := &Response{}
		err := resp.ParseFromStream(nil, 12)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot read from nil reader")
	})

	t.Run("Reader error", func(t *testing.T) {
		resp := &Response{}
		err := resp.ParseFromStream(errReader("some read error"), 100)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "some read error")
	})
}

func TestResponse_ParseFromBytes(t *testing.T) {
	t.Run("Valid response with result", func(t *testing.T) {
		raw := []byte(`{"jsonrpc":"2.0","id":1,"result":{"foo":"bar"}}`)
		resp := &Response{}
		err := resp.ParseFromBytes(raw)
		require.NoError(t, err)
		assert.NotNil(t, resp.rawID)
		assert.NotNil(t, resp.Result)
		assert.Nil(t, resp.Error)
		assert.Nil(t, resp.errBytes)
	})
	t.Run("Invalid JSON", func(t *testing.T) {
		raw := []byte(`{invalid-json`)
		resp := &Response{}
		err := resp.ParseFromBytes(raw)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "{invalid-json")
		assert.Nil(t, resp.Error)
		assert.Nil(t, resp.Result)
	})

	t.Run("Nil data", func(t *testing.T) {
		resp := &Response{}
		err := resp.ParseFromBytes(nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "input json is empty")
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

func TestResponseFromStream(t *testing.T) {
	t.Run("Nil reader => error", func(t *testing.T) {
		resp, err := ResponseFromStream(nil, 0)
		require.Error(t, err)
		require.Nil(t, resp)
	})

	// Content based test cases
	// TODO: more cases, and make sure the parsing actually denies malformed JSON-RPC responses
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

// TODO: add concurrency test
