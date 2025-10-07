// Package jsonrpc provides a Go implementation of the JSON-RPC 2.0 specification, as well as tools
// to parse and work with JSON-RPC requests and responses.
package jsonrpc

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequest_IDString(t *testing.T) {
	t.Run("String ID", func(t *testing.T) {
		req := &Request{ID: "abc"}
		assert.Equal(t, "abc", req.IDString())
	})

	t.Run("Int64 ID", func(t *testing.T) {
		req := &Request{ID: int64(123)}
		assert.Equal(t, "123", req.IDString())
	})

	t.Run("Float64 ID", func(t *testing.T) {
		req := &Request{ID: float64(123.456)}
		assert.Equal(t, "123.456", req.IDString())
	})

	t.Run("Float64 ID, with integer value", func(t *testing.T) {
		resp := &Request{ID: float64(25.0)}
		assert.Equal(t, "25.0", resp.IDString())
	})
	t.Run("Nil ID", func(t *testing.T) {
		req := &Request{ID: nil}
		assert.Equal(t, "", req.IDString())
	})

	t.Run("Unknown type ID", func(t *testing.T) {
		req := &Request{ID: []int{1, 2, 3}}
		assert.Equal(t, "", req.IDString())
	})
}

func TestRequest_IsEmpty(t *testing.T) {
	t.Run("Nil receiver => true", func(t *testing.T) {
		var req *Request
		assert.True(t, req.IsEmpty())
	})

	t.Run("Empty => true", func(t *testing.T) {
		req := &Request{}
		assert.True(t, req.IsEmpty())
	})

	t.Run("Empty method => true", func(t *testing.T) {
		req := &Request{Method: ""}
		assert.True(t, req.IsEmpty())
	})

	t.Run("Non-empty method => false", func(t *testing.T) {
		req := &Request{Method: "testMethod"}
		assert.False(t, req.IsEmpty())
	})
}

func TestRequest_MarshalJSON(t *testing.T) {
	t.Run("Valid request", func(t *testing.T) {
		cases := []struct {
			name     string
			req      *Request
			expected string
		}{
			{
				name: "With int ID",
				req: &Request{JSONRPC: "2.0", Method: "testMethod",
					Params: []any{"0x123"}, ID: int64(99)},
				expected: `{"jsonrpc":"2.0","id":99,"method":"testMethod","params":["0x123"]}`,
			},
			{
				name: "With string ID",
				req: &Request{JSONRPC: "2.0", Method: "eth_getBalance",
					Params: []any{}, ID: "abc"},
				expected: `{"jsonrpc":"2.0","id":"abc","method":"eth_getBalance","params":[]}`,
			},
			{
				name:     "With nil Params",
				req:      &Request{JSONRPC: "2.0", Method: "eth_chainId", ID: "abc"},
				expected: `{"jsonrpc":"2.0","id":"abc","method":"eth_chainId"}`,
			},
			{
				name: "With empty Params array",
				req: &Request{JSONRPC: "2.0", Method: "eth_chainId",
					Params: []any{}, ID: "abc"},
				expected: `{"jsonrpc":"2.0","id":"abc","method":"eth_chainId","params":[]}`,
			},
			{
				name: "With object Params",
				req: &Request{JSONRPC: "2.0", Method: "eth_getBalance",
					Params: map[string]any{"address": "0x123"}, ID: "abc"},
				expected: `{"jsonrpc":"2.0","id":"abc","method":"eth_getBalance",` +
					`"params":{"address":"0x123"}}`,
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				data, err := tc.req.MarshalJSON()
				require.NoError(t, err)
				assert.JSONEq(t, tc.expected, string(data))
			})
		}
	})

	t.Run("Invalid request", func(t *testing.T) {
		cases := []struct {
			name string
			req  *Request
		}{
			{
				name: "Nil receiver",
				req:  nil,
			},
			{
				name: "Empty method",
				req:  &Request{Method: ""},
			},
			{
				name: "Empty JSONRPC",
				req:  &Request{JSONRPC: ""},
			},
			{
				name: "Wrong JSONRPC version",
				req:  &Request{JSONRPC: "1.0", Method: "testMethod"},
			},
			{
				name: "Invalid ID type",
				req:  &Request{JSONRPC: "2.0", Method: "testMethod", ID: []int{1, 2, 3}},
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				_, err := tc.req.MarshalJSON()
				assert.Error(t, err, "should fail to marshal invalid request")
			})
		}
	})
}

