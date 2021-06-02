package anonymize

import (
	"regexp"
	"strings"
)

func URLCSV(s string) string {
	strs := strings.Split(s, ",")
	outSlice := URLSlice(strs)
	return strings.Join(outSlice, ",")
}

func URLSlice(in []string) []string {
	var outSlice []string
	for _, str := range in {
		outSlice = append(outSlice, URL(str))
	}
	return outSlice
}

var reURL = regexp.MustCompile(`[^.\-/:]`)

func URL(s string) string { return reURL.ReplaceAllString(s, "x") }
