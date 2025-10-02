package jsonrpc

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodeBatchRequest(t *testing.T) {
	t.Run("Valid batch with multiple requests", func(t *testing.T) {
		data := []byte(`[
			{"jsonrpc":"2.0","id":1,"method":"sum","params":[1,2]},
			{"jsonrpc":"2.0","id":2,"method":"subtract","params":[5,3]}
		]`)
		reqs, err := DecodeBatchRequest(data)
		require.NoError(t, err)
		assert.Len(t, reqs, 2)
		assert.Equal(t, "sum", reqs[0].Method)
		assert.Equal(t, "subtract", reqs[1].Method)
		assert.Equal(t, int64(1), reqs[0].ID)
		assert.Equal(t, int64(2), reqs[1].ID)
	})

	t.Run("Empty batch returns error", func(t *testing.T) {
		data := []byte(`[]`)
		_, err := DecodeBatchRequest(data)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "at least one")
	})

	t.Run("Batch with notification (no ID)", func(t *testing.T) {
		data := []byte(`[
			{"jsonrpc":"2.0","method":"notify"},
			{"jsonrpc":"2.0","id":1,"method":"sum","params":[1,2]}
		]`)
		reqs, err := DecodeBatchRequest(data)
		require.NoError(t, err)
		assert.Len(t, reqs, 2)
		assert.True(t, reqs[0].IsNotification())
		assert.False(t, reqs[1].IsNotification())
	})

	t.Run("Batch with one invalid request", func(t *testing.T) {
		data := []byte(`[
			{"jsonrpc":"2.0","id":1,"method":"sum"},
			{"jsonrpc":"1.0","id":2,"method":"bad"}
		]`)
		_, err := DecodeBatchRequest(data)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "index 1")
	})

	t.Run("Not a JSON array", func(t *testing.T) {
		data := []byte(`{"jsonrpc":"2.0","id":1,"method":"sum"}`)
		_, err := DecodeBatchRequest(data)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid batch format")
	})

	t.Run("Empty data", func(t *testing.T) {
		data := []byte(``)
		_, err := DecodeBatchRequest(data)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "empty data")
	})

	t.Run("Batch with mixed ID types", func(t *testing.T) {
		data := []byte(`[
			{"jsonrpc":"2.0","id":"abc","method":"method1"},
			{"jsonrpc":"2.0","id":123,"method":"method2"},
			{"jsonrpc":"2.0","id":45.67,"method":"method3"}
		]`)
		reqs, err := DecodeBatchRequest(data)
		require.NoError(t, err)
		assert.Len(t, reqs, 3)
		assert.Equal(t, "abc", reqs[0].ID)
		assert.Equal(t, int64(123), reqs[1].ID)
		assert.Equal(t, 45.67, reqs[2].ID)
	})
}

func TestEncodeBatchRequest(t *testing.T) {
	t.Run("Valid batch encoding", func(t *testing.T) {
		reqs := []*Request{
			NewRequest("sum", []any{1, 2}),
			NewRequest("subtract", []any{5, 3}),
		}
		data, err := EncodeBatchRequest(reqs)
		require.NoError(t, err)
		assert.True(t, bytes.HasPrefix(bytes.TrimSpace(data), []byte("[")))
		assert.True(t, bytes.HasSuffix(bytes.TrimSpace(data), []byte("]")))

		// Verify it can be decoded back
		decoded, err := DecodeBatchRequest(data)
		require.NoError(t, err)
		assert.Len(t, decoded, 2)
	})

	t.Run("Empty input returns error", func(t *testing.T) {
		_, err := EncodeBatchRequest([]*Request{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "at least one")
	})

	t.Run("Invalid request in batch", func(t *testing.T) {
		reqs := []*Request{
			{JSONRPC: "2.0", Method: "valid"},
			{JSONRPC: "1.0", Method: "invalid"}, // Invalid version
		}
		_, err := EncodeBatchRequest(reqs)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "index 1")
	})

	t.Run("Nil request in batch", func(t *testing.T) {
		reqs := []*Request{
			NewRequest("sum", []int{1, 2}),
			nil,
		}
		_, err := EncodeBatchRequest(reqs)
		require.Error(t, err)
	})
}

