package anonymize

import "strings"

func String(s string) string {
	return strings.Repeat("x", len(s))
}

func Bytes(s []byte) []byte {
	return []byte(strings.Repeat("x", len(s)))
}
