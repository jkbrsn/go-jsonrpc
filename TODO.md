# todo

## next minor

- Consider functions to unmarshal individual fields of Response (and Request?)
- Add dynamic options
  - Skip forcing "jsonrpc: 2.0" (for either Request or Response or both)

## future considerations

- A “parse then make immutable” pattern for Request, Response, and Error
- The ability to add custom fields to Request, Response, and Error
  - This would allow for flexible usage with custom API:s
- Enforce that error codes fall within the standard reserved ranges