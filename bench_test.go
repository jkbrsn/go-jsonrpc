// Package jsonrpc provides a Go implementation of the JSON-RPC 2.0 specification, as well as tools
// to parse and work with JSON-RPC requests and responses.
package jsonrpc

import (
	"bytes"
	"strings"
	"testing"
)

// Benchmark payloads of different sizes
var (
	smallRequestJSON  = []byte(`{"jsonrpc":"2.0","id":1,"method":"ping","params":null}`)
	mediumRequestJSON = []byte(`{"jsonrpc":"2.0","id":42,"method":"updateUser","params":{"userId":12345,"name":"Alice Johnson","email":"alice@example.com","preferences":{"theme":"dark","language":"en","notifications":true}}}`)
	largeRequestJSON  = []byte(`{"jsonrpc":"2.0","id":999,"method":"processData","params":{"items":[` + strings.Repeat(`{"id":1,"value":"data","metadata":{"key":"value"}},`, 100) + `{"id":101,"value":"final"}]}}`)

	smallResponseJSON  = []byte(`{"jsonrpc":"2.0","id":1,"result":"pong"}`)
	mediumResponseJSON = []byte(`{"jsonrpc":"2.0","id":42,"result":{"userId":12345,"name":"Alice Johnson","email":"alice@example.com","status":"active","lastLogin":"2024-01-15T10:30:00Z"}}`)
	largeResponseJSON  = []byte(`{"jsonrpc":"2.0","id":999,"result":{"items":[` + strings.Repeat(`{"id":1,"processed":true,"value":"result","timestamp":"2024-01-15T10:30:00Z"},`, 100) + `{"id":101,"processed":true}]}}`)

	errorResponseJSON = []byte(`{"jsonrpc":"2.0","id":1,"error":{"code":-32601,"message":"Method not found","data":{"method":"unknownMethod","available":["ping","echo","status"]}}}`)
)