func TestRequest_String(t *testing.T) {
	t.Run("With int ID", func(t *testing.T) {
		req := &Request{JSONRPC: "2.0", Method: "testMethod", Params: []any{"0x123"}, ID: int64(99)}
		expected := "ID: 99, Method: testMethod"
		assert.Equal(t, expected, req.String())
	})

	t.Run("With string ID", func(t *testing.T) {
		req := &Request{JSONRPC: "2.0", Method: "eth_getBalance", Params: []any{}, ID: "abc"}
		expected := "ID: abc, Method: eth_getBalance"
		assert.Equal(t, expected, req.String())
	})

	t.Run("With nil ID", func(t *testing.T) {
		req := &Request{JSONRPC: "2.0", Method: "eth_chainId", ID: nil}
		expected := "ID: <nil>, Method: eth_chainId"
		assert.Equal(t, expected, req.String())
	})

	t.Run("With float ID", func(t *testing.T) {
		req := &Request{JSONRPC: "2.0", Method: "testMethod", ID: float64(123.456)}
		expected := "ID: 123.456, Method: testMethod"
		assert.Equal(t, expected, req.String())
	})

	t.Run("With empty Method", func(t *testing.T) {
		req := &Request{JSONRPC: "2.0", Method: "", ID: "abc"}
		expected := "ID: abc, Method: "
		assert.Equal(t, expected, req.String())
	})
}

