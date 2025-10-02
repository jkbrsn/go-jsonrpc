# Response Immutability Updates

This document outlines the step-by-step plan to make the `Response` type truly immutable, replacing the current three-mutex design with a `sync.Once`-based approach.

**Status**: ⏳ Not yet implemented
**Breaking Change**: Yes - Part of v2.0 redesign
**Rationale**: Responses are immutable after decoding; the current mutex design adds unnecessary overhead for a write-once, read-many pattern.

---

## Current State Analysis

### Existing Implementation (response.go:16-30)

```go
type Response struct {
    JSONRPC string

    ID    any
    rawID json.RawMessage
    muID  sync.RWMutex        // ← Remove

    Error    *Error
    rawError json.RawMessage
    muErr    sync.RWMutex     // ← Remove

    Result   json.RawMessage
    muResult sync.RWMutex     // ← Remove
}
```

### Problems with Current Design

1. **Three separate mutexes** for fields that are written once and read many times
2. **Lock contention** on every read operation (`IDString()`, `IsEmpty()`, `Equals()`, etc.)
3. **Memory overhead**: ~24 bytes per response for mutexes
4. **Misleading API**: Suggests responses can be modified after creation
5. **Cognitive complexity**: Managing three locks correctly is error-prone

---

## Target Design

### New Implementation

```go
type Response struct {
    JSONRPC string

    // Public fields (set once during decode, never modified)
    ID     any
    Error  *Error
    Result json.RawMessage

    // Internal raw fields for lazy unmarshaling
    rawID    json.RawMessage
    rawError json.RawMessage

    // One-time initialization guards
    idOnce  sync.Once
    errOnce sync.Once
}
```

### Key Principles

1. **Immutable after decode**: Fields are set during `UnmarshalJSON`/`parseFromBytes` and never modified
2. **Lazy unmarshaling preserved**: `sync.Once` ensures lazy operations run exactly once
3. **Thread-safe reads**: Multiple goroutines can safely read without locks
4. **No setters**: Remove any methods that modify response state

---

## Implementation Plan

### Phase 1: Update Response Struct

**File**: `response.go`

**1.1 Replace mutexes with sync.Once**

```go
type Response struct {
    JSONRPC string

    // Public immutable fields
    ID     any
    Error  *Error
    Result json.RawMessage

    // Internal fields for lazy unmarshaling
    rawID    json.RawMessage
    rawError json.RawMessage

    // One-time initialization guards for lazy operations
    idOnce  sync.Once
    errOnce sync.Once
}
```

**1.2 Update godoc**

```go
// Response is a struct for JSON-RPC responses conforming to the JSON-RPC 2.0 specification.
// Response instances are immutable after decoding and safe for concurrent reads.
// Do not modify Response fields directly after calling DecodeResponse or UnmarshalJSON.
//
// The Response type uses lazy unmarshaling for the ID and Error fields to optimize performance.
// These fields are unmarshaled on first access via IDOrNil() or UnmarshalError() respectively.
type Response struct { /* ... */ }
```

---

### Phase 2: Update unmarshalID Method

**File**: `response.go:117-155`

**Current implementation (with mutex)**:
```go
func (r *Response) unmarshalID() error {
    r.muID.Lock()
    defer r.muID.Unlock()

    if len(r.rawID) > 0 {
        // ... unmarshaling logic ...
    }
    return nil
}
```

**New implementation (sync.Once pattern)**:
```go
// unmarshalID unmarshals the raw ID bytes into the ID field.
// This function is designed to be called via sync.Once to ensure it runs exactly once.
// It stores any error in a captured variable for the caller to handle.
func (r *Response) unmarshalID() error {
    if len(r.rawID) == 0 {
        r.ID = nil
        return nil
    }

    var id any
    if err := sonic.Unmarshal(r.rawID, &id); err != nil {
        return fmt.Errorf("invalid id field: %w", err)
    }

    // If the value is "null", id will be nil
    if id == nil {
        r.ID = nil
        return nil
    }

    switch v := id.(type) {
    case float64:
        // JSON numbers are unmarshalled as float64, so an explicit integer check is needed
        if v != float64(int64(v)) {
            r.ID = v
        } else {
            r.ID = int64(v)
        }
    case string:
        if v == "" {
            r.ID = nil
        } else {
            r.ID = v
        }
    default:
        return errors.New("id field must be a string or a number")
    }

    return nil
}
```

