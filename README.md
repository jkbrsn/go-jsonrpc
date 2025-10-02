# go-jsonrpc [![Go Documentation](http://img.shields.io/badge/go-documentation-blue.svg?style=flat-square)][godocs] [![MIT License](http://img.shields.io/badge/license-MIT-blue.svg?style=flat-square)][license]

[godocs]: http://godoc.org/github.com/jkbrsn/go-jsonrpc
[license]: /LICENSE

A JSON-RPC 2.0 implementation in Go.

Utilizes the [bytedance/sonic](https://github.com/bytedance/sonic) library for JSON serialization.

Attempts to conform fully to the [JSON-RPC 2.0 Specification](https://www.jsonrpc.org/specification), with a few minor exceptions:

- The `id` field is allowed to be fractional numbers, in addition to integers and strings. The specification notes that "numbers should not contain fractional parts", but this library allows them for convenience.
- Error handling
  - Error codes can be zero if a message is provided (the specification allows any integer).
  - The `error` field is flexible in unmarshaling, with fallback logic to handle various error formats for custom error handling downstream.


## Install

```bash
go get github.com/jkbrsn/go-jsonrpc
```

## Usage

### Single Requests and Responses

#### Creating and Encoding a Request

```go
// Create a request with auto-generated ID
req := jsonrpc.NewRequest("sum", []any{1, 2})

// Or create with a specific ID
req := jsonrpc.NewRequestWithID("sum", []any{1, 2}, "my-id")

// Encode to JSON
data, err := req.MarshalJSON()
```

#### Decoding and Handling a Response

```go
// Decode a response from JSON
resp, err := jsonrpc.DecodeResponse(data)
if err != nil {
    // Handle decode error
}

// Check for JSON-RPC error
if resp.Error != nil {
    fmt.Printf("RPC Error: %s\n", resp.Error.Message)
    return
}

// Unmarshal the result into your type
var result int
if err := resp.UnmarshalResult(&result); err != nil {
    // Handle unmarshal error
}
fmt.Printf("Result: %d\n", result)
```

#### Creating a Notification

```go
// Notifications are requests without IDs (no response expected)
notification := jsonrpc.NewNotification("log", map[string]any{
    "level": "info",
    "message": "Operation completed",
})
```

### Batch Requests and Responses

The library supports JSON-RPC 2.0 batch operations for sending multiple requests or responses in a single call.

#### Encoding a Batch Request

```go
reqs := []*jsonrpc.Request{
    jsonrpc.NewRequest("sum", []any{1, 2}),
    jsonrpc.NewRequest("subtract", []any{5, 3}),
}
data, err := jsonrpc.EncodeBatchRequest(reqs)

// Or use the helper:
reqs, err := jsonrpc.NewBatchRequest(
    []string{"sum", "subtract"},
    []any{[]any{1, 2}, []any{5, 3}},
)
```

#### Decoding a Batch Response

```go
resps, err := jsonrpc.DecodeBatchResponse(data)
for _, resp := range resps {
    if resp.Error != nil {
        // Handle error
    } else {
        var result int
        resp.UnmarshalResult(&result)
    }
}
```

#### Auto-detecting Single vs Batch

```go
resps, isBatch, err := jsonrpc.DecodeResponseOrBatch(data)
if isBatch {
    fmt.Printf("Received batch with %d responses\n", len(resps))
} else {
    fmt.Println("Received single response")
}
```

#### Notifications in Batches

Batches can contain notifications (requests without IDs). The server should not send responses for notifications:

```go
reqs, err := jsonrpc.NewBatchNotification(
    []string{"log", "notify"},
    []any{map[string]any{"level": "info"}, map[string]any{"message": "test"}},
)
```

## Contributing

For contributions, please open a GitHub issue with your questions and suggestions. Before submitting an issue, have a look at the existing [TODO list](TODO.md) to see if your idea is already in the works.