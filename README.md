# jsonrpc [![Go Documentation](http://img.shields.io/badge/go-documentation-blue.svg?style=flat-square)][godocs] [![MIT License](http://img.shields.io/badge/license-MIT-blue.svg?style=flat-square)][license]

[godocs]: http://godoc.org/github.com/jkbrsn/jsonrpc
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
go get github.com/jkbrsn/jsonrpc
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
if resp.Err() != nil {
    fmt.Printf("RPC Error: %s\n", resp.Err().Message)
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

### Working with Params

The library supports both positional (array) and named (object) parameters, as well as structured parameter unmarshaling.

#### Positional Parameters

```go
// Create a request with array parameters
req := jsonrpc.NewRequest("subtract", []any{42, 23})
// Params: [42, 23]
```

#### Named Parameters

```go
// Create a request with object parameters
req := jsonrpc.NewRequest("updateUser", map[string]any{
    "userId": 123,
    "name":   "Alice",
    "active": true,
})
// Params: {"userId": 123, "name": "Alice", "active": true}
```

#### Unmarshaling Params into Structs

```go
// Define your parameter structure
type UserParams struct {
    Name  string `json:"name"`
    Email string `json:"email"`
    Age   int    `json:"age"`
}

// Unmarshal params into the struct
var params UserParams
if err := req.UnmarshalParams(&params); err != nil {
    // Handle error
}
// Use params.Name, params.Email, params.Age
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
    if resp.Err() != nil {
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

## Performance

This library is optimized for high-throughput server applications using several techniques:

### Performance Profiles

The library provides five performance profiles that allow you to choose the right trade-off between speed, safety, and compatibility:

```go
// Use balanced profile for production (recommended)
jsonrpc.SetPerformanceProfile(jsonrpc.ProfileBalanced)

// Or choose another profile based on your needs
jsonrpc.SetPerformanceProfile(jsonrpc.ProfileFast)
```

**Available Profiles:**

- **`ProfileDefault`** (default): Sonic's efficient defaults - recommended for most usess
- **`ProfileCompatible`**: Mimics `encoding/json` behavior for a slower, but deterministic, output with sorted keys
- **`ProfileBalanced`**: Safe performance optimizations, ~5-10% faster than default with effectively zero risk
- **`ProfileFast`**: Sonic's `ConfigFastest` - skips some validation for internal services
- **`ProfileAggressive`**: Maximum speed, minimum validation - only for experts with controlled input

**Choosing a Profile:**

| Profile | Best For | Performance | Safety |
|---------|----------|-------------|--------|
| **ProfileDefault** | General use, untrusted input | Baseline | High |
| **ProfileCompatible** | Migration from encoding/json | Slower | High |
| **ProfileBalanced** | Production apps | +5-10% | High |
| **ProfileFast** | Internal services | +10-15% | Medium |
| **ProfileAggressive** | HFT, real-time systems | +15-20% | Low |

See [performance.go](performance.go) for detailed documentation on each profile.

### Codec Pre-compilation (Enabled by Default)

The library pre-compiles JSON codecs at startup using `sonic.Pretouch`, which eliminates JIT compilation overhead on the first marshal/unmarshal operation. This provides:

- **80-99% improvement in first-call latency**: From ~1-5ms down to ~10-50μs
- **Better P99 latency**: No unpredictable "warm-up" spike
- **Small startup cost**: ~10-50ms additional initialization time

**For CLI tools or single-shot programs**, this optimization can be disabled to avoid the startup cost:

```bash
go build -tags nopretouch
```

### Buffer Pooling

Stream reading operations (`DecodeResponseFromReader`, `DecodeBatchRequestFromReader`, etc.) use `sync.Pool` for buffer reuse, reducing GC pressure in high-throughput scenarios.

### Lazy Unmarshaling

Response objects use lazy unmarshaling for ID and Error fields, deferring parsing until accessed. This is beneficial when handling large batches where you may not need to inspect every field.

### ID Byte Caching

Response IDs are marshaled once and cached, avoiding re-marshaling on every `MarshalJSON` or `WriteTo` call. This is most valuable when responses are marshaled multiple times (e.g., for caching or retries).

## Release Process

See [release.yml](.github/workflows/release.yml) for the release process specifics.

### Release Workflow Behavior

The Manual Release workflow enforces consistent versioning rules:

- `version = 1.5.0`, `prerelease = true` → creates `v1.5.0-rc.1` (or the next `-rc.N` if others exist).
- `version = 1.5.0-rc.7`, `prerelease = true` → creates exactly `v1.5.0-rc.7` (if not already tagged).
- `version = v1.5.0`, `prerelease = false` → creates final `v1.5.0` (allowed even if prereleases exist).
- If final `v1.5.0` already exists → both `prerelease = true` and final runs for `1.5.0` are blocked.
- Base version bumping → the base `X.Y.Z` must always be strictly greater than the latest existing final tag in the repo.

This means you can iterate with `prerelease = true` and later “promote” the same base to a final, but you cannot reuse or downgrade existing finals.


## Contributing

For contributions, please open a GitHub issue with your questions and suggestions. Before submitting an issue, have a look at the existing [TODO list](TODO.md) to see if your idea is already in the works.