# Migration Guide

This document provides guidance for migrating from deprecated functions to their current replacements.

## v1.x → v2.0

The following functions are deprecated and will be removed in v2.0. Update your code to use the recommended replacements.

### Request Functions

#### `RequestFromBytes` → `DecodeRequest`

**Before:**
```go
req, err := jsonrpc.RequestFromBytes(data)
```

**After:**
```go
req, err := jsonrpc.DecodeRequest(data)
```

**Rationale:** The `Decode*` naming pattern is more consistent with Go conventions and clearly indicates parsing from bytes.

### Response Functions

#### `NewResponseFromBytes` → `DecodeResponse`

**Before:**
```go
resp, err := jsonrpc.NewResponseFromBytes(data)
```

**After:**
```go
resp, err := jsonrpc.DecodeResponse(data)
```

**Rationale:** `New*` constructors typically create fresh instances, while `Decode*` better conveys parsing existing data.

#### `NewResponseFromStream` → `DecodeResponseFromReader`

**Before:**
```go
resp, err := jsonrpc.NewResponseFromStream(body, expectedSize)
```

**After:**
```go
resp, err := jsonrpc.DecodeResponseFromReader(body, expectedSize)
```

**Note:** The new function does NOT automatically close the reader. If you need the old behavior:

```go
defer body.Close()
resp, err := jsonrpc.DecodeResponseFromReader(body, expectedSize)
```

**Rationale:** Automatic resource cleanup violates the principle that the caller should manage resources. The new API is explicit about lifecycle management.

#### `IDRaw` → `IDOrNil`

**Before:**
```go
id := resp.IDRaw()
```

**After:**
```go
id := resp.IDOrNil()
```

**Rationale:** `IDOrNil` more clearly expresses the return semantics—it returns the ID value or nil if unmarshaling fails.

## Quick Reference Table

| Deprecated Function | Replacement | Breaking Changes |
|---------------------|-------------|------------------|
| `RequestFromBytes(data)` | `DecodeRequest(data)` | None |
| `NewResponseFromBytes(data)` | `DecodeResponse(data)` | None |
| `NewResponseFromStream(body, size)` | `DecodeResponseFromReader(body, size)` | Does not auto-close reader |
| `resp.IDRaw()` | `resp.IDOrNil()` | None |

## Migration Checklist

- [ ] Search codebase for `RequestFromBytes` and replace with `DecodeRequest`
- [ ] Search codebase for `NewResponseFromBytes` and replace with `DecodeResponse`
- [ ] Search codebase for `NewResponseFromStream` and replace with `DecodeResponseFromReader`
- [ ] Add `defer body.Close()` where `NewResponseFromStream` was used
- [ ] Search codebase for `.IDRaw()` and replace with `.IDOrNil()`
- [ ] Run tests to verify migration: `go test ./...`

## Need Help?

If you encounter migration issues, please open an issue at https://github.com/jkbrsn/go-jsonrpc/issues
