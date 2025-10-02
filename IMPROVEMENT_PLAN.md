# Improvement Plan

This document outlines specific improvements identified during code review, organized by category with step-by-step implementation plans.

## UX / API Surface

### 1. Deprecated functions still present

**Issue:** Functions marked for v2.0 removal (`RequestFromBytes`, `NewResponseFromBytes`, `NewResponseFromStream`, `IDRaw`) lack clear migration guidance.

**Implementation steps:**
1. Add a `MIGRATION.md` document listing all deprecated functions with their replacements
2. Update godoc comments on deprecated functions to reference the migration guide
3. Add examples in README.md showing migration patterns (e.g., `RequestFromBytes` → `DecodeRequest`)

### 2. `IsEmpty()` semantics are fragile

**Issue:** `Response.IsEmpty()` (response.go:219-246) checks for specific byte patterns (`"0x"`, `null`, etc.) which is brittle and overly specific.

**Implementation steps:**
1. Document the rationale for `IsEmpty()` in godoc—explain what use case requires these specific checks
2. Add test cases covering each byte pattern check to prevent regression
3. Consider simplifying to just `len(Result) == 0 && Error.IsEmpty()` if the hex/null checks aren't critical
4. If keeping the implementation, extract pattern checks to named helper functions for clarity

### 3. Error type has ambiguous validation

**Issue:** error.go:106 allows zero error codes "for spec compliance" but error.go:51 treats zero codes as empty. CLAUDE.md states "Error codes must be non-zero" but code allows them.

**Implementation steps:**
1. Decide on policy: either disallow zero codes (per CLAUDE.md) or allow them (per spec)
2. If disallowing: update `Error.Validate()` to reject zero codes, update CLAUDE.md to note this deviation
3. If allowing: update CLAUDE.md to remove "must be non-zero" statement, clarify that zero is valid but considered "empty"
4. Update `Error.IsEmpty()` godoc to explicitly state that zero codes are treated as empty
5. Add test cases for zero-code errors in both validation and empty checks

### 4. `Params` and `Data` fields are `any`

**Issue:** While flexible, `any` types force manual type assertions without guidance.

**Implementation steps:**
1. Add a new file `examples_test.go` with `Example*` test functions showing common patterns:
   - Params as positional array (`[]any{1, 2, 3}`)
   - Params as named object (`map[string]any{"id": 123}`)
   - Unmarshaling Params using `sonic.Unmarshal`
2. Add a "Working with Params" section to README.md with these patterns
3. Consider adding helper methods like `Request.UnmarshalParams(dst any) error` to match `Response.UnmarshalResult`

## Parsing Performance

### 1. No benchmarks

**Issue:** 44 test functions but zero benchmarks. Cannot quantify Sonic's performance benefit.

**Implementation steps:**
1. Create benchmark file `bench_test.go`
2. Add benchmarks for hot paths:
   - `BenchmarkDecodeRequest` (small, medium, large payloads)
   - `BenchmarkDecodeResponse` (with/without large Result)
   - `BenchmarkDecodeBatchRequest` (1, 10, 100 items)
   - `BenchmarkDecodeBatchResponse` (1, 10, 100 items)
   - `BenchmarkRequestMarshal`
   - `BenchmarkResponseMarshal`
3. Run benchmarks and document baseline performance in README or docs
4. Add `make bench` target to simplify running benchmarks

### 2. Sonic everywhere except when it's not

**Issue:** Uses Sonic for most paths but `encoding/json` for `json.RawMessage` fields (response.go:279). Inconsistency is undocumented.

**Implementation steps:**
1. Audit all uses of `encoding/json` vs `sonic` in codebase
2. Add code comment above mixed usage explaining why (e.g., "RawMessage interop requires stdlib json")
3. Consider whether `sonic.RawMessage` could replace `json.RawMessage` for consistency
4. Document in CLAUDE.md when Sonic is used vs stdlib

### 3. `RandomJSONRPCID()` uses unseeded `rand`

**Issue:** utils.go:35 uses `math/rand` which is deterministic per-process and not cryptographically secure.

**Implementation steps:**
1. Replace `math/rand` with `crypto/rand` for unpredictable IDs:
   ```go
   import "crypto/rand"
   func RandomJSONRPCID() int64 {
       var b [8]byte
       rand.Read(b[:])
       return int64(binary.BigEndian.Uint64(b[:]) & 0x7FFFFFFF) // Keep in int31 range
   }
   ```
2. Or use `math/rand/v2` (available in Go 1.22+) which auto-seeds:
   ```go
   import "math/rand/v2"
   func RandomJSONRPCID() int64 {
       return int64(rand.IntN(2147483647))
   }
   ```
3. Update tests to verify ID uniqueness across multiple calls
4. Document in godoc that IDs are randomly generated (remove "32-bit range" if using crypto/rand)

### 4. `readAll` allocates 16KB upfront even for tiny messages

