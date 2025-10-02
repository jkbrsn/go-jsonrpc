# BATCH SUPPORT

## Implement Batch Request/Response Support

**Problem**: JSON-RPC 2.0 spec requires batch support; library doesn't provide it.

**Status**: ⏳ Not yet implemented

**Scope**: This is a significant feature addition that requires:
1. New batch decoding/encoding functions
2. Comprehensive validation (spec requires non-empty arrays, handling notifications in batches)
3. Extensive test coverage for edge cases
4. Documentation updates with examples


## Detailed Batch Support Implementation Plan

### Phase 1: Core Batch Types & Detection

**File**: `batch.go` (new file)

**1.1 Add batch detection helper**
```go
// isBatchJSON returns true if the trimmed data starts with '[', indicating a batch request/response.
func isBatchJSON(data []byte) bool {
    trimmed := bytes.TrimSpace(data)
    return len(trimmed) > 0 && trimmed[0] == '['
}
```

**1.2 Add unified decode functions for auto-detection**
```go
// DecodeRequestOrBatch attempts to parse either a single request or a batch of requests.
// Returns (requests, isBatch, error).
// - For single requests: returns slice with one element, isBatch=false
// - For batch requests: returns slice with multiple elements, isBatch=true
// - Empty batches are rejected per JSON-RPC 2.0 spec
func DecodeRequestOrBatch(data []byte) ([]*Request, bool, error) {
    if len(bytes.TrimSpace(data)) == 0 {
        return nil, false, fmt.Errorf("empty data")
    }

    if isBatchJSON(data) {
        reqs, err := DecodeBatchRequest(data)
        return reqs, true, err
    }

    req, err := DecodeRequest(data)
    if err != nil {
        return nil, false, err
    }
    return []*Request{req}, false, nil
}

// DecodeResponseOrBatch attempts to parse either a single response or a batch of responses.
// Returns (responses, isBatch, error).
func DecodeResponseOrBatch(data []byte) ([]*Response, bool, error) {
    if len(bytes.TrimSpace(data)) == 0 {
        return nil, false, fmt.Errorf("empty data")
    }

    if isBatchJSON(data) {
        resps, err := DecodeBatchResponse(data)
        return resps, true, err
    }

    resp, err := DecodeResponse(data)
    if err != nil {
        return nil, false, err
    }
    return []*Response{resp}, false, nil
}
```

### Phase 2: Batch Request Functions

**2.1 Add DecodeBatchRequest**
```go
// DecodeBatchRequest parses a JSON-RPC batch request from a byte slice.
// Returns an error if:
// - Input is not a JSON array
// - Array is empty (per JSON-RPC 2.0 spec: "The Server should respond with an error")
// - Any element fails to parse as a valid Request
func DecodeBatchRequest(data []byte) ([]*Request, error) {
    if len(bytes.TrimSpace(data)) == 0 {
        return nil, fmt.Errorf("empty data")
    }

    // Unmarshal as array of raw messages
    var rawMessages []json.RawMessage
    if err := sonic.Unmarshal(data, &rawMessages); err != nil {
        return nil, fmt.Errorf("invalid batch format: %w", err)
    }

    // Spec requires non-empty batches
    if len(rawMessages) == 0 {
        return nil, fmt.Errorf("batch request must contain at least one request")
    }

    // Parse each request
    requests := make([]*Request, 0, len(rawMessages))
    for i, raw := range rawMessages {
        req, err := DecodeRequest(raw)
        if err != nil {
            return nil, fmt.Errorf("invalid request at index %d: %w", i, err)
        }
        requests = append(requests, req)
    }

    return requests, nil
}
```

**2.2 Add EncodeBatchRequest**
```go
// EncodeBatchRequest marshals a slice of JSON-RPC requests into a batch (JSON array).
// Returns an error if:
// - Input slice is empty
// - Any request fails validation
func EncodeBatchRequest(reqs []*Request) ([]byte, error) {
    if len(reqs) == 0 {
        return nil, fmt.Errorf("batch request must contain at least one request")
    }

    // Validate all requests first
    for i, req := range reqs {
        if err := req.Validate(); err != nil {
            return nil, fmt.Errorf("invalid request at index %d: %w", i, err)
        }
    }

    // Marshal as array
    return sonic.Marshal(reqs)
}
```

