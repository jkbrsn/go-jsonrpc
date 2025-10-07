//go:build !nopretouch

package jsonrpc

import (
	"reflect"

	"github.com/bytedance/sonic"
	"github.com/bytedance/sonic/option"
)

// init pre-compiles sonic codecs for hot-path types to eliminate JIT overhead. This improves
// first-call latency from ~1-5ms to ~10-50μs at the cost of a small increase in startup time.
//
// This optimization is enabled by default. To disable it (e.g., for CLI tools or single-shot
// programs), build with: go build -tags nopretouch
func init() {
	// Pre-compile codecs for the three core JSON-RPC types
	types := []reflect.Type{
		reflect.TypeOf(Request{}),
		reflect.TypeOf(Response{}),
		reflect.TypeOf(Error{}),
	}

	// Use WithCompileMaxInlineDepth(1) to limit code bloat since these types are simple
	opts := []option.CompileOption{
		option.WithCompileMaxInlineDepth(1),
	}

	for _, typ := range types {
		// Fail silently if pretouch fails
		// The library will still work, just with slightly higher first-call latency
		_ = sonic.Pretouch(typ, opts...)
	}
}
