# Go Idiomacy / Repo Structure - Findings and Recommendations

## 1. Mutable Response Fields (INVESTIGATION ONLY)

**Finding:** Response fields are currently exported (JSONRPC, ID, Error, Result), allowing users to mutate them after decoding despite documentation stating "Do not modify Response fields directly".

**Scope of Change:**
- Would require making all 4 fields unexported
- Adding getter methods: `Version()`, `ID()`, `Err()`, `RawResult()`
- Updating approximately 50+ test assertions across:
  - `batch_test.go`
  - `response_test.go`
  - `request_test.go`
  - `bench_test.go`
  - `examples_test.go`
- Breaking change affecting any external users

**Recommendation:** This should be implemented as a separate, dedicated PR given the scope. The change would enforce true immutability but requires:
1. Comprehensive test updates
2. Migration guide expansion
3. Version bump consideration (likely warrants v2.0)
4. Coordination with users for migration

**Temporary Mitigation:** The current godoc warning is sufficient for now. Most users treat the struct as immutable already.

---

**Status:** DEFERRED to future PR (too large for current session)

---

## 4. TODO Comment in Response.Equals() (INVESTIGATION ONLY)

**Finding:** response.go:159 contains TODO: "adapt to both raw (parsed) and unmarshalled cases, test that"

**Current Behavior:**
- `Equals()` compares `r.ID` vs `other.ID` directly
- If one Response has lazy-loaded fields (rawID not yet unmarshaled to ID) and another has eagerly loaded fields, comparison may fail incorrectly
- Same issue applies to `Error` field comparison

**Example Scenario:**
```go
resp1, _ := DecodeResponse(data)  // ID eagerly unmarshaled
resp2 := &Response{rawID: [...]}  // ID still in raw form
resp1.Equals(resp2) // May incorrectly return false even if IDs are semantically equal
```

**Recommendation:**
1. Call `unmarshalID()` on both responses before comparing IDs
2. Call `UnmarshalError()` on both responses before comparing errors
3. Add test case for mixed parsed/unparsed response comparison

**Priority:** Low - this is an edge case that rarely occurs in practice since most Responses are created via DecodeResponse()

**Status:** DOCUMENTED for future improvement