### Phase 3: Batch Response Functions

**3.1 Add DecodeBatchResponse**
```go
// DecodeBatchResponse parses a JSON-RPC batch response from a byte slice.
// Returns an error if:
// - Input is not a JSON array
// - Array is empty
// - Any element fails to parse as a valid Response
func DecodeBatchResponse(data []byte) ([]*Response, error) {
    if len(bytes.TrimSpace(data)) == 0 {
        return nil, fmt.Errorf("empty data")
    }

    // Unmarshal as array of raw messages
    var rawMessages []json.RawMessage
    if err := sonic.Unmarshal(data, &rawMessages); err != nil {
        return nil, fmt.Errorf("invalid batch format: %w", err)
    }

    if len(rawMessages) == 0 {
        return nil, fmt.Errorf("batch response must contain at least one response")
    }

    // Parse each response
    responses := make([]*Response, 0, len(rawMessages))
    for i, raw := range rawMessages {
        resp, err := DecodeResponse(raw)
        if err != nil {
            return nil, fmt.Errorf("invalid response at index %d: %w", i, err)
        }
        responses = append(responses, resp)
    }

    return responses, nil
}
```

**3.2 Add EncodeBatchResponse**
```go
// EncodeBatchResponse marshals a slice of JSON-RPC responses into a batch (JSON array).
// Returns an error if:
// - Input slice is empty
// - Any response fails validation
func EncodeBatchResponse(resps []*Response) ([]byte, error) {
    if len(resps) == 0 {
        return nil, fmt.Errorf("batch response must contain at least one response")
    }

    // Validate all responses first
    for i, resp := range resps {
        if err := resp.Validate(); err != nil {
            return nil, fmt.Errorf("invalid response at index %d: %w", i, err)
        }
    }

    // Marshal as array
    return sonic.Marshal(resps)
}
```

### Phase 4: Batch Reader Support

**4.1 Add DecodeBatchRequestFromReader**
```go
// DecodeBatchRequestFromReader parses a JSON-RPC batch request from an io.Reader.
func DecodeBatchRequestFromReader(r io.Reader, expectedSize int) ([]*Request, error) {
    if r == nil {
        return nil, errors.New("cannot read from nil reader")
    }

    chunkSize := 16 * 1024
    data, err := readAll(r, int64(chunkSize), expectedSize)
    if err != nil {
        return nil, fmt.Errorf("failed to read batch request: %w", err)
    }

    return DecodeBatchRequest(data)
}
```

**4.2 Add DecodeBatchResponseFromReader**
```go
// DecodeBatchResponseFromReader parses a JSON-RPC batch response from an io.Reader.
func DecodeBatchResponseFromReader(r io.Reader, expectedSize int) ([]*Response, error) {
    if r == nil {
        return nil, errors.New("cannot read from nil reader")
    }

    chunkSize := 16 * 1024
    data, err := readAll(r, int64(chunkSize), expectedSize)
    if err != nil {
        return nil, fmt.Errorf("failed to read batch response: %w", err)
    }

    return DecodeBatchResponse(data)
}
```

### Phase 5: Helper Functions

**5.1 Add NewBatchRequest**
```go
// NewBatchRequest creates a batch of JSON-RPC requests from methods and params.
// Each request receives an auto-generated ID.
func NewBatchRequest(methods []string, params []any) ([]*Request, error) {
    if len(methods) == 0 {
        return nil, fmt.Errorf("batch must contain at least one method")
    }
    if len(params) > 0 && len(params) != len(methods) {
        return nil, fmt.Errorf("params length must match methods length or be empty")
    }

    requests := make([]*Request, len(methods))
    for i, method := range methods {
        var p any
        if i < len(params) {
            p = params[i]
        }
        requests[i] = NewRequest(method, p)
    }

    return requests, nil
}
```

