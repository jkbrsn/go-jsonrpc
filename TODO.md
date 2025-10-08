# todo

## next minor

- Support for choice of JSON parser
  - Removes hard dependency on `sonic`, e.g. allows for using `encoding/json` instead of `sonic`
  - Standard pattern in Go libraries (e.g., go-redis, gocql)
  - Benchmarks will be able to justify using `sonic` as default

## future considerations

- Add strict (current implementation) vs non-strict (more lax on types, required fields) mode
- Add ability to use custom fields with Request, Response, and Error
  - This would enable more real-world RPC patterns, e.g. tracing IDs, request metadata, custom error context
  - Compliant with JSON-RPC 2.0 spec
- Enforce that error codes fall within the standard reserved ranges
