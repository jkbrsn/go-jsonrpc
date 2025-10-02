# go-jsonrpc Improvements Analysis

This document provides a comprehensive evaluation of the `go-jsonrpc` library across multiple dimensions, along with detailed recommendations for improvement.

## Overall Assessment: ★★★★☆ (4/5)

A high-quality, well-engineered library with excellent performance characteristics and good attention to detail. The lazy unmarshaling pattern for `Response` is particularly well-designed, and test coverage at 87.3% is strong.

---

## Evaluation by Category

### 1. Go Idiomatics: ★★★★☆ (4/5)

#### Strengths:
- Proper use of unexported auxiliary types to avoid infinite recursion in `MarshalJSON`/`UnmarshalJSON`
- Comprehensive error handling with wrapped errors (`fmt.Errorf` with `%w`)
- Good use of type aliases for recursive prevention (request.go:59, error.go:73)
- Nil-safe methods (`IsEmpty`, `Validate`)
- Concurrency-safe `Response` type with `sync.RWMutex`

#### Issues:
- `utils.go:35-36` - Uses deprecated `rand.Int31()` instead of `rand.N()` (Go 1.24+ target)
- Inconsistent constructor naming: `RequestFromBytes` vs `DecodeResponse`/`DecodeResponseFromReader`
- Deprecated functions still present (`NewResponseFromBytes`, `NewResponseFromStream`) - should be removed or marked more clearly

---

### 2. UX/API Design: ★★★½☆ (3.5/5)

#### Strengths:
- Lazy unmarshaling in `Response` is excellent for performance
- Clear separation: `DecodeResponse` for parsing, `UnmarshalResult` for accessing data
- Type-safe ID handling with `IDString()` convenience method
- Thread-safe `Response` allows concurrent reads

#### Issues:
- **API confusion**: Multiple ways to do the same thing (`RequestFromBytes` vs manual `UnmarshalJSON`, `DecodeResponse` vs `NewResponseFromBytes`)
- **Inconsistent patterns**: `Request` has `RequestFromBytes` but `Response` has both `DecodeResponse` and `NewResponseFromBytes`
- **Missing constructor for Request**: No `NewRequest()` helper; users must manually construct and set `JSONRPC = "2.0"`
- `IDRaw()` method (response.go:188-196) returns `any` after unmarshaling error but swallows the error - confusing API
- No batch request/response support despite being in the spec

---

### 3. Performance: ★★★★★ (5/5)

#### Strengths:
- Excellent use of `bytedance/sonic` for fast JSON operations
- Lazy unmarshaling in `Response` prevents unnecessary work
- Efficient chunked reading with `readAll` (16KB chunks, pre-allocates based on expected size)
- `RWMutex` allows concurrent reads on `Response`
- Smart buffer management (response.go:46-54)

#### Notes:
- Thread-safety overhead might be unnecessary for immutable responses, but this is a defensible design choice

---

### 4. JSON-RPC 2.0 Conformance: ★★★★☆ (4/5)

#### Conformant:
- ✅ Validates `jsonrpc: "2.0"` exactly
- ✅ Validates method is required
- ✅ Validates params must be array or object
- ✅ Response must have result XOR error
- ✅ Properly handles string/number/null IDs
- ✅ Standard error codes defined
- ✅ Allows extra fields (forwards compatible)

#### Documented Deviations:
- ⚠️ Allows fractional IDs (spec says "should not" - acceptable deviation)
- ⚠️ Disallows zero error codes (stricter than spec - questionable choice)

#### Missing Features:
- ❌ No batch request/response support (required by spec)
- ❌ No notification support documentation (requests without ID)
- ❌ Doesn't validate method names starting with "rpc." (reserved by spec)

#### Questionable Decisions:
- error.go:108 - Disallowing zero error codes deviates unnecessarily from spec. If internal sentinel value needed, use `-1` or similar

---

## Detailed Recommendations

### 1. Consolidate and Standardize API

**Problem**: Multiple inconsistent ways to create requests and responses causes confusion.

**Current State**:
- Request: `RequestFromBytes(data)`
- Response: `DecodeResponse(data)`, `NewResponseFromBytes(data)` (deprecated), `DecodeResponseFromReader(r, size)`, `NewResponseFromStream(r, size)` (deprecated)

**Recommended Actions**:
- Rename `RequestFromBytes` → `DecodeRequest` for consistency
- Remove or clearly mark deprecated functions in godoc with deprecation timeline
- Standardize naming pattern: `Decode*` for parsing, `New*` for construction
- Update README with clear migration guide if breaking changes introduced

