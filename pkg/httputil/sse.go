package httputil

import (
	"bufio"
	"bytes"
	"io"
)

// ProcessSSEStream reads server-sent events from io.Reader and invokes the callback for each 'data:' payload
func ProcessSSEStream(r io.Reader, onData func(data []byte) error) error {
	scanner := bufio.NewScanner(r)
	prefix := []byte("data: ")

	for scanner.Scan() {
		line := scanner.Bytes()

		// Skip empty lines
		if len(line) == 0 {
			continue
		}

		// Stop if data is [DONE]
		if bytes.Equal(line, []byte("data: [DONE]")) {
			break
		}

		if bytes.HasPrefix(line, prefix) {
			payload := bytes.TrimPrefix(line, prefix)
			if err := onData(payload); err != nil {
				return err
			}
		}
	}

	return scanner.Err()
}
