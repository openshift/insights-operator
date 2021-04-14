package anonymize

import "strings"

func AnonymizeString(s string) string {
	return strings.Repeat("x", len(s))
}
