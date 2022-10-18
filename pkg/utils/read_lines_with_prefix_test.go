package utils

import (
	"io"
	"strings"
	"testing"
)

func Test_ReadAllLinesWithPrefix(t *testing.T) {
	reader := strings.NewReader(strings.Join([]string{
		"prefix_first_line",
		"second_line",
		"third_line",
		"prefix_fourth_line",
		"prefix_last_line",
	}, string(MetricsLineSep)))
	lines, err := ReadAllLinesWithPrefix(reader, []byte("prefix_"), nil)
	if err != nil && err != io.EOF {
		t.Fatal(err)
	}
	if string(lines) != "prefix_first_line\nprefix_fourth_line\nprefix_last_line" {
		t.Fatalf("unexpected lines returned: %q", lines)
	}
}
