# Go Idiomacy / Repo Structure - Findings and Recommendations

## 1. Mutable Response Fields

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

**Temporary Mitigation:** The current godoc warning is sufficient for now. Most users treat the struct as immutable already.