func TestDecodeBatchResponse(t *testing.T) {
	t.Run("Valid batch with multiple responses", func(t *testing.T) {
		data := []byte(`[
			{"jsonrpc":"2.0","id":1,"result":3},
			{"jsonrpc":"2.0","id":2,"result":2}
		]`)
		resps, err := DecodeBatchResponse(data)
		require.NoError(t, err)
		assert.Len(t, resps, 2)

		var result1, result2 int
		require.NoError(t, resps[0].UnmarshalResult(&result1))
		require.NoError(t, resps[1].UnmarshalResult(&result2))
		assert.Equal(t, 3, result1)
		assert.Equal(t, 2, result2)
	})

	t.Run("Empty batch returns error", func(t *testing.T) {
		data := []byte(`[]`)
		_, err := DecodeBatchResponse(data)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "at least one")
	})

	t.Run("Batch with error response", func(t *testing.T) {
		data := []byte(`[
			{"jsonrpc":"2.0","id":1,"result":3},
			{"jsonrpc":"2.0","id":2,"error":{"code":-32601,"message":"Method not found"}}
		]`)
		resps, err := DecodeBatchResponse(data)
		require.NoError(t, err)
		assert.Len(t, resps, 2)
		assert.Nil(t, resps[0].Err())

		// Unmarshal the error for the second response
		require.NoError(t, resps[1].UnmarshalError())
		assert.NotNil(t, resps[1].Err())
		assert.Equal(t, MethodNotFound, resps[1].Err().Code)
	})

	t.Run("Batch with one invalid response", func(t *testing.T) {
		data := []byte(`[
			{"jsonrpc":"2.0","id":1,"result":3},
			{"jsonrpc":"1.0","id":2,"result":2}
		]`)
		_, err := DecodeBatchResponse(data)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "index 1")
	})

	t.Run("Not a JSON array", func(t *testing.T) {
		data := []byte(`{"jsonrpc":"2.0","id":1,"result":3}`)
		_, err := DecodeBatchResponse(data)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid batch format")
	})

	t.Run("Empty data", func(t *testing.T) {
		data := []byte(``)
		_, err := DecodeBatchResponse(data)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "empty data")
	})
}

func TestEncodeBatchResponse(t *testing.T) {
	t.Run("Valid batch encoding", func(t *testing.T) {
		resp1, err := NewResponse(int64(1), 3)
		require.NoError(t, err)
		resp2, err := NewResponse(int64(2), 2)
		require.NoError(t, err)

		resps := []*Response{resp1, resp2}
		data, err := EncodeBatchResponse(resps)
		require.NoError(t, err)
		assert.True(t, bytes.HasPrefix(bytes.TrimSpace(data), []byte("[")))
		assert.True(t, bytes.HasSuffix(bytes.TrimSpace(data), []byte("]")))

		// Verify it can be decoded back
		decoded, err := DecodeBatchResponse(data)
		require.NoError(t, err)
		assert.Len(t, decoded, 2)
	})

	t.Run("Empty input returns error", func(t *testing.T) {
		_, err := EncodeBatchResponse([]*Response{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "at least one")
	})

	t.Run("Invalid response in batch", func(t *testing.T) {
		validResp, err := NewResponse(int64(1), 3)
		require.NoError(t, err)

		resps := []*Response{
			validResp,
			{jsonrpc: "1.0", id: int64(2)}, // Invalid version
		}
		_, err = EncodeBatchResponse(resps)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "index 1")
	})

	t.Run("Nil response in batch", func(t *testing.T) {
		validResp, err := NewResponse(int64(1), 3)
		require.NoError(t, err)

		resps := []*Response{
			validResp,
			nil,
		}
		_, err = EncodeBatchResponse(resps)
		require.Error(t, err)
	})
}

func TestDecodeRequestOrBatch(t *testing.T) {
	t.Run("Single request", func(t *testing.T) {
		data := []byte(`{"jsonrpc":"2.0","id":1,"method":"sum","params":[1,2]}`)
		reqs, isBatch, err := DecodeRequestOrBatch(data)
		require.NoError(t, err)
		assert.False(t, isBatch)
		assert.Len(t, reqs, 1)
		assert.Equal(t, "sum", reqs[0].Method)
	})

	t.Run("Batch request", func(t *testing.T) {
		data := []byte(`[
			{"jsonrpc":"2.0","id":1,"method":"sum","params":[1,2]},
			{"jsonrpc":"2.0","id":2,"method":"subtract","params":[5,3]}
		]`)
		reqs, isBatch, err := DecodeRequestOrBatch(data)
		require.NoError(t, err)
		assert.True(t, isBatch)
		assert.Len(t, reqs, 2)
	})

	t.Run("Empty data", func(t *testing.T) {
		data := []byte(``)
		_, _, err := DecodeRequestOrBatch(data)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "empty data")
	})
}

