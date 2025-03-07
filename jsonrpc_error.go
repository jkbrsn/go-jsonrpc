package jsonrpc

// Error represents a standard JSON-RPC error.
type Error struct {
	Code    int    `json:"code,omitempty"`
	Message string `json:"message,omitempty"`

	// Errors might contain additional data, e.g. revert reason
	Data any `json:"data,omitempty"`
}