**5.2 Add batch notification helper**
```go
// NewBatchNotification creates a batch of notifications (requests without IDs).
func NewBatchNotification(methods []string, params []any) ([]*Request, error) {
    if len(methods) == 0 {
        return nil, fmt.Errorf("batch must contain at least one method")
    }
    if len(params) > 0 && len(params) != len(methods) {
        return nil, fmt.Errorf("params length must match methods length or be empty")
    }

    requests := make([]*Request, len(methods))
    for i, method := range methods {
        var p any
        if i < len(params) {
            p = params[i]
        }
        requests[i] = NewNotification(method, p)
    }

    return requests, nil
}
```

### Phase 6: Testing

**File**: `batch_test.go` (new file)

**6.1 Test Coverage Required**:
- `TestDecodeBatchRequest`: Valid batches, empty batches, invalid JSON, mixed valid/invalid requests
- `TestEncodeBatchRequest`: Valid batches, empty input, invalid requests
- `TestDecodeBatchResponse`: Valid batches, empty batches, invalid JSON, mixed valid/invalid responses
- `TestEncodeBatchResponse`: Valid batches, empty input, invalid responses
- `TestDecodeRequestOrBatch`: Single requests, batch requests, auto-detection
- `TestDecodeResponseOrBatch`: Single responses, batch responses, auto-detection
- `TestNewBatchRequest`: Valid input, mismatched lengths
- `TestBatchNotifications`: All-notification batches (spec allows no responses)
- `TestBatchWithMixedIDTypes`: String, int, float IDs in same batch
- `TestBatchFromReader`: Reader-based batch parsing
- **Edge cases**:
  - Very large batches (1000+ requests)
  - Batches containing only notifications (no responses expected)
  - Batches with duplicate IDs (allowed but questionable)
  - Deeply nested params in batch requests

**6.2 Example Test Structure**:
```go
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
}
```

### Phase 7: Documentation Updates

**7.1 Update README.md**:
Add section "Batch Requests and Responses":
```markdown
### Batch Requests and Responses

The library supports JSON-RPC 2.0 batch operations for sending multiple requests or responses in a single call.

#### Encoding a Batch Request
```go
reqs := []*jsonrpc.Request{
    jsonrpc.NewRequest("sum", []int{1, 2}),
    jsonrpc.NewRequest("subtract", []int{5, 3}),
}
data, err := jsonrpc.EncodeBatchRequest(reqs)

// Or use the helper:
reqs, err := jsonrpc.NewBatchRequest(
    []string{"sum", "subtract"},
    []any{[]int{1, 2}, []int{5, 3}},
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
    []any{"message 1", "message 2"},
)
```

**7.2 Add godoc examples**:
Create `example_batch_test.go` with runnable examples for godoc.

### Phase 8: Spec Compliance Verification

**8.1 JSON-RPC 2.0 Batch Requirements Checklist**:
- ✅ Batch = JSON array of request/response objects
- ✅ Empty arrays must return error
- ✅ Server may process requests in any order
- ✅ Responses may be returned in any order
- ✅ Notifications in batch don't get responses
- ✅ If all requests are notifications, no response sent (document this behavior)
- ✅ Invalid JSON returns single error response (handled at transport layer, not library)

### Implementation Priority & Effort Estimate

**Total Effort**: ~8-12 hours
- Phase 1-3 (Core functions): 3-4 hours
- Phase 4 (Reader support): 1 hour
- Phase 5 (Helpers): 1 hour
- Phase 6 (Testing): 3-4 hours
- Phase 7-8 (Docs & validation): 1-2 hours

**Recommended Approach**:
1. Implement Phases 1-3 first (minimal viable batch support)
2. Write tests (Phase 6) immediately after each phase
3. Add helpers and readers (Phases 4-5) once core is stable
4. Document and validate (Phases 7-8)

**Breaking Changes**: None - this is purely additive
