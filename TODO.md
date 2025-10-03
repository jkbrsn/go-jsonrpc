# todo

## next minor

- Support for choice of JSON parser
  - E.g. allow for using `encoding/json` instead of `sonic`
  - Benchmark and compare performance
- Add dynamic options
  - Skip forcing "jsonrpc: 2.0" (for either Request or Response or both)

## future considerations

- Consider functions to unmarshal individual fields of Response (and Request?)
- The ability to add custom fields to Request, Response, and Error
  - This would allow for flexible usage with custom API:s
- Enforce that error codes fall within the standard reserved ranges
