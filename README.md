# go-jsonrpc [![Go Documentation](http://img.shields.io/badge/go-documentation-blue.svg?style=flat-square)][godocs] [![MIT License](http://img.shields.io/badge/license-MIT-blue.svg?style=flat-square)][license]

[godocs]: http://godoc.org/github.com/jkbrsn/go-jsonrpc
[license]: /LICENSE

A JSON-RPC 2.0 implementation in Go.

Utilizes the [bytedance/sonic](https://github.com/bytedance/sonic) library for JSON serialization.

Attempts to conform fully to the [JSON-RPC 2.0 Specification](https://www.jsonrpc.org/specification), with a few minor exceptions:

- The `id` field is allowed to be fractional numbers, in addition to integers and strings. The specification notes that "numbers should not contain fractional parts", but this library allows them for convenience.
- Error
  - Error codes are restricted to non-zero integers. The specification allows for any integer, but this library restricts them to non-zero integers to avoid confusion with the nil integer value.
  - The `error` field is otherwise quite flexible in this library, as basically any will be marshalled into a valid `Error` struct. This is to allow for custom error handling downstream.


## Install

```bash
go get github.com/jkbrsn/go-jsonrpc
```

## Contributing

For contributions, please open a GitHub issue with your questions and suggestions. Before submitting an issue, have a look at the existing [TODO list](TODO.md) to see if your idea is already in the works.