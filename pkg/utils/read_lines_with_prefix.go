package utils

import (
	"bytes"
	"fmt"
	"io"
)

// ReadAllLinesWithPrefix reads lines from the given reader
// and returns those that begin with the specified prefix.
func ReadAllLinesWithPrefix(reader io.Reader, prefix []byte) ([]byte, error) {
	buff := []byte{}
	tmp := make([]byte, 1024)
	partialLine := []byte{}
	for {
		rc, err := reader.Read(tmp)
		// If nothing was read or if a non-EOF error occurred.
		if rc <= 0 || err != nil && err != io.EOF {
			buff = appendBufferIfLinePrefixed(buff, prefix, partialLine)
			return buff, err
		}
		lines := bytes.SplitAfter(tmp[:rc], MetricsLineSep)

		// If the last line of the previous iteration wasn't properly terminated.
		if len(partialLine) > 0 {
			partialLine = append(partialLine, lines[0]...)
			// Remove the first line from the slice.
			lines = lines[1:]

			// If the partial line has been finished, it should be processed.
			if bytes.HasSuffix(partialLine, MetricsLineSep) {
				buff = appendBufferIfLinePrefixed(buff, prefix, partialLine)
				partialLine = []byte{}
			} else if len(lines) > 0 {
				// It should never happen, but it's better to have this sanity check.
				return buff, fmt.Errorf("unexpected line before the end of the previous line")
			}
		}

		// Check if the last line is terminated with a line separator.
		if linesLen := len(lines); linesLen > 0 && !bytes.HasSuffix(lines[linesLen-1], MetricsLineSep) {
			partialLine = append(partialLine, lines[linesLen-1]...)
			// Remove the last line from the slice.
			lines = lines[:linesLen-1]
		}

		// The slice now only contains full lines.
		for _, line := range lines {
			buff = appendBufferIfLinePrefixed(buff, prefix, line)
		}

		// If the EOF was reported by the reader.
		if err == io.EOF {
			buff = appendBufferIfLinePrefixed(buff, prefix, partialLine)
			return buff, err
		}
	}
}

func appendBufferIfLinePrefixed(buff, prefix, line []byte) []byte {
	if bytes.HasPrefix(line, prefix) {
		buff = append(buff, line...)
	}
	return buff
}