// BenchmarkDecodeRequest benchmarks request decoding with different payload sizes
// TODO: Add comparison benchmarks for alternative JSON parsers (e.g., encoding/json, goccy/go-json)
// TODO: Add sub-benchmarks for different ID types (string, int, float) to measure ID parsing overhead
func BenchmarkDecodeRequest(b *testing.B) {
	b.Run("Small", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, err := DecodeRequest(smallRequestJSON)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("Medium", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, err := DecodeRequest(mediumRequestJSON)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("Large", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, err := DecodeRequest(largeRequestJSON)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkDecodeResponse benchmarks response decoding with different payload sizes and types
// TODO: Add comparison benchmarks for alternative JSON parsers
// TODO: Add benchmarks for responses with rawError lazy unmarshaling vs eager unmarshaling
func BenchmarkDecodeResponse(b *testing.B) {
	b.Run("Small_Result", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, err := DecodeResponse(smallResponseJSON)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("Medium_Result", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, err := DecodeResponse(mediumResponseJSON)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("Large_Result", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, err := DecodeResponse(largeResponseJSON)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("Error_Response", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, err := DecodeResponse(errorResponseJSON)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkDecodeBatchRequest benchmarks batch request decoding with varying batch sizes
// TODO: Add comparison benchmarks for alternative JSON parsers
// TODO: Add benchmarks for batches with mixed request types (requests + notifications)
func BenchmarkDecodeBatchRequest(b *testing.B) {
	batch1 := []byte(`[{"jsonrpc":"2.0","id":1,"method":"ping"}]`)
	batch10 := makeBatchRequestJSON(10)
	batch100 := makeBatchRequestJSON(100)

	b.Run("Batch_1", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, err := DecodeBatchRequest(batch1)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("Batch_10", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, err := DecodeBatchRequest(batch10)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("Batch_100", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, err := DecodeBatchRequest(batch100)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkDecodeBatchResponse benchmarks batch response decoding with varying batch sizes
// TODO: Add comparison benchmarks for alternative JSON parsers
// TODO: Add benchmarks for batches with mixed response types (results + errors)
func BenchmarkDecodeBatchResponse(b *testing.B) {
	batch1 := []byte(`[{"jsonrpc":"2.0","id":1,"result":"pong"}]`)
	batch10 := makeBatchResponseJSON(10)
	batch100 := makeBatchResponseJSON(100)

	b.Run("Batch_1", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, err := DecodeBatchResponse(batch1)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("Batch_10", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, err := DecodeBatchResponse(batch10)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("Batch_100", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, err := DecodeBatchResponse(batch100)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkRequestMarshal benchmarks request marshaling
// TODO: Add comparison benchmarks for alternative JSON parsers
// TODO: Add benchmarks for different param types (nil, array, object)
func BenchmarkRequestMarshal(b *testing.B) {
	req := &Request{
		JSONRPC: "2.0",
		ID:      int64(42),
		Method:  "updateUser",
		Params: map[string]any{
			"userId": 12345,
			"name":   "Alice Johnson",
			"email":  "alice@example.com",
		},
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := req.MarshalJSON()
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkResponseMarshal benchmarks response marshaling
// TODO: Add comparison benchmarks for alternative JSON parsers
// TODO: Add separate benchmarks for error responses vs result responses
func BenchmarkResponseMarshal(b *testing.B) {
	resp := &Response{
		jsonrpc: "2.0",
		id:      int64(42),
		result:  []byte(`{"userId":12345,"name":"Alice Johnson","status":"active"}`),
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := resp.MarshalJSON()
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkUnmarshalResult benchmarks lazy result unmarshaling
// TODO: Add comparison benchmarks for alternative JSON parsers
func BenchmarkUnmarshalResult(b *testing.B) {
	resp, _ := DecodeResponse(mediumResponseJSON)

	type Result struct {
		UserID    int    `json:"userId"`
		Name      string `json:"name"`
		Email     string `json:"email"`
		Status    string `json:"status"`
		LastLogin string `json:"lastLogin"`
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var result Result
		if err := resp.UnmarshalResult(&result); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkUnmarshalParams benchmarks lazy params unmarshaling
// TODO: Add comparison benchmarks for alternative JSON parsers
func BenchmarkUnmarshalParams(b *testing.B) {
	req, _ := DecodeRequest(mediumRequestJSON)

	type Params struct {
		UserID      int            `json:"userId"`
		Name        string         `json:"name"`
		Email       string         `json:"email"`
		Preferences map[string]any `json:"preferences"`
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var params Params
		if err := req.UnmarshalParams(&params); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkDecodeResponseFromReader benchmarks streaming response decoding
// TODO: Add comparison benchmarks for alternative JSON parsers
// TODO: Add benchmarks with different buffer sizes to measure chunking overhead
func BenchmarkDecodeResponseFromReader(b *testing.B) {
	b.Run("Small", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			reader := bytes.NewReader(smallResponseJSON)
			_, err := DecodeResponseFromReader(reader, len(smallResponseJSON))
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("Large", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			reader := bytes.NewReader(largeResponseJSON)
			_, err := DecodeResponseFromReader(reader, len(largeResponseJSON))
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// Helper functions to generate batch payloads

func makeBatchRequestJSON(count int) []byte {
	var buf bytes.Buffer
	buf.WriteByte('[')
	for i := 0; i < count; i++ {
		if i > 0 {
			buf.WriteByte(',')
		}
		buf.WriteString(`{"jsonrpc":"2.0","id":`)
		buf.WriteString(string(rune('0' + (i % 10))))
		buf.WriteString(`,"method":"test","params":[`)
		buf.WriteString(string(rune('0' + (i % 10))))
		buf.WriteString(`]}`)
	}
	buf.WriteByte(']')
	return buf.Bytes()
}

func makeBatchResponseJSON(count int) []byte {
	var buf bytes.Buffer
	buf.WriteByte('[')
	for i := 0; i < count; i++ {
		if i > 0 {
			buf.WriteByte(',')
		}
		buf.WriteString(`{"jsonrpc":"2.0","id":`)
		buf.WriteString(string(rune('0' + (i % 10))))
		buf.WriteString(`,"result":"ok"}`)
	}
	buf.WriteByte(']')
	return buf.Bytes()
}
