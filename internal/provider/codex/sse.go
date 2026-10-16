package codex

import (
	"bufio"
	"bytes"
	"io"
	"unicode/utf8"

	"github.com/Jghh42/atuin-ai/internal/config"
)

// The 1 MiB budget includes every wire byte in a frame: comments, ignored
// fields, field names, line endings and blank delimiter. Long lines use the
// same budget; an endless heartbeat line cannot evade it. Blank frames reset it.
func readSSE(r io.Reader, consume func(string, []byte) (bool, error)) error {
	b := bufio.NewReaderSize(r, 4096)
	var line, data []byte
	var event string
	size := 0
	for {
		fragment, err := b.ReadSlice('\n')
		size += len(fragment)
		if size > config.MaxProviderEventBytes {
			return ErrStream
		}
		line = append(line, fragment...)
		if err == bufio.ErrBufferFull {
			continue
		}
		if err != nil {
			return ErrStream
		} // even a valid JSON payload needs its blank delimiter
		if !utf8.Valid(line) {
			return ErrStream
		}
		line = bytes.TrimSuffix(line, []byte{'\n'})
		line = bytes.TrimSuffix(line, []byte{'\r'})
		if bytes.ContainsRune(line, '\r') {
			return ErrStream
		}
		if len(line) == 0 {
			if len(data) > 0 {
				done, e := consume(event, data[:len(data)-1])
				if e != nil {
					return e
				}
				if done {
					return nil
				}
			}
			size = 0
			line = line[:0]
			data = data[:0]
			event = ""
			continue
		}
		if line[0] != ':' {
			key, value, found := bytes.Cut(line, []byte{':'})
			if !found {
				value = nil
			}
			value = bytes.TrimPrefix(value, []byte{' '})
			switch string(key) {
			case "data":
				data = append(data, value...)
				data = append(data, '\n')
			case "event":
				if event != "" && event != string(value) {
					return ErrStream
				}
				event = string(value)
			}
		}
		line = line[:0]
	}
}
