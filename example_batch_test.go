package jsonrpc_test

import (
	"fmt"

	"github.com/jkbrsn/go-jsonrpc"
)

func ExampleEncodeBatchRequest() {
	reqs := []*jsonrpc.Request{
		jsonrpc.NewRequest("sum", []any{1, 2}),
		jsonrpc.NewRequest("subtract", []any{5, 3}),
	}
	data, err := jsonrpc.EncodeBatchRequest(reqs)
	if err != nil {
		panic(err)
	}
	fmt.Println(string(data))
	// Output will contain JSON array with two requests
}

func ExampleDecodeBatchRequest() {
	data := []byte(`[
		{"jsonrpc":"2.0","id":1,"method":"sum","params":[1,2]},
		{"jsonrpc":"2.0","id":2,"method":"subtract","params":[5,3]}
	]`)
	reqs, err := jsonrpc.DecodeBatchRequest(data)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Decoded %d requests\n", len(reqs))
	// Output: Decoded 2 requests
}

func ExampleNewBatchRequest() {
	methods := []string{"sum", "subtract"}
	params := []any{[]any{1, 2}, []any{5, 3}}
	reqs, err := jsonrpc.NewBatchRequest(methods, params)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Created %d requests\n", len(reqs))
	// Output: Created 2 requests
}

func ExampleNewBatchNotification() {
	methods := []string{"log", "notify"}
	params := []any{
		map[string]any{"level": "info"},
		map[string]any{"message": "test"},
	}
	reqs, err := jsonrpc.NewBatchNotification(methods, params)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Created %d notifications\n", len(reqs))
	for _, req := range reqs {
		fmt.Printf("Notification: %t\n", req.IsNotification())
	}
	// Output:
	// Created 2 notifications
	// Notification: true
	// Notification: true
}

func ExampleDecodeRequestOrBatch() {
	// Single request
	singleData := []byte(`{"jsonrpc":"2.0","id":1,"method":"sum","params":[1,2]}`)
	reqs, isBatch, err := jsonrpc.DecodeRequestOrBatch(singleData)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Single - Batch: %t, Count: %d\n", isBatch, len(reqs))

	// Batch request
	batchData := []byte(`[
		{"jsonrpc":"2.0","id":1,"method":"sum","params":[1,2]},
		{"jsonrpc":"2.0","id":2,"method":"subtract","params":[5,3]}
	]`)
	reqs, isBatch, err = jsonrpc.DecodeRequestOrBatch(batchData)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Batch - Batch: %t, Count: %d\n", isBatch, len(reqs))
	// Output:
	// Single - Batch: false, Count: 1
	// Batch - Batch: true, Count: 2
}

func ExampleDecodeBatchResponse() {
	data := []byte(`[
		{"jsonrpc":"2.0","id":1,"result":3},
		{"jsonrpc":"2.0","id":2,"error":{"code":-32601,"message":"Method not found"}}
	]`)
	resps, err := jsonrpc.DecodeBatchResponse(data)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Decoded %d responses\n", len(resps))
	// Output: Decoded 2 responses
}

func ExampleEncodeBatchResponse() {
	resp1, _ := jsonrpc.NewResponse(int64(1), 42)
	resp2 := jsonrpc.NewErrorResponse(int64(2), &jsonrpc.Error{
		Code:    jsonrpc.MethodNotFound,
		Message: "Method not found",
	})

	resps := []*jsonrpc.Response{resp1, resp2}
	data, err := jsonrpc.EncodeBatchResponse(resps)
	if err != nil {
		panic(err)
	}
	fmt.Println(len(data) > 0)
	// Output: true
}