func TestDecodeResponseOrBatch(t *testing.T) {
	t.Run("Single response", func(t *testing.T) {
		data := []byte(`{"jsonrpc":"2.0","id":1,"result":3}`)
		resps, isBatch, err := DecodeResponseOrBatch(data)
		require.NoError(t, err)
		assert.False(t, isBatch)
		assert.Len(t, resps, 1)
	})

	t.Run("Batch response", func(t *testing.T) {
		data := []byte(`[
			{"jsonrpc":"2.0","id":1,"result":3},
			{"jsonrpc":"2.0","id":2,"result":2}
		]`)
		resps, isBatch, err := DecodeResponseOrBatch(data)
		require.NoError(t, err)
		assert.True(t, isBatch)
		assert.Len(t, resps, 2)
	})

	t.Run("Empty data", func(t *testing.T) {
		data := []byte(``)
		_, _, err := DecodeResponseOrBatch(data)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "empty data")
	})
}

func TestNewBatchRequest(t *testing.T) {
	t.Run("Valid batch with params", func(t *testing.T) {
		methods := []string{"sum", "subtract"}
		params := []any{[]any{1, 2}, []any{5, 3}}
		reqs, err := NewBatchRequest(methods, params)
		require.NoError(t, err)
		assert.Len(t, reqs, 2)
		assert.Equal(t, "sum", reqs[0].Method)
		assert.Equal(t, "subtract", reqs[1].Method)
		assert.NotNil(t, reqs[0].ID)
		assert.NotNil(t, reqs[1].ID)
	})

	t.Run("Valid batch without params", func(t *testing.T) {
		methods := []string{"method1", "method2"}
		reqs, err := NewBatchRequest(methods, nil)
		require.NoError(t, err)
		assert.Len(t, reqs, 2)
		assert.Nil(t, reqs[0].Params)
		assert.Nil(t, reqs[1].Params)
	})

	t.Run("Empty methods returns error", func(t *testing.T) {
		_, err := NewBatchRequest([]string{}, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "at least one method")
	})

	t.Run("Mismatched params length returns error", func(t *testing.T) {
		methods := []string{"sum", "subtract"}
		params := []any{[]any{1, 2}}
		_, err := NewBatchRequest(methods, params)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must match")
	})
}

func TestNewBatchNotification(t *testing.T) {
	t.Run("Valid batch notifications", func(t *testing.T) {
		methods := []string{"log", "notify"}
		params := []any{"message 1", "message 2"}
		reqs, err := NewBatchNotification(methods, params)
		require.NoError(t, err)
		assert.Len(t, reqs, 2)
		assert.True(t, reqs[0].IsNotification())
		assert.True(t, reqs[1].IsNotification())
		assert.Equal(t, "log", reqs[0].Method)
		assert.Equal(t, "notify", reqs[1].Method)
	})

	t.Run("Empty methods returns error", func(t *testing.T) {
		_, err := NewBatchNotification([]string{}, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "at least one method")
	})
}

func TestDecodeBatchRequestFromReader(t *testing.T) {
	t.Run("Valid batch from reader", func(t *testing.T) {
		data := `[
			{"jsonrpc":"2.0","id":1,"method":"sum","params":[1,2]},
			{"jsonrpc":"2.0","id":2,"method":"subtract","params":[5,3]}
		]`
		reader := strings.NewReader(data)
		reqs, err := DecodeBatchRequestFromReader(reader, len(data))
		require.NoError(t, err)
		assert.Len(t, reqs, 2)
	})

	t.Run("Nil reader returns error", func(t *testing.T) {
		_, err := DecodeBatchRequestFromReader(nil, 0)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "nil reader")
	})
}

func TestDecodeBatchResponseFromReader(t *testing.T) {
	t.Run("Valid batch from reader", func(t *testing.T) {
		data := `[
			{"jsonrpc":"2.0","id":1,"result":3},
			{"jsonrpc":"2.0","id":2,"result":2}
		]`
		reader := strings.NewReader(data)
		resps, err := DecodeBatchResponseFromReader(reader, len(data))
		require.NoError(t, err)
		assert.Len(t, resps, 2)
	})

	t.Run("Nil reader returns error", func(t *testing.T) {
		_, err := DecodeBatchResponseFromReader(nil, 0)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "nil reader")
	})
}

