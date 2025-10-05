# Potential Improvements

This document tracks potential improvements gleaned from analyzing similar JSON-RPC libraries, specifically focusing on features that would enhance performance, usability, and production readiness.

## Priority Ranking

1. **WriteTo (Streaming Serialization)** - Immediate performance win
2. **AST-based PeekByPath** - Killer feature for large responses
3. **Clone** - Common need, well-scoped
4. **Size** - Simple utility
5. **Free (Memory Management)** - Critical for production use
6. **ID Byte Caching** - Micro-optimization

---

## High-Value Additions

### 1. WriteTo Method for Streaming Serialization (Priority: 1)

**What:**
```go
// WriteTo implements io.WriterTo for efficient streaming without buffering entire response
func (r *Response) WriteTo(w io.Writer) (n int64, err error)
```

**Why:**
- Avoids allocating memory for the entire marshaled response
- Current `MarshalJSON` allocates a full buffer before writing
- For large responses (e.g., getLogs with 10k+ events), this significantly reduces memory pressure
- Enables efficient HTTP response writing without intermediate buffers

**Implementation Notes:**
- Manually write JSON fields: `{"jsonrpc":"2.0","id":`
- Stream ID, error, or result directly without buffering
- Mutex protection same as current implementation
- Reference: Other library lines 152-201

**Example Use Case:**
```go
var buf bytes.Buffer
response.WriteTo(&buf)
// or directly to http.ResponseWriter
response.WriteTo(w)
```

---

### 2. AST-Based Field Access (Priority: 2)

**What:**
```go
// PeekStringByPath traverses the result JSON using sonic's AST to extract a field
// without unmarshaling the entire result
func (r *Response) PeekStringByPath(path ...interface{}) (string, error)

// PeekBytesByPath returns raw JSON bytes for a nested field
func (r *Response) PeekBytesByPath(path ...interface{}) ([]byte, error)
```

**Why:**
- Extract specific fields from large result payloads without full unmarshaling
- Example: Get `blockNumber` from an `eth_getBlockByNumber` response without deserializing the entire block
- Uses `bytedance/sonic/ast` for zero-copy JSON traversal
- **Extremely valuable** for large responses

**Implementation Notes:**
- Lazily build AST node on first call, cache it
- Use `ast.NewSearcher` with `ValidateJSON: false` for performance
- Thread-safe via mutex on cached node
- Reference: Other library lines 196-221, 239-266

**Example Use Case:**
```go
// Extract blockNumber from {"result": {"blockNumber": "0x1234", ...}}
blockNum, err := response.PeekStringByPath("blockNumber")

// For nested paths: response.PeekStringByPath("transaction", "from")
```

---

### 3. Clone Method with Deep Copying (Priority: 3)

**What:**
```go
// Clone creates a deep copy of the response, ensuring no shared references
func (r *Response) Clone() (*Response, error)
```

**Why:**
- Your library is immutable by design, but cloning is useful when deriving new responses
- Critical to avoid shared byte slice references between clones
- Useful for middleware that needs to modify responses without affecting original

**Implementation Notes:**
- Deep copy all byte slices (`rawID`, `rawError`, `result`)
- Copy parsed values (`id`, `err`)
- Do NOT copy cached AST nodes (rebuild on demand to avoid retaining large buffers)
- Reference: Other library lines 203-252

**Example Use Case:**
```go
original := NewResponse(id, result)
modified, _ := original.Clone()
// Modify 'modified' without affecting 'original'
```

---

### 4. Size Calculation (Priority: 4)

**What:**
```go
// Size returns the approximate serialized size of the response in bytes
func (r *Response) Size() int
```

**Why:**
- Useful for metrics and logging
- Helps decide whether to buffer or stream
- Simple to implement

**Implementation Notes:**
- Sum of: ID bytes + error bytes + result bytes
- Account for JSON structure overhead (~20 bytes)
- Reference: Other library lines 137-151

**Example Use Case:**
```go
if response.Size() > 1024*1024 {
    log.Warn("Large response detected", "size", response.Size())
}
```

---

### 5. Memory Management (Priority: 5)

**What:**
```go
// Free releases heavy memory-retaining fields after response is consumed
func (r *Response) Free()
```

**Why:**
- Explicitly releases byte slices (`rawID`, `rawError`, `result`) after consumption
- Critical for long-running services to prevent memory leaks from retained buffers
- Particularly important when parsing from pooled buffers

**Implementation Notes:**
- Nil out all byte slices
- Keep small parsed values (`id`, `err`) for logging
- Release cached AST nodes
- Mark as unsafe for concurrent use after calling Free
- Reference: Other library lines 58-89

**Example Use Case:**
```go
response, _ := DecodeResponse(data)
// Use response
json.NewEncoder(w).Encode(response)
// Explicitly free when done
response.Free()
```

---

### 6. ID Byte Caching (Priority: 6)

**What:**
- Store both `id` (parsed) and `idBytes` (raw) to avoid re-marshaling

