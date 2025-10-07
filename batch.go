package jsonrpc

import (
	"bytes"
	// Used for json.RawMessage type, which provides interop with stdlib encoding/json
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// isBatchJSON returns true if the trimmed data starts with '[', indicating a JSON array.
func isBatchJSON(data []byte) bool {
	trimmed := bytes.TrimSpace(data)
	return len(trimmed) > 0 && trimmed[0] == '['
}

// DecodeRequestOrBatch attempts to parse either a single request or a batch of requests.
// - For single requests: returns slice with one element
// - For batch requests: returns slice with multiple elements
func DecodeRequestOrBatch(data []byte) (reqs []*Request, isBatch bool, err error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, false, errors.New(errEmptyData)
	}

	if isBatchJSON(data) {
		reqs, err := DecodeBatchRequest(data)
		return reqs, true, err
	}

	req, err := DecodeRequest(data)
	if err != nil {
		return nil, false, err
	}
	return []*Request{req}, false, nil
}

// DecodeResponseOrBatch attempts to parse either a single response or a batch of responses.
// - For single responses: returns slice with one element
// - For batch responses: returns slice with multiple elements
func DecodeResponseOrBatch(data []byte) (resps []*Response, isBatch bool, err error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, false, errors.New(errEmptyData)
	}

	if isBatchJSON(data) {
		resps, err := DecodeBatchResponse(data)
		return resps, true, err
	}

	resp, err := DecodeResponse(data)
	if err != nil {
		return nil, false, err
	}
	return []*Response{resp}, false, nil
}

// DecodeBatchRequest parses a JSON-RPC batch request from a byte slice.
// Returns an error if:
// - Input is not a JSON array
// - Array is empty
// - Any element fails to parse as a valid Request
func DecodeBatchRequest(data []byte) ([]*Request, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, errors.New(errEmptyData)
	}

	// Unmarshal as array of raw messages
	var rawMessages []json.RawMessage
	if err := getSonicAPI().Unmarshal(data, &rawMessages); err != nil {
		return nil, fmt.Errorf("invalid batch format: %w", err)
	}

	// JSON-RPC 2.0 spec requires non-empty batches
	if len(rawMessages) == 0 {
		return nil, errors.New("batch request must contain at least one request")
	}

	// Parse each request
	requests := make([]*Request, 0, len(rawMessages))
	for i, raw := range rawMessages {
		req, err := DecodeRequest(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid request at index %d: %w", i, err)
		}
		requests = append(requests, req)
	}

	return requests, nil
}

// EncodeBatchRequest marshals a slice of JSON-RPC requests into a batch.
// Returns an error if:
// - Input slice is empty
// - Any request fails validation
func EncodeBatchRequest(reqs []*Request) ([]byte, error) {
	if len(reqs) == 0 {
		return nil, errors.New("batch request must contain at least one request")
	}

	// Validate all requests first
	for i, req := range reqs {
		if err := req.Validate(); err != nil {
			return nil, fmt.Errorf("invalid request at index %d: %w", i, err)
		}
	}

	// Marshal as array
	return getSonicAPI().Marshal(reqs)
}

// DecodeBatchResponse parses a JSON-RPC batch response from a byte slice.
// Returns an error if:
// - Input is not a JSON array
// - Array is empty
// - Any element fails to parse as a valid Response
func DecodeBatchResponse(data []byte) ([]*Response, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, errors.New(errEmptyData)
	}

	// Unmarshal as array of raw messages
	var rawMessages []json.RawMessage
	if err := getSonicAPI().Unmarshal(data, &rawMessages); err != nil {
		return nil, fmt.Errorf("invalid batch format: %w", err)
	}

	if len(rawMessages) == 0 {
		return nil, errors.New("batch response must contain at least one response")
	}

	// Parse each response
	responses := make([]*Response, 0, len(rawMessages))
	for i, raw := range rawMessages {
		resp, err := DecodeResponse(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid response at index %d: %w", i, err)
		}
		responses = append(responses, resp)
	}

	return responses, nil
}

// EncodeBatchResponse marshals a slice of JSON-RPC responses into a batch.
// Returns an error if:
// - Input slice is empty
// - Any response fails validation
func EncodeBatchResponse(resps []*Response) ([]byte, error) {
	if len(resps) == 0 {
		return nil, errors.New("batch response must contain at least one response")
	}

	// Validate all responses first
	for i, resp := range resps {
		if err := resp.Validate(); err != nil {
			return nil, fmt.Errorf("invalid response at index %d: %w", i, err)
		}
	}

	// Marshal as array
	return getSonicAPI().Marshal(resps)
}

// DecodeBatchRequestFromReader parses a JSON-RPC batch request from an io.Reader.
func DecodeBatchRequestFromReader(r io.Reader, expectedSize int) ([]*Request, error) {
	if r == nil {
		return nil, errors.New("cannot read from nil reader")
	}

	chunkSize := defaultChunkSize
	data, err := readAll(r, int64(chunkSize), expectedSize)
	if err != nil {
		return nil, fmt.Errorf("failed to read batch request: %w", err)
	}

	return DecodeBatchRequest(data)
}

// DecodeBatchResponseFromReader parses a JSON-RPC batch response from an io.Reader.
func DecodeBatchResponseFromReader(r io.Reader, expectedSize int) ([]*Response, error) {
	if r == nil {
		return nil, errors.New("cannot read from nil reader")
	}

	chunkSize := defaultChunkSize
	data, err := readAll(r, int64(chunkSize), expectedSize)
	if err != nil {
		return nil, fmt.Errorf("failed to read batch response: %w", err)
	}

	return DecodeBatchResponse(data)
}

// NewBatchRequest creates a batch of JSON-RPC requests from methods and params.
// Each request receives an auto-generated ID.
func NewBatchRequest(methods []string, params []any) ([]*Request, error) {
	if len(methods) == 0 {
		return nil, errors.New("batch must contain at least one method")
	}
	if len(params) > 0 && len(params) != len(methods) {
		return nil, errors.New("params length must match methods length or be empty")
	}

	requests := make([]*Request, len(methods))
	for i, method := range methods {
		var p any
		if i < len(params) {
			p = params[i]
		}
		requests[i] = NewRequest(method, p)
	}

	return requests, nil
}

// NewBatchNotification creates a batch of notifications (requests without IDs).
func NewBatchNotification(methods []string, params []any) ([]*Request, error) {
	if len(methods) == 0 {
		return nil, errors.New("batch must contain at least one method")
	}
	if len(params) > 0 && len(params) != len(methods) {
		return nil, errors.New("params length must match methods length or be empty")
	}

	requests := make([]*Request, len(methods))
	for i, method := range methods {
		var p any
		if i < len(params) {
			p = params[i]
		}
		requests[i] = NewNotification(method, p)
	}

	return requests, nil
}