func TestBatchNotifications(t *testing.T) {
	t.Run("All-notification batch", func(t *testing.T) {
		data := []byte(`[
			{"jsonrpc":"2.0","method":"notify1"},
			{"jsonrpc":"2.0","method":"notify2"},
			{"jsonrpc":"2.0","method":"notify3"}
		]`)
		reqs, err := DecodeBatchRequest(data)
		require.NoError(t, err)
		assert.Len(t, reqs, 3)
		for _, req := range reqs {
			assert.True(t, req.IsNotification())
		}
	})
}

func TestBatchWithMixedIDTypes(t *testing.T) {
	t.Run("Batch with string, int, and float IDs", func(t *testing.T) {
		data := []byte(`[
			{"jsonrpc":"2.0","id":"string-id","method":"method1"},
			{"jsonrpc":"2.0","id":42,"method":"method2"},
			{"jsonrpc":"2.0","id":3.14,"method":"method3"}
		]`)
		reqs, err := DecodeBatchRequest(data)
		require.NoError(t, err)
		assert.Len(t, reqs, 3)
		assert.Equal(t, "string-id", reqs[0].ID)
		assert.Equal(t, int64(42), reqs[1].ID)
		assert.Equal(t, 3.14, reqs[2].ID)
	})
}

func TestLargeBatch(t *testing.T) {
	t.Run("Large batch with 1000+ requests", func(t *testing.T) {
		// Create a large batch
		methods := make([]string, 1000)
		params := make([]any, 1000)
		for i := 0; i < 1000; i++ {
			methods[i] = "method"
			params[i] = []any{i}
		}

		reqs, err := NewBatchRequest(methods, params)
		require.NoError(t, err)
		assert.Len(t, reqs, 1000)

		// Encode and decode
		data, err := EncodeBatchRequest(reqs)
		require.NoError(t, err)

		decoded, err := DecodeBatchRequest(data)
		require.NoError(t, err)
		assert.Len(t, decoded, 1000)
	})
}

func TestBatchWithDuplicateIDs(t *testing.T) {
	t.Run("Batch with duplicate IDs (allowed but questionable)", func(t *testing.T) {
		data := []byte(`[
			{"jsonrpc":"2.0","id":1,"method":"method1"},
			{"jsonrpc":"2.0","id":1,"method":"method2"}
		]`)
		reqs, err := DecodeBatchRequest(data)
		require.NoError(t, err)
		assert.Len(t, reqs, 2)
		assert.Equal(t, reqs[0].ID, reqs[1].ID)
	})
}

func TestBatchWithDeeplyNestedParams(t *testing.T) {
	t.Run("Batch with deeply nested params", func(t *testing.T) {
		data := []byte(`[
			{
				"jsonrpc":"2.0",
				"id":1,
				"method":"complex",
				"params":{
					"level1":{
						"level2":{
							"level3":{
								"value":"deep"
							}
						}
					}
				}
			}
		]`)
		reqs, err := DecodeBatchRequest(data)
		require.NoError(t, err)
		assert.Len(t, reqs, 1)
		assert.NotNil(t, reqs[0].Params)
	})
}

func TestBatchRoundTrip(t *testing.T) {
	t.Run("Request batch round-trip", func(t *testing.T) {
		original := []*Request{
			NewRequest("sum", []any{1, 2}),
			NewNotification("notify", map[string]any{"message": "test"}),
			NewRequestWithID("custom", map[string]any{"key": "value"}, "custom-id"),
		}

		encoded, err := EncodeBatchRequest(original)
		require.NoError(t, err)

		decoded, err := DecodeBatchRequest(encoded)
		require.NoError(t, err)
		assert.Len(t, decoded, 3)
		assert.Equal(t, original[0].Method, decoded[0].Method)
		assert.True(t, decoded[1].IsNotification())
		assert.Equal(t, "custom-id", decoded[2].ID)
	})

	t.Run("Response batch round-trip", func(t *testing.T) {
		resp1, _ := NewResponse(int64(1), 42)
		resp2 := NewErrorResponse(int64(2), &Error{Code: InvalidRequest, Message: "test error"})

		original := []*Response{resp1, resp2}

		encoded, err := EncodeBatchResponse(original)
		require.NoError(t, err)

		decoded, err := DecodeBatchResponse(encoded)
		require.NoError(t, err)
		assert.Len(t, decoded, 2)

		var result int
		require.NoError(t, decoded[0].UnmarshalResult(&result))
		assert.Equal(t, 42, result)

		// Unmarshal the error
		require.NoError(t, decoded[1].UnmarshalError())
		assert.NotNil(t, decoded[1].Err())
		assert.Equal(t, InvalidRequest, decoded[1].Err().Code)
	})
}