func TestRequest_UnmarshalJSON(t *testing.T) {
	t.Run("Valid JSON with int ID", func(t *testing.T) {
		data := []byte(`{"jsonrpc":"2.0","method":"test","params":["0x123"],"id":99}`)
		expected := Request{JSONRPC: "2.0", Method: "test", Params: []any{"0x123"}, ID: int64(99)}

		var result Request
		err := result.UnmarshalJSON(data)
		assert.NoError(t, err, "Unexpected error")
		assert.Equal(t, expected.JSONRPC, result.JSONRPC)
		assert.Equal(t, expected.Method, result.Method)
		assert.Equal(t, expected.Params, result.Params)
		assert.Equal(t, expected.ID, result.ID)
		assert.IsType(t, int64(0), result.ID)
	})

	t.Run("Valid JSON with float ID", func(t *testing.T) {
		data := []byte(`{"jsonrpc":"2.0","method":"test","id":33.3}`)
		expected := Request{JSONRPC: "2.0", Method: "test", ID: float64(33.3)}

		var result Request
		err := result.UnmarshalJSON(data)
		assert.NoError(t, err, "Unexpected error")
		assert.Equal(t, expected.JSONRPC, result.JSONRPC)
		assert.Equal(t, expected.Method, result.Method)
		assert.Equal(t, expected.ID, result.ID)
		assert.IsType(t, float64(0), result.ID)
	})

	t.Run("Valid JSON with string ID", func(t *testing.T) {
		data := []byte(`{"jsonrpc":"2.0","method":"eth_getBalance","params":[],"id":"abc"}`)
		expected := Request{JSONRPC: "2.0", Method: "eth_getBalance", ID: "abc"}

		var result Request
		err := result.UnmarshalJSON(data)
		assert.NoError(t, err, "Unexpected error")
		assert.Equal(t, expected.JSONRPC, result.JSONRPC)
		assert.Equal(t, expected.Method, result.Method)
		assert.Empty(t, result.Params)
		assert.IsType(t, "", result.ID)
		assert.Equal(t, expected.ID, result.ID)
	})

	t.Run("Valid JSON with extra field", func(t *testing.T) {
		data := []byte(`{"jsonrpc":"2.0","method":"test","id":32123,"something":"extra"}`)
		expected := Request{JSONRPC: "2.0", Method: "test", ID: int64(32123)}

		var result Request
		err := result.UnmarshalJSON(data)
		assert.NoError(t, err, "Unexpected error")
		assert.Equal(t, expected.JSONRPC, result.JSONRPC)
		assert.Equal(t, expected.Method, result.Method)
		assert.Equal(t, expected.Params, result.Params)
		assert.Equal(t, expected.ID, result.ID)
		assert.IsType(t, int64(0), result.ID)
	})

	t.Run("Empty string ID => replaced with nil", func(t *testing.T) {
		data := []byte(`{"jsonrpc":"2.0","id":"","method":"eth_chainId"}`)
		var req Request
		err := req.UnmarshalJSON(data)
		require.NoError(t, err)
		assert.Nil(t, req.ID)
		// If empty string, ID should be nil
		_, ok := req.ID.(string)
		assert.False(t, ok)
		_, ok = req.ID.(int64)
		assert.False(t, ok)
		_, ok = req.ID.(float64)
		assert.False(t, ok)
		assert.Equal(t, "eth_chainId", req.Method)
	})

	t.Run("Invalid JSONRPC => error", func(t *testing.T) {
		invalidJSONs := [][]byte{
			// Invalid JSONRPC field
			[]byte(`{"json":"2.0","id":1,"method":"eth_chainId"`),
			// Invalid ID: missing value
			[]byte(`{"jsonrpc":"2.0","id":,"method":"eth_chainId"}`),
			// Invalid ID: boolean
			[]byte(`{"jsonrpc":"2.0","id":true,"method":"eth_chainId"}`),
			// Invalid ID: object
			[]byte(`{"jsonrpc":"2.0","id":{},"method":"eth_chainId"}`),
			// Invalid ID: array
			[]byte(`{"jsonrpc":"2.0","id":[],"method":"eth_chainId"}`),
			// Invalid method: number
			[]byte(`{"jsonrpc":"2.0","id":1,"method":15`),
			// Invalid method: empty string
			[]byte(`{"jsonrpc":"2.0","id":1,"method":""}`),
			// Invalid method: object
			[]byte(`{"jsonrpc":"2.0","id":1,"method":{}}`),
			// Invalid method: array
			[]byte(`{"jsonrpc":"2.0","id":1,"method":[]}`),
			// Invalid method: boolean
			[]byte(`{"jsonrpc":"2.0","id":1,"method":true}`),
			// Invalid params: nested invalid JSON
			[]byte(`{"jsonrpc":"2.0","id":1,"method":"eth_chainId","params":[{"nested":}]}`),
			// Invalid params: simple string
			[]byte(`{"jsonrpc":"2.0","id":1,"method":"eth_chainId","params":"not_array"}`),
			// Invalid params: number
			[]byte(`{"jsonrpc":"2.0","id":1,"method":"eth_chainId","params":15}`),
			// Invalid params: missing value
			[]byte(`{"jsonrpc":"2.0","id":1,"method":"eth_chainId","params":}`),
			// Invalid params: boolean
			[]byte(`{"jsonrpc":"2.0","id":1,"method":"eth_chainId","params":true}`),
			// Missing method
			[]byte(`{"jsonrpc":"2.0","id":1,"params":[]}`),
			// Missing method field + params
			[]byte(`{"jsonrpc":"2.0","id":1}`),
			// Invalid JSONRPC request, but valid JSON
			[]byte(`{"0x123": "abs"}`),
			// Missing closing bracket
			[]byte(`{"jsonrpc":"2.0","id":1,"method":"eth_chainId","params":[]`),
			[]byte(`{}`), // Empty JSON object
			[]byte(``),   // Empty string
		}

		for _, data := range invalidJSONs {
			var req Request
			err := req.UnmarshalJSON(data)
			assert.Error(t, err, "should fail to unmarshal invalid JSON: %s", data)
		}
	})

	t.Run("Valid JSONRPC => no error", func(t *testing.T) {
		validJSONs := [][]byte{
			// string id
			[]byte(`{"jsonrpc":"2.0","id":"one","method":"eth_chainId"}`),
			// float id
			[]byte(`{"jsonrpc":"2.0","id":1.1,"method":"eth_chainId"}`),
			// int id
			[]byte(`{"jsonrpc":"2.0","id":25,"method":"eth_chainId"}`),
			// null id
			[]byte(`{"jsonrpc":"2.0","id":null,"method":"eth_chainId"}`),
			// No ID (notification)
			[]byte(`{"jsonrpc":"2.0","method":"eth_chainId","params":[]}`),
			// Extra field
			[]byte(`{"jsonrpc":"2.0","id":1,"method":"eth_chainId","extra":"field"}`),
			// Empty list params
			[]byte(`{"jsonrpc":"2.0","id":2,"method":"eth_blockNumber","params":[]}`),
			// Empty object params
			[]byte(`{"jsonrpc":"2.0","id":1,"method":"eth_chainId","params":{}}`),
			// Empty string params
			[]byte(`{"jsonrpc":"2.0","id":1,"method":"eth_chainId","params":""}`),
			// Object params
			[]byte(`{"jsonrpc":"2.0","id":1,"method":"eth_chainId","params":{"key": "value"}}`),
			// Multiple params
			[]byte(`{"jsonrpc":"2.0","id":3,"method":"eth_getBalance",` +
				`"params":["0x123456", "latest"]}`),
			// Nested params
			[]byte(`{"jsonrpc":"2.0","id":1,"method":"eth_getBalance",` +
				`"params":[{"address": "0x123", "block": "latest"}]}`),
			// Missing params
			[]byte(`{"jsonrpc":"2.0","id":1,"method":"eth_chainId"}`),
			// Null params
			[]byte(`{"jsonrpc":"2.0","id":1,"method":"eth_chainId","params":null}`),
		}

		for _, data := range validJSONs {
			var req Request
			err := req.UnmarshalJSON(data)
			assert.NoError(t, err, "should successfully unmarshal valid JSON: %s", data)
		}
	})
}

