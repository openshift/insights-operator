package main

import (
	"fmt"
	"log"
	"reflect"
	"encoding/json"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"net/http"
	"io/ioutil"
)

var (
	mergeRequest	= regexp.MustCompile(`Merge-pull-request-([\d]+)`)
	prefix			= regexp.MustCompile(`^.+: (.+)`)

	// PR categories
	categories = map[string] *regexp.Regexp {
		"BugFix" : regexp.MustCompile(`- \[[xX]\] Bugfix`),
		"Enhancement": regexp.MustCompile(`- \[[xX]\] Enhancement`),
		"Other": regexp.MustCompile(`- \[[xX]\] Others`),
		"Backporting": regexp.MustCompile(`- \[[xX]\] Backporting`),
	}
)


type Change struct {
	pullId 		string
	title 		string
	description string
	category 	string
}

func (c Change) toMarkdown() string {
	return fmt.Sprintf("- [#%s](%s) %s\n", c.pullId, createPullRequestLink(c.pullId), c.title)
}

const gitHubRepo = "insights-operator"
const gitHubRepoOwner = "openshift"
const gitHubPath = "https://github.com/openshift/insights-operator"
// API reference: https://docs.github.com/en/rest/reference/pulls#get-a-pull-request
const gitHubAPIFormat = "https://api.github.com/repos/%s/%s/pulls/%s" //owner, repo, pull-number

func main() {
	log.SetFlags(0)
	if len(os.Args) != 3 {
		log.Fatalf("Must specify two date arguments, AFTER and UNTIL, example 2021-01-01")
	}
	after := os.Args[1]
	until := os.Args[2]

	gitLog := simpleReverseGitLog(after, until)
	pullRequestIds := getPullRequestIds(gitLog)

	changes := pruneChanges(getChanges(pullRequestIds))
	createCHANGELOG(changes)
}

func createCHANGELOG(changes []Change) {
	var bugfixes []Change
	var others []Change
	var enhancements []Change
	for _, ch := range changes {
		if ch.category == "BugFix" {
			bugfixes = append(bugfixes, ch)
		} else if ch.category == "Other" {
			others = append(others, ch)
		} else if ch.category == "Enhancement" {
			enhancements = append(enhancements, ch)
		}
	}
	file, _ := os.Create("example_CHANGELOG.md")
	defer file.Close()
	_,_ = file.WriteString("# Note: This CHANGELOG is only for the changes in insights operator. Please see OpenShift release notes for official changes\n")

	// TODO: Get the version info somehow
	_,_ = file.WriteString("## VERSION\n")

	_,_ = file.WriteString("### Enhancement\n")
	for _, e := range enhancements {
		_,_ = file.WriteString(e.toMarkdown())
	}
	_,_ = file.WriteString("### Bug fixes\n")
	for _, b := range bugfixes {
		_,_ = file.WriteString(b.toMarkdown())
	}
	_,_ = file.WriteString("### Others\n")
	for _, o := range others {
		_,_ = file.WriteString(o.toMarkdown())
	}

}

func pruneChanges(changes []Change) []Change {
	// TODO: Somehow determine which change is changelog worthy
	return changes
}

func getChanges(pullRequestIds []string) []Change {
	var changes []Change
	var cases []reflect.SelectCase
	for _, id := range pullRequestIds {
		channel := make(chan Change)
		cases = append(cases, reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(channel)})
		go getPullRequestFromGitHub(id, channel)
	}

	remaining := len(cases)
	for remaining > 0 {
		chosen, value, _ := reflect.Select(cases)
		cases[chosen].Chan = reflect.ValueOf(nil)
		remaining -= 1
		change, _ := value.Interface().(Change)
		changes = append(changes, change)
	}
	return changes
}

func getPullRequestFromGitHub(id string, channel chan<- Change) {
	defer close(channel)
	resp, err := http.Get(fmt.Sprintf(gitHubAPIFormat, gitHubRepoOwner, gitHubRepo, id))
	if err != nil {
		log.Fatalf(err.Error())
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf(err.Error())
	}
	var jsonMap map[string]json.RawMessage
	_ = json.Unmarshal(body, &jsonMap)

	var ch Change
	ch.pullId = id
	_ = json.Unmarshal(jsonMap["title"], &ch.title)
	// Remove nosy prefix
	if match := prefix.FindStringSubmatch(ch.title); len(match) > 0 {
		ch.title = match[1]
	}

	_ = json.Unmarshal(jsonMap["body"], &ch.description)
	// Figure out the change`s category
	for cat, reg := range categories {
		if match := reg.FindStringSubmatch(ch.description); len(match) > 0 {
			ch.category = cat
			break
		}
	}
	channel <- ch
}

func createPullRequestLink(id string) string {
	return fmt.Sprintf("%s/pull/%s", gitHubPath, id)
}

func simpleReverseGitLog(after string, until string) []string {
	out, err := exec.Command("git", "log", "--topo-order", "--pretty=tformat:%f", "--reverse", fmt.Sprintf("--after=%s", after), fmt.Sprintf("--until=%s", until)).CombinedOutput()
	if err != nil {
		log.Fatal(err)
	}
	return strings.Split(string(out), "\n")
}

func getPullRequestIds(gitLog []string) []string {
	var pullRequestIds []string
	for _, line := range gitLog {
		if match := mergeRequest.FindStringSubmatch(line); len(match) > 0 {
			pullRequestIds = append(pullRequestIds, match[1])
		}
	}
	return pullRequestIds
}