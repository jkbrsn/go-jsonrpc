package jsonrpc

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// responseParseFormat is the wire format for parsing JSON-RPC responses.
type responseParseFormat struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   json.RawMessage `json:"error,omitempty"`
}

// responseMarshalFormat is the wire format for marshaling JSON-RPC responses.
type responseMarshalFormat struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Error   *Error          `json:"error,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
}

// MarshalJSON serializes the Response into a JSON-RPC 2.0 compliant byte slice.
//
// The method prioritizes parsed fields (id, err) over their raw counterparts (rawID, rawError)
// when both are present. If only raw fields exist, they are used directly to avoid unnecessary
// re-marshaling.
func (r *Response) MarshalJSON() ([]byte, error) {
	err := r.Validate()
	if err != nil {
		return nil, err
	}

	var id any
	if len(r.rawID) > 0 {
		id = r.rawID
	} else if r.id != nil {
		id = r.id
	} else {
		id = nil
	}

	if len(r.rawError) > 0 && r.err == nil {
		r.err = &Error{}
		if err := r.err.UnmarshalJSON(r.rawError); err != nil {
			return nil, fmt.Errorf("failed to unmarshal JSON-RPC error: %w", err)
		}
	}
	errVal := r.err

	var result json.RawMessage
	if len(r.result) > 0 {
		result = json.RawMessage(r.result)
	}

	output := responseMarshalFormat{
		JSONRPC: r.jsonrpc,
		ID:      id,
		Error:   errVal,
		Result:  result,
	}

	marshaled, err := getSonicAPI().Marshal(output)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal JSON-RPC response: %w", err)
	}

	return marshaled, nil
}

// UnmarshalJSON deserializes JSON-RPC 2.0 response data into the Response.
// Returns an error if the data is invalid JSON, has an incorrect JSON-RPC version, contains
// both result and error fields, or contains neither.
func (r *Response) UnmarshalJSON(data []byte) error {
	if err := r.parseFromBytes(data); err != nil {
		return fmt.Errorf("failed to unmarshal JSON-RPC response: %w", err)
	}

	// If the response carries an error (and no result), decode it eagerly so callers
	// can inspect *Response.Error without an extra step.
	if len(r.result) == 0 {
		if r.err == nil && len(r.rawError) > 0 {
			r.err = &Error{}
			if err := r.err.UnmarshalJSON(r.rawError); err != nil {
				return fmt.Errorf("failed to unmarshal JSON-RPC error: %w", err)
			}
		}
	}

	return nil
}

// WriteTo implements io.WriterTo for efficient streaming serialization without buffering
// the entire response in memory. This is beneficial for large responses as it significantly
// reduces memory pressure.
func (r *Response) WriteTo(w io.Writer) (n int64, err error) {
	if err := r.Validate(); err != nil {
		return 0, err
	}

	var total int64

	if err = writeString(w, `{"jsonrpc":"2.0","id":`, &total); err != nil {
		return total, err
	}

	idBytes, err := r.getIDBytes()
	if err != nil {
		return total, err
	}
	if err = writeBytes(w, idBytes, &total); err != nil {
		return total, err
	}

	if r.err != nil || len(r.rawError) > 0 {
		if err = r.writeErrorField(w, &total); err != nil {
			return total, err
		}
	} else {
		if err = r.writeResultField(w, &total); err != nil {
			return total, err
		}
	}

	if err = writeString(w, `}`, &total); err != nil {
		return total, err
	}

	return total, nil
}

// UnmarshalResult deserializes the raw Result field into the provided destination.
func (r *Response) UnmarshalResult(dst any) error {
	if dst == nil {
		return errors.New("destination pointer cannot be nil")
	}

	if len(r.result) == 0 {
		return errors.New("response has no result field")
	}

	return getSonicAPI().Unmarshal(r.result, dst)
}

// Unmarshal deserializes the entire Response into a custom struct.
//
// Note: It performs a marshal-then-unmarshal round trip, making it less efficient than using
// individual getter methods (IDOrNil(), Err(), RawResult(), UnmarshalResult()).
func (r *Response) Unmarshal(dst any) error {
	if dst == nil {
		return errors.New("destination pointer cannot be nil")
	}

	data, err := r.MarshalJSON()
	if err != nil {
		return err
	}

	return getSonicAPI().Unmarshal(data, dst)
}

// UnmarshalError deserializes the raw error bytes into the err field of the Response.
func (r *Response) UnmarshalError() error {
	var unmarshalErr error

	r.errOnce.Do(func() {
		if r.err == nil && len(r.rawError) > 0 {
			r.err = &Error{}
			unmarshalErr = r.err.UnmarshalJSON(r.rawError)
		}
	})

	return unmarshalErr
}

// parseFromBytes parses a JSON-RPC response from a byte slice. This function does not unmarshal
// the []byte data of the error or the result, it only stores the raw slices in the Response, to
// allow for any unmarshalling to occur at the caller's discretion.
func (r *Response) parseFromBytes(data []byte) error {
	var aux responseParseFormat
	if err := getSonicAPI().Unmarshal(data, &aux); err != nil {
		return err
	}

	if aux.JSONRPC != jsonRPCVersion {
		return fmt.Errorf("invalid JSON-RPC version: %s", aux.JSONRPC)
	}
	r.jsonrpc = aux.JSONRPC

	// Validate that either result or error is present
	resultExists := len(aux.Result) > 0
	errorExists := len(aux.Error) > 0

	if !resultExists && !errorExists {
		return errors.New("response must contain either result or error")
	}
	if resultExists && errorExists {
		return errors.New("response must not contain both result and error")
	}

	// Parse and unmarshal the ID field
	r.rawID = aux.ID
	if err := r.unmarshalID(); err != nil {
		return fmt.Errorf("failed to unmarshal ID: %w", err)
	}

	// Assign result or error accordingly
	if aux.Result != nil {
		r.result = aux.Result
	} else {
		r.rawError = aux.Error
	}

	if err := r.Validate(); err != nil {
		return fmt.Errorf("failed to parse JSON-RPC response: %w", err)
	}

	return nil
}

// parseFromReader parses a JSON-RPC response from a reader.
func (r *Response) parseFromReader(reader io.Reader, expectedSize int) error {
	// 16KB chunks by default
	chunkSize := defaultChunkSize
	data, err := readAll(reader, int64(chunkSize), expectedSize)
	if err != nil {
		return err
	}

	return r.parseFromBytes(data)
}

// unmarshalID unmarshals the raw ID bytes into the ID field.
// This function is designed to be called via sync.Once to ensure it runs exactly once.
func (r *Response) unmarshalID() error {
	// If there's no rawID to unmarshal, leave ID field as-is (may be nil or already set)
	if len(r.rawID) == 0 {
		return nil
	}

	var id any
	if err := getSonicAPI().Unmarshal(r.rawID, &id); err != nil {
		return fmt.Errorf("invalid id field: %w", err)
	}

	// If the value is "null", id will be nil
	if id == nil {
		r.id = nil
		return nil
	}

	switch v := id.(type) {
	case float64:
		// JSON numbers are unmarshalled as float64, so an explicit integer check is needed
		if v != float64(int64(v)) {
			r.id = v
		} else {
			r.id = int64(v)
		}
	case string:
		if v == "" {
			r.id = nil
		} else {
			r.id = v
		}
	default:
		return errors.New("id field must be a string or a number")
	}

	return nil
}

// writeString writes a string to the writer and updates the total byte count
func writeString(w io.Writer, s string, total *int64) error {
	return writeBytes(w, []byte(s), total)
}

// writeBytes writes bytes to the writer and updates the total byte count
func writeBytes(w io.Writer, b []byte, total *int64) error {
	written, err := w.Write(b)
	*total += int64(written)
	return err
}

// getIDBytes returns the marshaled ID bytes. Uses cached rawID if available to avoid re-marshaling.
func (r *Response) getIDBytes() ([]byte, error) {
	if len(r.rawID) > 0 {
		return r.rawID, nil
	}
	if r.id != nil {
		return getSonicAPI().Marshal(r.id)
	}
	return []byte("null"), nil
}

// writeErrorField writes the error field to the writer
func (r *Response) writeErrorField(w io.Writer, total *int64) error {
	if err := writeString(w, `,"error":`, total); err != nil {
		return err
	}

	errorBytes, err := r.getErrorBytes()
	if err != nil {
		return err
	}

	return writeBytes(w, errorBytes, total)
}

// getErrorBytes returns the marshaled error bytes
func (r *Response) getErrorBytes() ([]byte, error) {
	if r.err != nil {
		return getSonicAPI().Marshal(r.err)
	}
	return r.rawError, nil
}

// writeResultField writes the result field to the writer
func (r *Response) writeResultField(w io.Writer, total *int64) error {
	if err := writeString(w, `,"result":`, total); err != nil {
		return err
	}
	return writeBytes(w, r.result, total)
}