**Issue:** utils.go:46 pre-allocates 16KB buffer regardless of message size.

**Implementation steps:**
1. Change initial buffer size based on first read or expected size:
   ```go
   initialSize := 512 // Start small
   if expectedSize > 0 && expectedSize < upperSizeLimit {
       initialSize = expectedSize
   }
   buffer := bytes.NewBuffer(make([]byte, 0, initialSize))
   ```
2. Benchmark before/after to measure impact on small vs large messages
3. Document buffer sizing strategy in code comment

## Go Idiomacy / Repo Structure

### 1. Mutable fields on `Response` after decoding

**Issue:** response.go:17 claims "immutable after decoding" but exported fields can be mutated by users.

**Implementation steps:**
1. Choose approach:
   - **Option A (breaking):** Make fields unexported, add getters
   - **Option B (non-breaking):** Clarify in godoc that mutation after decode is unsupported
2. If choosing Option B:
   - Update godoc to state: "Do not modify Response fields after decoding; behavior is undefined"
   - Add note to CLAUDE.md about immutability contract
3. Add test that validates concurrent reads work correctly (already thread-safe for reads)

### 2. Inconsistent error wrapping

**Issue:** Some functions use `fmt.Errorf("... %w", err)`, others use `errors.New()` losing context.

**Implementation steps:**
1. Audit all error returns using: `rg 'return.*errors\.New|return.*fmt\.Errorf'`
2. Update all error returns to wrap underlying errors with `%w` where applicable
3. Ensure top-level functions add context (e.g., `fmt.Errorf("failed to decode request: %w", err)`)
4. Add linter rule (e.g., `wrapcheck` in golangci-lint) to prevent future regressions

### 3. `formatFloat64ID` logic is non-trivial

**Issue:** utils.go:14-31 handles spec deviation for fractional IDs without explanation.

**Implementation steps:**
1. Add detailed godoc comment to `formatFloat64ID`:
   ```go
   // formatFloat64ID formats a float64 ID as a string with minimal decimal places.
   // This supports the library's deviation from JSON-RPC 2.0 spec, which states
   // numbers "should not contain fractional parts". We allow fractional IDs for
   // flexibility. See CLAUDE.md for rationale.
   ```
2. Add reference link to JSON-RPC 2.0 spec section about IDs
3. Add test cases demonstrating formatting behavior (1.0 → "1.0", 1.5 → "1.5", 1.23000 → "1.23")

### 4. TODO comment in production code

**Issue:** response.go:159 has unaddressed TODO: "adapt to both raw (parsed) and unmarshalled cases, test that"

**Implementation steps:**
1. Investigate what the TODO refers to—does `Equals` fail when comparing parsed vs constructed responses?
2. Either:
   - Implement the fix: normalize `rawID`/`ID` and `rawError`/`Error` comparison
   - Or file GitHub issue documenting the limitation and remove inline TODO
3. Add test case for `Equals()` with mixed raw/unmarshaled responses
4. Update godoc if limitation exists

### 5. Mixed use of `sonic.Unmarshal` vs `json.Unmarshal`

**Issue:** Most code uses Sonic but stdlib is used in some places without explanation.

**Implementation steps:**
1. Add comment where stdlib `json` is used:
   ```go
   // Use json.RawMessage for stdlib compatibility when callers may use encoding/json
   ```
2. Verify Sonic's `json.RawMessage` interop—test whether `sonic.Unmarshal` → `json.Marshal` round-trips correctly
3. Document JSON library choice in CLAUDE.md under "JSON Handling" section

### 6. `Equals` methods don't compare `Data` field

**Issue:** error.go:29 skips comparing `Error.Data` without documentation.

**Implementation steps:**
1. Update `Error.Equals()` godoc to state: "Note: Data field is not compared due to its `any` type"
2. Consider adding `Error.EqualsStrict(other *Error) bool` that does deep comparison of Data
3. Document in CLAUDE.md that `Data` comparison is omitted for simplicity
4. Add test demonstrating that `Equals` returns true even when Data differs

## Additional Notes

The following items were noted but are not included in the implementation plan as they require external tools or are lower priority:

- **No linter output shown:** Run `golangci-lint run` to catch common issues
- **Missing examples in godoc:** Would benefit from `Example*` test functions showing client/server patterns (partially addressed in UX section #4)
- **Go version alignment:** CLAUDE.md mentions Go 1.24+ but go.mod specifies 1.25.1—align messaging for consistency

---

## Prioritization

If implementing incrementally, suggested priority order:

1. **Critical:** Add benchmarks (#2.1) and fix `RandomJSONRPCID()` (#2.3)
2. **High:** Address TODO comment (#3.4), fix error wrapping (#3.2), clarify error validation (#1.3)
3. **Medium:** Add documentation improvements (#1.1, #1.4, #3.3, #3.5, #3.6)
4. **Low:** Refine `IsEmpty()` (#1.2), optimize buffer allocation (#2.4), document Sonic usage (#2.2)
