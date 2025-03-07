# go-jsonrpc [![Go Documentation](http://img.shields.io/badge/go-documentation-blue.svg?style=flat-square)][godocs] [![MIT License](http://img.shields.io/badge/license-MIT-blue.svg?style=flat-square)][license]

[godocs]: http://godoc.org/github.com/jakobilobi/go-jsonrpc
[license]: /LICENSE

A JSON-RPC 2.0 implementation in Go. It attempts to conform fully to the [JSON-RPC 2.0 Specification](https://www.jsonrpc.org/specification), with a few minor exceptions:

- The `id` field is allowed to be fractional numbers, in addition to integers and strings. The specification notes that "numbers should not contain fractional parts", but this library allows them for convenience.

Utilizes the [bytedance/sonic](https://github.com/bytedance/sonic) library for JSON serialization.

## Install

```bash
go get github.com/jakobilobi/go-jsonrpc
```
