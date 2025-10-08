package jsonrpc

import (
	"errors"
	"fmt"

	"github.com/bytedance/sonic/ast"
)

// PeekStringByPath traverses the result JSON using sonic's AST to extract a string field without
// unmarshaling the entire result. This is valuable for large responses where you only need to
// access specific nested fields.
//
// The path is specified as a sequence of keys for nested objects. An example using multiple
// arguments:
//
//	from, err := response.PeekStringByPath("transaction", "from")
//
// The AST node is lazily built on first call and cached for subsequent calls, making repeated
// field access very efficient.
func (r *Response) PeekStringByPath(path ...any) (string, error) {
	node, err := r.getASTNode()
	if err != nil {
		return "", err
	}

	// Navigate to the requested path
	if len(path) > 0 {
		targetNode := node.GetByPath(path...)
		if targetNode == nil || !targetNode.Valid() {
			return "", errors.New("path not found")
		}
		node = *targetNode
	}

	// Extract string value
	str, err := node.String()
	if err != nil {
		return "", fmt.Errorf("value at path is not a string: %w", err)
	}

	return str, nil
}

// PeekBytesByPath returns raw JSON bytes for a nested field without unmarshaling the entire result.
// This is useful when you want to extract a sub-object or array and unmarshal it separately.
//
// Example usage to extract a nested transaction object:
//
//	txBytes, err := response.PeekBytesByPath("transaction")
//
// The AST node is lazily built on first call and cached for subsequent calls, making repeated
// field access very efficient.
func (r *Response) PeekBytesByPath(path ...any) ([]byte, error) {
	node, err := r.getASTNode()
	if err != nil {
		return nil, err
	}

	// Navigate to the requested path
	if len(path) > 0 {
		targetNode := node.GetByPath(path...)
		if targetNode == nil || !targetNode.Valid() {
			return nil, errors.New("path not found")
		}
		node = *targetNode
	}

	// Get raw JSON bytes
	raw, err := node.Raw()
	if err != nil {
		return nil, fmt.Errorf("failed to get raw bytes: %w", err)
	}

	return []byte(raw), nil
}

// buildASTNode lazily builds the AST node for the result field.
func (r *Response) buildASTNode() {
	if len(r.result) == 0 {
		r.astErr = errors.New("response has no result field")
		return
	}

	// Parse the result field into an AST node
	// The result field has already been validated during decode
	node, err := ast.NewSearcher(string(r.result)).GetByPath()
	if err != nil {
		r.astErr = fmt.Errorf("failed to build AST node: %w", err)
		return
	}
	r.astNode = node
}

// getASTNode returns the cached AST node, building it if necessary.
func (r *Response) getASTNode() (ast.Node, error) {
	r.astOnce.Do(r.buildASTNode)

	r.astMutex.RLock()
	defer r.astMutex.RUnlock()

	if r.astErr != nil {
		return ast.Node{}, r.astErr
	}

	return r.astNode, nil
}
