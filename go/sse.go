package parallelworks

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"
)

// sseError represents an error event from the SSE stream.
type sseError struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error"`
}

// SSEStreamError is a failure forwarded as an SSE error event, so it arrives inside
// a 200 and carries no HTTP status. Code is empty unless the server set one.
type SSEStreamError struct {
	Code    string
	Message string
}

func (e *SSEStreamError) Error() string { return e.Message }

// SSEReader reads SSE events from a streaming response body.
type SSEReader struct {
	scanner *bufio.Scanner
	done    bool
}

// NewSSEReader creates a new SSE reader from a response body.
func NewSSEReader(body io.Reader) *SSEReader {
	return &SSEReader{scanner: bufio.NewScanner(body)}
}

// Next reads the next SSE event and returns a ChatCompletionChunk.
// Returns io.EOF when the stream is done (receives [DONE] signal).
// Returns an error if the server sends an SSE error event.
func (r *SSEReader) Next() (*ChatCompletionChunk, error) {
	for r.scanner.Scan() {
		line := r.scanner.Text()
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			r.done = true
			return nil, io.EOF
		}

		// Check for SSE error events before parsing as a chunk
		var errEvent sseError
		if err := json.Unmarshal([]byte(data), &errEvent); err == nil && errEvent.Error.Message != "" {
			return nil, &SSEStreamError{Code: errEvent.Error.Code, Message: errEvent.Error.Message}
		}

		var chunk ChatCompletionChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue // skip malformed chunks
		}
		return &chunk, nil
	}
	if err := r.scanner.Err(); err != nil {
		return nil, err
	}
	return nil, io.EOF
}

// Done reports whether the stream sent its explicit [DONE] terminator.
func (r *SSEReader) Done() bool {
	return r.done
}
