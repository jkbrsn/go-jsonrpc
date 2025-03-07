package jsonrpc

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
	t.Run("Valid request with int ID", func(t *testing.T) {
		req := &Request{JSONRPC: "2.0", Method: "testMethod", Params: []any{"0x123"}, ID: int64(99)}
		expected := `{"jsonrpc":"2.0","id":99,"method":"testMethod","params":["0x123"]}`
		data, err := req.MarshalJSON()
		assert.NoError(t, err)
		assert.JSONEq(t, expected, string(data))
	})

	t.Run("Valid request with string ID", func(t *testing.T) {
		req := &Request{JSONRPC: "2.0", Method: "eth_getBalance", Params: []any{}, ID: "abc"}
		expected := `{"jsonrpc":"2.0","id":"abc","method":"eth_getBalance","params":[]}`
		data, err := req.MarshalJSON()
		assert.NoError(t, err)
		assert.JSONEq(t, expected, string(data))
	})

	t.Run("Valid request with nil Params", func(t *testing.T) {
		req := &Request{JSONRPC: "2.0", Method: "eth_chainId", ID: "abc"}
		expected := `{"jsonrpc":"2.0","id":"abc","method":"eth_chainId"}`
		data, err := req.MarshalJSON()
		assert.NoError(t, err)
		assert.JSONEq(t, expected, string(data))
	})

	t.Run("Valid request with empty Params", func(t *testing.T) {
		req := &Request{JSONRPC: "2.0", Method: "eth_chainId", Params: []any{}, ID: "abc"}
		expected := `{"jsonrpc":"2.0","id":"abc","method":"eth_chainId","params":[]}`
		data, err := req.MarshalJSON()
		assert.NoError(t, err)
		assert.JSONEq(t, expected, string(data))
	})

	t.Run("Valid request with object Params", func(t *testing.T) {
		req := &Request{JSONRPC: "2.0", Method: "eth_getBalance", Params: map[string]any{"address": "0x123"}, ID: "abc"}
		expected := `{"jsonrpc":"2.0","id":"abc","method":"eth_getBalance","params":{"address":"0x123"}}`
		data, err := req.MarshalJSON()
		assert.NoError(t, err)
		assert.JSONEq(t, expected, string(data))
	})

	t.Run("Empty request", func(t *testing.T) {
		req := &Request{}
		expected := `{}`
		data, err := req.MarshalJSON()
		assert.NoError(t, err)
		assert.JSONEq(t, expected, string(data))
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
		assert.Equal(t, expected.ID, result.ID)
	})

	t.Run("No ID => random assigned", func(t *testing.T) {
		data := []byte(`{"jsonrpc":"2.0","method":"eth_gasPrice"}`)
		var req Request
		err := req.UnmarshalJSON(data)
		require.NoError(t, err)
		assert.NotNil(t, req.ID)
		// We can't predict the random ID, but ensure it's not empty
		assert.NotEqual(t, "", req.ID)
		assert.Equal(t, "eth_gasPrice", req.Method)
	})

	t.Run("Empty string ID => replaced with random", func(t *testing.T) {
		data := []byte(`{"jsonrpc":"2.0","id":"","method":"eth_chainId"}`)
		var req Request
		err := req.UnmarshalJSON(data)
		require.NoError(t, err)
		assert.NotNil(t, req.ID)
		// If empty string, is still replaced with random int ID
		_, ok := req.ID.(string)
		assert.False(t, ok)
		_, ok = req.ID.(int64)
		assert.True(t, ok)
		assert.Equal(t, "eth_chainId", req.Method)
	})

	t.Run("Invalid JSONRPC => error", func(t *testing.T) {
		invalidJSONs := [][]byte{
			[]byte(`{"json":"2.0","id":1,"method":"eth_chainId"`),                            // Invalid JSONRPC field
			[]byte(`{"jsonrpc":"2.0","id":,"method":"eth_chainId"}`),                         // Invalid ID: missing value
			[]byte(`{"jsonrpc":"2.0","id":true,"method":"eth_chainId"}`),                     // Invalid ID: boolean
			[]byte(`{"jsonrpc":"2.0","id":{},"method":"eth_chainId"}`),                       // Invalid ID: object
			[]byte(`{"jsonrpc":"2.0","id":[],"method":"eth_chainId"}`),                       // Invalid ID: array
			[]byte(`{"jsonrpc":"2.0","id":1,"method":15`),                                    // Invalid method: number
			[]byte(`{"jsonrpc":"2.0","id":1,"method":""}`),                                   // Invalid method: empty string
			[]byte(`{"jsonrpc":"2.0","id":1,"method":{}}`),                                   // Invalid method: object
			[]byte(`{"jsonrpc":"2.0","id":1,"method":[]}`),                                   // Invalid method: array
			[]byte(`{"jsonrpc":"2.0","id":1,"method":true}`),                                 // Invalid method: boolean
			[]byte(`{"jsonrpc":"2.0","id":1,"method":"eth_chainId","params":[{"nested":}]}`), // Invalid params: nested invalid JSON
			[]byte(`{"jsonrpc":"2.0","id":1,"method":"eth_chainId","params":"not_array"}`),   // Invalid params: simple string
			[]byte(`{"jsonrpc":"2.0","id":1,"method":"eth_chainId","params":15}`),            // Invalid params: number
			[]byte(`{"jsonrpc":"2.0","id":1,"method":"eth_chainId","params":}`),              // Invalid params: missing value
			[]byte(`{"jsonrpc":"2.0","id":1,"method":"eth_chainId","params":true}`),          // Invalid params: boolean
			[]byte(`{"jsonrpc":"2.0","id":1,"params":[]}`),                                   // Missing method
			[]byte(`{"jsonrpc":"2.0","id":1}`),                                               // Missing method field + params
			[]byte(`{"0x123": "abs"}`),                                                       // Invalid JSONRPC request, but valid JSON
			[]byte(`{"jsonrpc":"2.0","id":1,"method":"eth_chainId","params":[]`),             // Missing closing bracket
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
			[]byte(`{"jsonrpc":"2.0","id":"one","method":"eth_chainId"}`),                                                   // string id
			[]byte(`{"jsonrpc":"2.0","id":1.1,"method":"eth_chainId"}`),                                                     // float id
			[]byte(`{"jsonrpc":"2.0","id":25,"method":"eth_chainId"}`),                                                      // int id
			[]byte(`{"jsonrpc":"2.0","id":null,"method":"eth_chainId"}`),                                                    // null id
			[]byte(`{"jsonrpc":"2.0","method":"eth_chainId","params":[]}`),                                                  // No ID (notification)
			[]byte(`{"jsonrpc":"2.0","id":1,"method":"eth_chainId","extra":"field"}`),                                       // Extra field
			[]byte(`{"jsonrpc":"2.0","id":2,"method":"eth_blockNumber","params":[]}`),                                       // Empty list params
			[]byte(`{"jsonrpc":"2.0","id":1,"method":"eth_chainId","params":{}}`),                                           // Empty object params
			[]byte(`{"jsonrpc":"2.0","id":1,"method":"eth_chainId","params":{"key": "value"}}`),                             // Object params
			[]byte(`{"jsonrpc":"2.0","id":3,"method":"eth_getBalance","params":["0x123456", "latest"]}`),                    // Multiple params
			[]byte(`{"jsonrpc":"2.0","id":1,"method":"eth_getBalance","params":[{"address": "0x123", "block": "latest"}]}`), // Nested params
			[]byte(`{"jsonrpc":"2.0","id":1,"method":"eth_chainId"}`),                                                       // Missing params
			[]byte(`{"jsonrpc":"2.0","id":1,"method":"eth_chainId","params":null}`),                                         // Null params
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
		req, err := RequestFromBytes(data)
		require.NoError(t, err)
		require.NotNil(t, req)
		assert.Equal(t, "testMethod", req.Method)
		assert.EqualValues(t, 1, req.ID)
		assert.Equal(t, []any{"0x123"}, req.Params)
	})

	t.Run("Unmarshal error", func(t *testing.T) {
		// Invalid JSON, ID is empty
		data := []byte(`{"jsonrpc":"2.0","id":,"method":"testMethod"}`)
		req, err := RequestFromBytes(data)
		require.Error(t, err)
		require.Nil(t, req)
	})

	t.Run("Empty input data", func(t *testing.T) {
		req, err := RequestFromBytes([]byte{})
		require.Error(t, err)
		require.Nil(t, req)
	})
}

func TestRequest_Concurrency(t *testing.T) {
	data := []byte(`{"jsonrpc":"2.0","id":1,"method":"testMethod","params":["0x123"]}`)
	req, err := RequestFromBytes(data)
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