func TestRequestFromBytes(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		data := []byte(`{"jsonrpc":"2.0","id":1,"method":"testMethod","params":["0x123"]}`)
		req, err := DecodeRequest(data)
		require.NoError(t, err)
		require.NotNil(t, req)
		assert.Equal(t, "testMethod", req.Method)
		assert.EqualValues(t, 1, req.ID)
		assert.Equal(t, []any{"0x123"}, req.Params)
	})

	t.Run("Unmarshal error", func(t *testing.T) {
		// Invalid JSON, ID is empty
		data := []byte(`{"jsonrpc":"2.0","id":,"method":"testMethod"}`)
		req, err := DecodeRequest(data)
		require.Error(t, err)
		require.Nil(t, req)
	})

	t.Run("Empty input data", func(t *testing.T) {
		req, err := DecodeRequest([]byte{})
		require.Error(t, err)
		require.Nil(t, req)
	})
}

func TestRequest_Concurrency(t *testing.T) {
	data := []byte(`{"jsonrpc":"2.0","id":1,"method":"testMethod","params":["0x123"]}`)
	req, err := DecodeRequest(data)
	require.NoError(t, err)
	require.NotNil(t, req)

	var wg sync.WaitGroup
	wg.Add(2)

	testFunc := func() {
		defer wg.Done()
		for range 2000 {
			_ = req.IDString()
			_ = req.Method
			_ = req.Params
			assert.False(t, req.IsEmpty())

			marshaledData, err := req.MarshalJSON()
			require.NoError(t, err)
			assert.JSONEq(t, string(data), string(marshaledData))
		}
	}

	go testFunc()
	go testFunc()

	wg.Wait()
}