**Why:**
- Current `MarshalJSON` (response.go:318-325) re-marshals ID each time
- Caching the bytes improves performance for repeated marshaling
- The other library does this with mutex-protected dual storage

**Implementation Notes:**
- Add `idBytes []byte` field
- Populate during unmarshal or when ID is set
- Use in `MarshalJSON` and `WriteTo` (if implemented)
- Reference: Other library lines 13-20, 155-178

**Impact:**
- Micro-optimization, but low-hanging fruit
- Most valuable when responses are marshaled multiple times (e.g., caching, retries)

---

## Code Quality Improvements

### 7. Better Stream Parsing with Buffer Pooling

**What:**
- Use buffer pooling in `DecodeResponseFromReader`
- Carefully copy only needed data to avoid retaining upstream buffers

**Why:**
- Your current implementation at response.go:71-78 uses `readAll` which may retain large buffers
- The other library uses buffer pools with explicit return calls
- Reduces GC pressure in high-throughput scenarios

**Implementation Notes:**
- Create `sync.Pool` for buffers
- `defer returnBuf()` immediately after reading
- Copy only parsed fields (ID, error, result) to response
- Reference: Other library lines 71-106

---

## Not Recommended

The following features from the other library are **not recommended** for your use case:

L **Canonical hashing with ignored fields** - Complex, niche use case specific to blockchain consensus
L **Zerolog integration** - Adds opinionated dependency, users can wrap if needed
L **ByteWriter interface** - Too complex for your scope, over-engineered
L **Extensive error translation** - Domain-specific to eRPC (upstream exhaustion, rate limiting)
L **Context tracing** - OpenTelemetry integration is application-level concern
L **Request-specific features** - Your `Request` type is already well-designed and simpler

---

## Implementation Sequence

If implementing these features, the recommended order is:

1. **Size** - Simplest, good warmup, useful immediately
2. **WriteTo** - High impact, moderate complexity, no dependencies
3. **Clone** - Well-scoped, no dependencies
4. **PeekByPath** - Requires understanding sonic/ast, highest value
5. **ID Byte Caching** - Micro-optimization, integrate with WriteTo
6. **Free** - Add after understanding memory patterns in production
7. **Buffer Pooling** - Performance optimization, measure first

---

## Sonic Performance Optimizations

### Background

The `bytedance/sonic` library offers two categories of optimizations:
1. **`sonic.Pretouch`** - Pre-compiles codecs at startup to eliminate JIT overhead
2. **Custom `sonic.Config`** - Fine-tunes encoding/decoding behavior with various performance/safety tradeoffs

These optimizations were observed in the reference library (`github.com/erpc/erpc/common`), which uses aggressive configuration for maximum performance.

### 8. sonic.Pretouch (Recommended) ✅

**What:**
```go
func init() {
    // Pre-compile codecs for hot-path types
    _ = sonic.Pretouch(reflect.TypeOf(Request{}))
    _ = sonic.Pretouch(reflect.TypeOf(Response{}))
    _ = sonic.Pretouch(reflect.TypeOf(Error{}))
}
```

**Why:**
- Eliminates JIT compilation overhead on first marshal/unmarshal call
- Reduces first-call latency from ~1-5ms to microseconds
- Better P99 latency (no unpredictable "warm-up" spike)
- Low cost: increases startup time by only ~10-50ms total

**Performance Impact:**
| Metric | Without Pretouch | With Pretouch | Improvement |
|--------|------------------|---------------|-------------|
| First call | ~1-5ms | ~10-50μs | **-80% to -99%** |
| Steady state | Same | Same | 0% |

**Recommendation:** **Implement this.** High value, low risk, standard practice for performance-sensitive libraries.

**Implementation Notes:**
- Call `sonic.Pretouch()` in `init()` for `Request`, `Response`, `Error`
- Use `option.WithCompileMaxInlineDepth(1)` to limit code bloat (your types are simple)
- Don't panic on error - log and continue (maintains robustness)

---

### 9. Custom sonic.Config (Consider Carefully) ⚠️

**What:**
The reference library uses a custom `sonic.Config` with aggressive optimizations:

```go
SonicCfg = sonic.Config{
    CopyString:              false,  // Zero-copy string->[]byte conversion
    NoNullSliceOrMap:        true,   // Omit null for empty slices/maps
    NoQuoteTextMarshaler:    true,   // Skip quotes for TextMarshaler types
    NoValidateJSONMarshaler: true,   // Skip validation in MarshalJSON
    NoValidateJSONSkip:      true,   // Skip validation for skipped fields
    EscapeHTML:              false,  // Don't escape HTML entities
    SortMapKeys:             false,  // Don't sort map keys (non-deterministic)
    CompactMarshaler:        true,   // No whitespace
    ValidateString:          false,  // Skip UTF-8 validation
}.Froze()
```

**Analysis of Each Setting:**