**Implementation**:
```go
// Deprecated: Use DecodeRequest instead. Will be removed in v2.0.
func RequestFromBytes(data []byte) (*Request, error) {
    return DecodeRequest(data)
}

// DecodeRequest parses a JSON-RPC request from a byte slice.
func DecodeRequest(data []byte) (*Request, error) {
    // existing implementation
}
```

---

### 2. Add Ergonomic Constructors

**Problem**: Users must manually set `JSONRPC = "2.0"` and remember field structure.

**Recommended Actions**:
- Add `NewRequest(method string, params any) *Request` constructor
- Add `NewRequestWithID(method string, params any, id any) *Request` variant
- Add `NewNotification(method string, params any) *Request` for ID-less requests
- Add `NewResponse(id any, result any) *Response` constructor
- Add `NewErrorResponse(id any, err *Error) *Response` constructor

**Implementation Examples**:
```go
// NewRequest creates a JSON-RPC 2.0 request with an auto-generated ID.
func NewRequest(method string, params any) *Request {
    return &Request{
        JSONRPC: "2.0",
        ID:      RandomJSONRPCID(),
        Method:  method,
        Params:  params,
    }
}

// NewRequestWithID creates a JSON-RPC 2.0 request with a specific ID.
func NewRequestWithID(method string, params any, id any) *Request {
    return &Request{
        JSONRPC: "2.0",
        ID:      id,
        Method:  method,
        Params:  params,
    }
}

// NewNotification creates a JSON-RPC 2.0 notification (request without ID).
func NewNotification(method string, params any) *Request {
    return &Request{
        JSONRPC: "2.0",
        Method:  method,
        Params:  params,
    }
}
```

---

### 3. Implement Batch Request/Response Support

**Problem**: JSON-RPC 2.0 spec requires batch support; library doesn't provide it.

**Recommended Actions**:
- Add `DecodeBatchRequest(data []byte) ([]*Request, error)`
- Add `EncodeBatchRequest(reqs []*Request) ([]byte, error)`
- Add `DecodeBatchResponse(data []byte) ([]*Response, error)`
- Add `EncodeBatchResponse(resps []*Response) ([]byte, error)`
- Handle mixed single/batch in decode functions or provide separate `DecodeRequestOrBatch`
- Document batch behavior clearly

**Implementation Considerations**:
```go
// DecodeBatchRequest parses a JSON-RPC batch request.
// Returns a slice of requests if input is an array, or error if invalid.
func DecodeBatchRequest(data []byte) ([]*Request, error) {
    // Check if data starts with '['
    // Unmarshal as []json.RawMessage
    // Parse each element as Request
    // Return slice
}

// DecodeRequestOrBatch attempts to parse either a single request or batch.
// Returns (requests, isBatch, error)
func DecodeRequestOrBatch(data []byte) ([]*Request, bool, error) {
    // Auto-detect single vs batch
}
```

---

### 4. Reconsider Zero Error Code Restriction

**Problem**: Library disallows `code: 0` in errors, deviating from spec unnecessarily.

**Current**: error.go:108 requires non-zero error code
**Spec**: Allows any integer error code

**Recommended Actions**:
- **Option A (Breaking)**: Remove zero-code restriction entirely
- **Option B (Non-breaking)**: Allow zero but document as non-idiomatic
- **Option C**: Use internal sentinel value (`-1`) if zero detection needed for `IsEmpty()`

**Rationale**:
- Spec doesn't forbid zero
- Some RPC systems may use zero for specific error semantics
- Current restriction adds constraint without clear benefit
- `IsEmpty()` could check `Code == 0 && Message == ""` without validation restriction

---

### 5. Fix `RandomJSONRPCID()` for Go 1.24+

**Problem**: Uses deprecated `rand.Int31()` instead of modern `rand.N()`.

**Current** (utils.go:35-36):
```go
func RandomJSONRPCID() int64 {
    return int64(rand.Int31())
}
```

**Recommended**:
```go
// RandomJSONRPCID returns a random JSON-RPC ID value.
// Uses the full int32 positive range as per JSON-RPC best practices.
func RandomJSONRPCID() int64 {
    return int64(rand.IntN(2147483647)) // math.MaxInt32
}
```

---

### 6. Improve or Remove `IDRaw()` Method