**Note**: Remove the Lock/Unlock calls since sync.Once handles synchronization.

---

### Phase 3: Update IDOrNil Method

**File**: `response.go:188-204`

**Current implementation**:
```go
func (r *Response) IDOrNil() any {
    if r.ID == nil {
        err := r.unmarshalID()
        if err != nil {
            return nil
        }
    }
    return r.ID
}
```

**New implementation**:
```go
// IDOrNil returns the unmarshaled ID, or nil if unmarshaling fails.
// The ID is unmarshaled lazily on first call and cached for subsequent calls.
// For error handling, check the Response's Validate() method during decode.
func (r *Response) IDOrNil() any {
    r.idOnce.Do(func() {
        // Ignore error - validation happens during decode
        // If unmarshal fails, ID remains nil
        _ = r.unmarshalID()
    })
    return r.ID
}
```

---

### Phase 4: Update UnmarshalError Method

**File**: `response.go:309-319`

**Current implementation**:
```go
func (r *Response) UnmarshalError() error {
    r.muErr.Lock()
    defer r.muErr.Unlock()

    if r.Error == nil && len(r.rawError) > 0 {
        r.Error = &Error{}
        return r.Error.UnmarshalJSON(r.rawError)
    }
    return nil
}
```

**New implementation**:
```go
// UnmarshalError unmarshals the raw error into the Error field.
// The error is unmarshaled lazily on first call and cached for subsequent calls.
// Returns an error if unmarshaling fails.
func (r *Response) UnmarshalError() error {
    var unmarshalErr error

    r.errOnce.Do(func() {
        if r.Error == nil && len(r.rawError) > 0 {
            r.Error = &Error{}
            unmarshalErr = r.Error.UnmarshalJSON(r.rawError)
        }
    })

    return unmarshalErr
}
```

**Important**: We need to capture the error from inside the `Do()` closure since sync.Once doesn't propagate errors.

---

### Phase 5: Update parseFromBytes Method

**File**: `response.go:54-115`

**Changes needed**:

1. **Remove all mutex locks** (lines 89-91, 100-102, 104-106)
2. **Direct field assignment** instead of lock-protected assignment

**Current code with mutexes**:
```go
// Parse the ID field
r.muID.Lock()
r.rawID = aux.ID
r.muID.Unlock()

// Assign result or error accordingly
if aux.Result != nil {
    r.muResult.Lock()
    r.Result = aux.Result
    r.muResult.Unlock()
} else {
    r.muErr.Lock()
    r.rawError = aux.Error
    r.muErr.Unlock()
}
```

**New code (immutable)**:
```go
// Parse the ID field (written once, immutable thereafter)
r.rawID = aux.ID

// Assign result or error accordingly (written once, immutable thereafter)
if aux.Result != nil {
    r.Result = aux.Result
} else {
    r.rawError = aux.Error
}
```

**Note**: This method is only called during initial decode, so no synchronization is needed.

---

### Phase 6: Update Equals Method

**File**: `response.go:157-186`

**Changes needed**: Remove RLock/RUnlock calls

**Current code**:
```go
r.muResult.RLock()
other.muResult.RLock()
defer r.muResult.RUnlock()
defer other.muResult.RUnlock()

if r.Result != nil && other.Result != nil {
    if string(r.Result) != string(other.Result) {
        return false
    }
}
```

**New code**:
```go
// Direct field access - responses are immutable
if r.Result != nil && other.Result != nil {
    if string(r.Result) != string(other.Result) {
        return false
    }
}
```

---

### Phase 7: Update IsEmpty Method

**File**: `response.go:212-246`

