package utils

import (
	"io"
	"bytes"
)

// CountLines reads the remainder of the reader and counts the number of lines.
//
// Inspired by: https://stackoverflow.com/a/24563853/
func CountLines(r io.Reader) (int, error) {
	buf := make([]byte, 0x8000)
	// Original implementation started from 0, but a file with no line separator
	// still contains a single line, so I would say that was an off-by-1 error.
	lineCount := 1
	for {
		c, err := r.Read(buf)
		lineCount += bytes.Count(buf[:c], lineSep)
		if err != nil {
			return lineCount, err
		}
	}
}