**Problem**: `IDRaw()` (response.go:188-196) swallows unmarshaling errors, returning `nil` on failure without indication.

**Current Behavior**:
```go
func (r *Response) IDRaw() any {
    if r.ID == nil {
        err := r.unmarshalID()
        if err != nil {
            return nil  // Error swallowed!
        }
    }
    return r.ID
}
```

**Recommended Actions**:
- **Option A**: Return `(any, error)` - breaking change but explicit
- **Option B**: Rename to `IDOrNil()` to clarify behavior
- **Option C**: Remove entirely - users can access `r.ID` directly after decoding
- **Option D**: Panic on error (follows Go convention for "must" operations)

**Preferred**: Option B for minimal disruption:
```go
// IDOrNil returns the unmarshaled ID, or nil if unmarshaling fails.
// For error handling, check the Response's Validate() method.
func (r *Response) IDOrNil() any {
    // existing implementation
}
```

---

### 7. Add Notification Support Documentation

**Problem**: Library supports notifications (requests without ID) but doesn't document or provide helpers.

**Recommended Actions**:
- Add `IsNotification() bool` method to `Request`
- Document notification behavior in godoc
- Provide `NewNotification()` constructor (see recommendation #2)
- Add example in README showing notification usage
- Clarify that notifications don't expect responses

**Implementation**:
```go
// IsNotification returns true if this is a notification (no ID expected).
func (r *Request) IsNotification() bool {
    return r.ID == nil
}
```

---

### 8. Add Method Name Validation

**Problem**: JSON-RPC 2.0 reserves method names starting with "rpc." but library doesn't validate.

**Recommended Actions**:
- Add check in `Request.Validate()` to warn/error on "rpc." prefix
- Make configurable via `ValidationOptions` if strictness varies
- Document that "rpc." methods are reserved by spec

**Implementation**:
```go
func (r *Request) Validate() error {
    // ... existing checks ...

    if strings.HasPrefix(r.Method, "rpc.") {
        return errors.New("method names starting with 'rpc.' are reserved by JSON-RPC 2.0 spec")
    }

    return nil
}
```

---

### 9. Consider Response Immutability

**Problem**: `Response` uses mutexes for thread-safety, but responses are typically immutable after decoding.

**Current**: Three separate mutexes for ID, Error, and Result fields

**Recommended Actions**:
- **Option A**: Document that `Response` should not be modified after creation
- **Option B**: Remove mutexes and make struct truly immutable (breaking change)
- **Option C**: Add `Freeze()` method to lock response after decoding
- **Keep current**: Thread-safety is valuable for long-lived responses in concurrent environments

**Analysis**: Current approach is defensible. If change made, do so only with major version bump.

---

### 10. Improve Test Coverage

**Current**: 87.3% coverage - strong but could be higher

**Recommended Actions**:
- Add fuzz tests for `UnmarshalJSON` on Request, Response, Error
- Add benchmarks for common operations (marshal, unmarshal, lazy decode)
- Test edge cases: very large payloads, malformed UTF-8, deeply nested params
- Add example tests for godoc
- Test concurrent access patterns more thoroughly

---

## Priority Ranking

### High Priority (Do First):
1. **Fix `RandomJSONRPCID()`** - Quick fix, improves Go version compliance
2. **Add constructors** - Major UX improvement, non-breaking
3. **Consolidate API naming** - Can be done with deprecation warnings

### Medium Priority (Do Soon):
4. **Add batch support** - Required by spec, significant feature gap
5. **Improve `IDRaw()`** - Can cause silent bugs
6. **Add notification helpers** - Improves spec compliance documentation

### Low Priority (Nice to Have):
7. **Reconsider zero error code** - Minor deviation, may not matter in practice
8. **Method name validation** - Edge case, rarely hits real usage
9. **Response immutability review** - Current design is acceptable
10. **Expanded test coverage** - Good but not urgent given 87% coverage

---

## Migration Path

If implementing breaking changes, consider:
1. Create `v2` branch for major refactor
2. Keep `v1` with deprecated functions for 6-12 months
3. Provide migration guide in README
4. Use Go module major version suffix (`/v2`) for clean upgrade path

---

## Conclusion

The `go-jsonrpc` library is well-designed with excellent performance characteristics. The main areas for improvement are:
- API consistency and ergonomics
- Complete JSON-RPC 2.0 spec compliance (batch requests)
- Minor Go idiom updates for modern versions

These improvements would elevate the library from "very good" to "excellent" while maintaining backward compatibility where possible.
