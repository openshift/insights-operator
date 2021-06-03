package anonymize

import "strings"

func String(s string) string {
	return strings.Repeat("x", len(s))
}