**Changes needed**: Remove RLock/RUnlock calls

**Current code**:
```go
r.muResult.RLock()
defer r.muResult.RUnlock()

// Case: both error and result are empty
if r.Error == nil && len(r.Result) == 0 {
    return true
}
```

**New code**:
```go
// Direct field access - responses are immutable
// Case: both error and result are empty
if r.Error == nil && len(r.Result) == 0 {
    return true
}
```

---

### Phase 8: Update MarshalJSON Method

**File**: `response.go:248-302`

**Changes needed**: Remove all mutex locks (RLock/RUnlock)

**Current code**:
```go
// Retrieve the ID value
r.muID.RLock()
var id any
if r.ID != nil {
    id = r.ID
} else if r.rawID != nil {
    id = r.rawID
} else {
    id = nil
}
r.muID.RUnlock()

// Retrieve the error value.
r.muErr.RLock()
if len(r.rawError) > 0 && r.Error == nil {
    // ...
}
errVal := r.Error
r.muErr.RUnlock()

// Retrieve the result.
r.muResult.RLock()
var result json.RawMessage
if len(r.Result) > 0 {
    result = json.RawMessage(r.Result)
}
r.muResult.RUnlock()
```

**New code**:
```go
// Retrieve the ID value (immutable read)
var id any
if r.ID != nil {
    id = r.ID
} else if r.rawID != nil {
    id = r.rawID
} else {
    id = nil
}

// Retrieve the error value (immutable read)
// If rawError exists but Error hasn't been unmarshaled, do it now
if len(r.rawError) > 0 && r.Error == nil {
    r.Error = &Error{}
    if err := r.Error.UnmarshalJSON(r.rawError); err != nil {
        return nil, fmt.Errorf("failed to unmarshal JSON-RPC error: %w", err)
    }
}
errVal := r.Error

// Retrieve the result (immutable read)
var result json.RawMessage
if len(r.Result) > 0 {
    result = json.RawMessage(r.Result)
}
```

---

### Phase 9: Update UnmarshalJSON Method

**File**: `response.go:321-349`

**Changes needed**: Remove mutex locks

**Current code**:
```go
// If the response carries an error (and no result), decode it eagerly
r.muResult.RLock()
resultEmpty := len(r.Result) == 0
r.muResult.RUnlock()

if resultEmpty {
    r.muErr.Lock()
    if r.Error == nil && len(r.rawError) > 0 {
        r.Error = &Error{}
        if err := r.Error.UnmarshalJSON(r.rawError); err != nil {
            r.muErr.Unlock()
            return fmt.Errorf("failed to unmarshal JSON-RPC error: %w", err)
        }
    }
    r.muErr.Unlock()
}
```

**New code**:
```go
// If the response carries an error (and no result), decode it eagerly
// (immutable read - no lock needed)
if len(r.Result) == 0 {
    if r.Error == nil && len(r.rawError) > 0 {
        r.Error = &Error{}
        if err := r.Error.UnmarshalJSON(r.rawError); err != nil {
            return fmt.Errorf("failed to unmarshal JSON-RPC error: %w", err)
        }
    }
}
```

---

### Phase 10: Update UnmarshalResult Method

**File**: `response.go:351-366`

**Changes needed**: Remove RLock/RUnlock

**Current code**:
```go
r.muResult.RLock()
if len(r.Result) == 0 {
    r.muResult.RUnlock()
    return errors.New("response has no result field")
}
raw := r.Result
r.muResult.RUnlock()

return sonic.Unmarshal(raw, dst)
```

**New code**:
```go
// Direct immutable read
if len(r.Result) == 0 {
    return errors.New("response has no result field")
}

return sonic.Unmarshal(r.Result, dst)
```

---

### Phase 11: Update Validate Method

**File**: `response.go:368-397`

**Changes needed**: Remove RLock/RUnlock calls

