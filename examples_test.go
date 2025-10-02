// Package jsonrpc provides a Go implementation of the JSON-RPC 2.0 specification, as well as tools
// to parse and work with JSON-RPC requests and responses.
package jsonrpc

import (
	"fmt"
)

// ExampleRequest_params_positional demonstrates using positional parameters (array).
func ExampleRequest_params_positional() {
	// Create a request with positional parameters
	req := NewRequest("subtract", []any{42, 23})

	fmt.Printf("Method: %s\n", req.Method)
	fmt.Printf("Params type: %T\n", req.Params)
	fmt.Printf("Params: %v\n", req.Params)
	// Output:
	// Method: subtract
	// Params type: []interface {}
	// Params: [42 23]
}

// ExampleRequest_params_named demonstrates using named parameters (object).
func ExampleRequest_params_named() {
	// Create a request with named parameters
	req := NewRequest("updateUser", map[string]any{
		"userId": 123,
		"name":   "Alice",
		"active": true,
	})

	fmt.Printf("Method: %s\n", req.Method)
	fmt.Printf("Params type: %T\n", req.Params)
	// Output:
	// Method: updateUser
	// Params type: map[string]interface {}
}

// ExampleRequest_UnmarshalParams demonstrates unmarshaling params into a struct.
func ExampleRequest_UnmarshalParams() {
	// Create a request with structured params
	req := NewRequest("createUser", map[string]any{
		"name":  "Bob",
		"email": "bob@example.com",
		"age":   30,
	})

	// Define target struct
	type UserParams struct {
		Name  string `json:"name"`
		Email string `json:"email"`
		Age   int    `json:"age"`
	}

	// Unmarshal params into struct
	var params UserParams
	if err := req.UnmarshalParams(&params); err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Name: %s, Email: %s, Age: %d\n", params.Name, params.Email, params.Age)
	// Output:
	// Name: Bob, Email: bob@example.com, Age: 30
}

// ExampleRequest_params_nil demonstrates a request without parameters.
func ExampleRequest_params_nil() {
	// Create a request with no parameters
	req := NewRequest("getServerTime", nil)

	fmt.Printf("Method: %s\n", req.Method)
	fmt.Printf("Params: %v\n", req.Params)
	// Output:
	// Method: getServerTime
	// Params: <nil>
}

// ExampleError_data demonstrates using the Data field in errors.
func ExampleError_data() {
	// Create an error with additional data
	err := &Error{
		Code:    -32602,
		Message: "Invalid params",
		Data: map[string]any{
			"field":  "email",
			"reason": "invalid format",
		},
	}

	fmt.Printf("Code: %d\n", err.Code)
	fmt.Printf("Message: %s\n", err.Message)
	fmt.Printf("Data type: %T\n", err.Data)
	// Output:
	// Code: -32602
	// Message: Invalid params
	// Data type: map[string]interface {}
}

// ExampleResponse_UnmarshalResult demonstrates unmarshaling result into different types.
func ExampleResponse_UnmarshalResult() {
	// Simulate decoding a response with a structured result
	jsonData := []byte(`{"jsonrpc":"2.0","id":1,"result":{"balance":1000,"currency":"USD"}}`)
	resp, err := DecodeResponse(jsonData)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	// Define target struct
	type BalanceResult struct {
		Balance  int    `json:"balance"`
		Currency string `json:"currency"`
	}

	// Unmarshal result
	var result BalanceResult
	if err := resp.UnmarshalResult(&result); err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Balance: %d %s\n", result.Balance, result.Currency)
	// Output:
	// Balance: 1000 USD
}

// ExampleResponse_UnmarshalResult_primitives demonstrates unmarshaling primitive results.
func ExampleResponse_UnmarshalResult_primitives() {
	// String result
	jsonData := []byte(`{"jsonrpc":"2.0","id":1,"result":"success"}`)
	resp, _ := DecodeResponse(jsonData)

	var strResult string
	resp.UnmarshalResult(&strResult)
	fmt.Printf("String: %s\n", strResult)

	// Number result
	jsonData = []byte(`{"jsonrpc":"2.0","id":2,"result":42}`)
	resp, _ = DecodeResponse(jsonData)

	var intResult int
	resp.UnmarshalResult(&intResult)
	fmt.Printf("Number: %d\n", intResult)

	// Boolean result
	jsonData = []byte(`{"jsonrpc":"2.0","id":3,"result":true}`)
	resp, _ = DecodeResponse(jsonData)

	var boolResult bool
	resp.UnmarshalResult(&boolResult)
	fmt.Printf("Boolean: %t\n", boolResult)

	// Output:
	// String: success
	// Number: 42
	// Boolean: true
}

// ExampleResponse_UnmarshalError demonstrates handling error responses.
func ExampleResponse_UnmarshalError() {
	jsonData := []byte(`{"jsonrpc":"2.0","id":1,"error":{"code":-32601,"message":"Method not found"}}`)
	resp, err := DecodeResponse(jsonData)
	if err != nil {
		fmt.Printf("Decode error: %v\n", err)
		return
	}

	// For responses with errors, the error is automatically unmarshaled during decode
	// Check if response has an error
	if resp.Error != nil {
		fmt.Printf("RPC Error %d: %s\n", resp.Error.Code, resp.Error.Message)
	}
	// Output:
	// RPC Error -32601: Method not found
}

// ExampleNewBatchRequest demonstrates creating batch requests with different param types.
func ExampleNewBatchRequest() {
	methods := []string{"sum", "subtract", "getUser"}
	params := []any{
		[]any{1, 2, 3},              // positional params for sum
		[]any{10, 5},                // positional params for subtract
		map[string]any{"id": 123},   // named params for getUser
	}

	reqs, err := NewBatchRequest(methods, params)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	for i, req := range reqs {
		fmt.Printf("Request %d: %s\n", i, req.Method)
	}
	// Output:
	// Request 0: sum
	// Request 1: subtract
	// Request 2: getUser
}