| Setting | Perf Gain | Risk | Verdict |
|---------|-----------|------|---------|
| `EscapeHTML: false` | +2-3% | **Low** - JSON-RPC doesn't contain HTML | ✅ Safe |
| `SortMapKeys: false` | +1-2% | **Low** - Determinism not required | ✅ Safe |
| `CompactMarshaler: true` | +1% | **None** - Just removes whitespace | ✅ Safe |
| `NoNullSliceOrMap: true` | +1% | **Low** - Cleaner JSON output | ✅ Safe |
| `CopyString: false` | +5% | **High** - Unsafe conversions, buffer lifecycle issues | ❌ Risky |
| `ValidateString: false` | +3-5% | **High** - Panics on invalid UTF-8 | ❌ Risky |
| `NoValidateJSONMarshaler: true` | +2% | **High** - Can produce invalid JSON | ❌ Risky |
| `NoQuoteTextMarshaler: true` | +1% | **Medium** - Niche optimization | ⚠️ Situational |
| `NoValidateJSONSkip: true` | <1% | **Low** - Minor edge case | ⚠️ Minor |

**Recommended Conservative Config:**
```go
// Conservative config suitable for a general-purpose library
var sonicCfg = sonic.Config{
    // Safe optimizations
    EscapeHTML:       false,  // JSON-RPC doesn't contain HTML
    SortMapKeys:      false,  // Determinism not required
    CompactMarshaler: true,   // No whitespace
    NoNullSliceOrMap: true,   // Cleaner JSON

    // Keep these defaults for safety
    CopyString:              true,   // Avoid unsafe conversions
    NoValidateJSONMarshaler: false,  // Validate JSON for robustness
    ValidateString:          true,   // Validate UTF-8
}.Froze()
```

**Performance vs. Safety Tradeoff:**

The aggressive settings can provide ~10-15% total performance improvement, but come with risks:
- **`CopyString: false`** - Can cause data corruption if buffers are reused
- **`ValidateString: false`** - Will panic on invalid UTF-8 instead of returning an error
- **`NoValidateJSONMarshaler: true`** - Can silently produce malformed JSON

**For a Library (Not a Service):**

Since this is a **general-purpose library** consumed by downstream users:
- Users expect **robustness and predictability** over raw speed
- You don't control the input sources (could be untrusted/malformed data)
- Silent failures or panics are worse than slightly slower processing

**However**, there is value in **allowing users to opt-in** to aggressive settings if they:
1. Control their input sources
2. Have profiled and identified JSON encoding as a bottleneck
3. Are willing to trade safety for speed

### Potential API for Configuration

**Option A: Global Config (Simple)**
```go
// SetSonicConfig allows users to provide a custom sonic.API for all operations
// Use this if you need maximum performance and control input validation externally.
// If not set, the library uses the default sonic behavior (safe).
func SetSonicConfig(cfg sonic.API) {
    sonicAPI = cfg
}

var sonicAPI sonic.API = sonic.ConfigDefault // Package-level default
```

**Option B: Per-Operation Config (Flexible)**
```go
// DecodeOptions allows customizing decode behavior
type DecodeOptions struct {
    SonicAPI sonic.API // If nil, uses default
}

func DecodeResponseWithOptions(data []byte, opts *DecodeOptions) (*Response, error)
```

**Option C: Pre-configured Profiles (User-Friendly)**
```go
// Performance profile constants
const (
    ProfileSafe       = iota // Default: validates everything
    ProfileBalanced          // Moderate: safe optimizations only
    ProfileAggressive        // Fast: disables validation (use with trusted sources)
)

func SetPerformanceProfile(profile int)
```

**Recommendation:**

1. **Implement `sonic.Pretouch` immediately** - No API changes needed, pure win
2. **Start with default sonic config** - Safe for all users
3. **Consider Option C (Profiles)** - Provides clear semantics:
   - `ProfileSafe` (default) - Current behavior
   - `ProfileBalanced` - Safe optimizations (EscapeHTML=false, etc.)
   - `ProfileAggressive` - User takes responsibility for validation
4. **Document tradeoffs clearly** - Users need to understand the risks

### Summary Table

| Optimization | Value | Risk | Recommendation |
|--------------|-------|------|----------------|
| `sonic.Pretouch` | High | Low | **✅ Implement now** |
| Safe config options | Low-Medium | Low | **✅ Implement as "Balanced" profile** |
| Aggressive config | Medium | High | **⚠️ Provide as opt-in "Aggressive" profile** |
| Custom API | High (for power users) | Medium | **✅ Add as optional config** |

**Implementation Priority:**
1. Add `sonic.Pretouch` in `init()` (Priority: High, Effort: Low)
2. Add performance profiles with clear documentation (Priority: Medium, Effort: Medium)
3. Benchmark and document actual performance differences (Priority: Low, Effort: High)

---

## References

- Source library: `github.com/erpc/erpc/common` (JSON-RPC response implementation)
- Sonic AST docs: https://github.com/bytedance/sonic/blob/main/ast/README.md
- Sonic configuration: https://github.com/bytedance/sonic#apis
- io.WriterTo interface: https://pkg.go.dev/io#WriterTo