**Current code**:
```go
r.muErr.RLock()
r.muResult.RLock()
defer r.muErr.RUnlock()
defer r.muResult.RUnlock()

if r.Error != nil && r.Result != nil || r.rawError != nil && r.Result != nil {
    return errors.New("response must not contain both result and error")
}
if r.Error == nil && len(r.rawError) == 0 && r.Result == nil {
    return errors.New("response must contain either result or error")
}
```

**New code**:
```go
// Direct immutable reads
if r.Error != nil && r.Result != nil || r.rawError != nil && r.Result != nil {
    return errors.New("response must not contain both result and error")
}
if r.Error == nil && len(r.rawError) == 0 && r.Result == nil {
    return errors.New("response must contain either result or error")
}
```

---

### Phase 12: Update Tests

**File**: `response_test.go`

**12.1 Update concurrency tests**

The existing concurrency tests (TestResponse_Concurrency) should still pass, but update documentation:

```go
// TestResponse_Concurrency verifies that Response is safe for concurrent reads.
// Responses are immutable after decode, so concurrent access should never race.
func TestResponse_Concurrency(t *testing.T) {
    // ... existing tests ...
}
```

**12.2 Add immutability verification test**

Add a new test to verify responses behave as immutable:

```go
// TestResponse_Immutability verifies that Response fields don't change after decode.
func TestResponse_Immutability(t *testing.T) {
    data := []byte(`{"jsonrpc":"2.0","id":123,"result":"success"}`)

    resp, err := DecodeResponse(data)
    require.NoError(t, err)

    // Capture initial state
    originalID := resp.ID
    originalResult := string(resp.Result)

    // Access ID multiple times (triggers lazy unmarshal on first call)
    id1 := resp.IDOrNil()
    id2 := resp.IDOrNil()
    id3 := resp.IDOrNil()

    // All should return same value
    assert.Equal(t, id1, id2)
    assert.Equal(t, id2, id3)
    assert.Equal(t, originalID, id1)

    // Result should never change
    assert.Equal(t, originalResult, string(resp.Result))

    // Concurrent reads should see consistent state
    var wg sync.WaitGroup
    for i := 0; i < 100; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            assert.Equal(t, originalID, resp.IDOrNil())
            assert.Equal(t, originalResult, string(resp.Result))
        }()
    }
    wg.Wait()
}
```

**12.3 Add lazy unmarshal sync.Once test**

```go
// TestResponse_LazyUnmarshalOnce verifies that unmarshalID runs exactly once.
func TestResponse_LazyUnmarshalOnce(t *testing.T) {
    data := []byte(`{"jsonrpc":"2.0","id":"test-id","result":true}`)

    resp, err := DecodeResponse(data)
    require.NoError(t, err)

    // Before any IDOrNil call, ID might be nil (if not eagerly unmarshaled)
    // Call IDOrNil concurrently from multiple goroutines
    var wg sync.WaitGroup
    results := make([]any, 100)

    for i := 0; i < 100; i++ {
        wg.Add(1)
        go func(idx int) {
            defer wg.Done()
            results[idx] = resp.IDOrNil()
        }(i)
    }
    wg.Wait()

    // All goroutines should see the same result
    for i := 1; i < len(results); i++ {
        assert.Equal(t, results[0], results[i],
            "All concurrent IDOrNil calls should return the same value")
    }
    assert.Equal(t, "test-id", results[0])
}
```

---

### Phase 13: Update Documentation

**13.1 Add migration notes to IMPROVEMENTS.md**

Add a section documenting the breaking change:

```markdown
### Response Immutability (v2.0 Breaking Change)

**Changed**: The `Response` type is now truly immutable after decoding.

**Before (v1.x)**:
- Three `sync.RWMutex` fields for thread-safety
- Fields could theoretically be modified (though not recommended)

**After (v2.0)**:
- Uses `sync.Once` for one-time lazy initialization
- Fields are strictly immutable after decode
- ~66% reduction in synchronization overhead
- Clearer API contract

**Migration**:
- No changes needed if you were only reading responses
- If you were modifying response fields after decode (unlikely), you must create new Response instances instead

**Example**:
```go
// v1.x (bad practice, but worked)
resp, _ := jsonrpc.DecodeResponse(data)
resp.ID = "new-id"  // Worked but not recommended

