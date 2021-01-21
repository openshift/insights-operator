package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

var (
	mergeRequest   = regexp.MustCompile(`Merge-pull-request-([\d]+)`)
	prefix         = regexp.MustCompile(`^.+: (.+)`)
)

const repoPath = "https://github.com/openshift/insights-operator"

func main() {
	log.SetFlags(0)
	if len(os.Args) != 3 {
		log.Fatalf("Must specify two date arguments, AFTER and UNTIL, example 2021-01-01")
	}
	after := os.Args[1]
	until := os.Args[2]

	out, err := exec.Command("git", "log", "--topo-order", "--pretty=tformat:%f|%b", "--reverse", fmt.Sprintf("--after=%s", after), fmt.Sprintf("--until=%s", until)).CombinedOutput()
	if err != nil {
		log.Fatal(err)
	}
	str_out := string(out)

	var mergeRequestIds []string
	var mergeRequestTitle []string
	for _, line := range strings.Split(str_out, "\n") {
		if match := mergeRequest.FindStringSubmatch(line); len(match) > 0 {
			mergeRequestIds = append(mergeRequestIds, match[1])
			title := strings.Split(line, "|")[1]
			if prefixMatch := prefix.FindStringSubmatch(title); len(prefixMatch) > 0 {
				title = prefixMatch[1]
			}
			mergeRequestTitle = append(mergeRequestTitle, title)
		}
	}
	for i := range mergeRequestIds {
		fmt.Printf("[#%s](%s/pulls/%s): %s\n", mergeRequestIds[i], repoPath, mergeRequestIds[i], mergeRequestTitle[i])
	}
}
