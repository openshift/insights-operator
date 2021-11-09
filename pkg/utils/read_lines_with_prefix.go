package utils

import (
	"bytes"
	"fmt"
	"io"
)

func ReadAllLinesWithPrefix(reader io.Reader, prefix []byte) ([]byte, error) {
	buff := []byte{}
	tmp := make([]byte, 0, 1024)
	partialLine := []byte{}
	for {
		rc, err := reader.Read(tmp)
		if err != nil {
			if bytes.HasPrefix(partialLine, prefix) {
				buff = append(buff, partialLine...)
			}
			return buff, err
		}
		lines := bytes.SplitAfter(tmp[:rc], lineSep)

		// If the last line of the previous iteration wasn't properly terminated.
		if len(partialLine) > 0 {
			partialLine = append(partialLine, lines[0]...)
			// Remove the first line from the slice.
			lines = lines[1:]

			// If the partial line has been finished, it should be processed.
			if bytes.HasSuffix(partialLine, lineSep) {
				if bytes.HasPrefix(partialLine, prefix) {
					buff = append(buff, partialLine...)
				}
				partialLine = []byte{}
			} else if len(lines) > 0 {
				// It should never happen, but it's better to have this sanity check.
				return buff, fmt.Errorf("unexpected line before the end of the previous line")
			}
		}

		// Check if the last line is terminated with a line separator.
		if linesLen := len(lines); linesLen > 0 && !bytes.HasSuffix(lines[linesLen-1], lineSep) {
			partialLine = append(partialLine, lines[linesLen-1]...)
			// Remove the last line from the slice.
			lines = lines[:linesLen-1]
		}

		// The slice now only contains full lines.
		for _, line := range lines {
			if bytes.HasPrefix(line, prefix) {
				buff = append(buff, line...)
			}
		}
	}
}