// v2.0 (correct way)
resp, _ := jsonrpc.DecodeResponse(data)
// resp.ID = "new-id"  // Won't work - use constructor instead
newResp, _ := jsonrpc.NewResponse("new-id", result)
```
```

**13.2 Update response.go godoc**

Ensure all public methods document immutability:

```go
// IDOrNil returns the unmarshaled ID, or nil if unmarshaling fails.
// The ID is unmarshaled lazily on first call and cached for subsequent calls.
// This method is safe for concurrent use.
func (r *Response) IDOrNil() any { /* ... */ }

// UnmarshalError unmarshals the raw error into the Error field.
// The error is unmarshaled lazily on first call and cached for subsequent calls.
// This method is safe for concurrent use.
func (r *Response) UnmarshalError() error { /* ... */ }
```

---

## Testing Strategy

### Pre-Implementation Tests

**Run existing test suite**:
```bash
go test ./... -v -race
```

Expected: All tests pass with current implementation.

### During Implementation

After each phase, run:
```bash
go test ./... -v -race -count=100
```

The `-race` flag detects race conditions.
The `-count=100` runs tests 100 times to catch intermittent issues.

### Post-Implementation Verification

**1. Race detection**:
```bash
go test ./... -race -count=1000
```

**2. Benchmark comparison**:

Create benchmark to compare old vs new:
```go
// benchmark_test.go
func BenchmarkResponseConcurrentReads_Old(b *testing.B) {
    // Benchmark with mutex-based implementation
}

func BenchmarkResponseConcurrentReads_New(b *testing.B) {
    // Benchmark with sync.Once implementation
}
```

Run:
```bash
go test -bench=BenchmarkResponseConcurrentReads -benchmem
```

**Expected improvement**: ~30-50% reduction in ns/op for concurrent reads.

**3. Coverage verification**:
```bash
go test ./... -cover
```

Expected: Maintain or improve current 87%+ coverage.

---

## Rollback Plan

If issues arise during implementation:

1. **Create backup branch**: `git checkout -b immutability-rollback`
2. **Revert changes**: `git revert <commit-range>`
3. **Document issues**: Add findings to this document under "Issues Encountered"
4. **Re-evaluate approach**: Consider hybrid options or phased rollout

---

## Success Criteria

- ✅ All existing tests pass
- ✅ No race conditions detected with `-race` flag
- ✅ Concurrent reads are 30%+ faster in benchmarks
- ✅ Memory usage per Response reduced by ~24 bytes
- ✅ Code is simpler (fewer lock/unlock calls)
- ✅ API contract is clearer (documented immutability)

---

## Timeline Estimate

**Total Effort**: ~4-6 hours

- **Phase 1-2** (Struct + unmarshalID): 30 minutes
- **Phase 3-11** (Method updates): 2-3 hours
- **Phase 12** (Testing): 1-2 hours
- **Phase 13** (Documentation): 30 minutes
- **Verification & benchmarking**: 30-60 minutes

**Recommended Schedule**:
1. Day 1: Phases 1-5 (struct updates + core methods)
2. Day 2: Phases 6-11 (remaining methods)
3. Day 3: Phases 12-13 (testing + docs)

---

## Open Questions

1. **Should we keep deprecated IDRaw() wrapper?**
   - Current: `IDRaw()` calls `IDOrNil()`
   - Decision: Keep for v2.0, remove in v3.0

2. **Error handling in sync.Once closures?**
   - `sync.Once` doesn't propagate errors
   - Solution: Capture error in closure-scoped variable (see Phase 4)

3. **Should parseFromBytes check if Response was already populated?**
   - Current: Always overwrites fields
   - Decision: Keep current behavior - decoding twice is caller error

---

## Related Work

This change pairs well with:
- Batch request/response support (IMPROVEMENTS.md)
- Additional constructor helpers
- V2.0 API consolidation

Consider implementing as part of unified v2.0 release.
