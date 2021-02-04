package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var (
	mergeRequestRegexp = regexp.MustCompile(`Merge-pull-request-([\d]+)`)
	prefixRegexp       = regexp.MustCompile(`^.+: (.+)`)
	releaseRegexp      = regexp.MustCompile(`(release-\d\.\d)`)
	latestHashRegexp   = regexp.MustCompile(`<!--Latest hash: (.+)-->`)

	version_sectionRegExp     = regexp.MustCompile(`^(\d.\d)`)
	backports_sectionRegExp   = regexp.MustCompile(`### Backports\n((.+\n)+)`)
	enhancement_sectionRegExp = regexp.MustCompile(`### Enhancements\n((.+\n)+)`)
	bugfix_sectionRegExp      = regexp.MustCompile(`### Bug fixes\n((.+\n)+)`)
	other_sectionRegExp       = regexp.MustCompile(`### Others\n((.+\n)+)`)
	misc_sectionRegExp        = regexp.MustCompile(`### Misc\n((.+\n)+)`)

	// PR categories
	categories = map[string]*regexp.Regexp{
		"BugFix":      regexp.MustCompile(`- \[[xX]\] Bugfix`),
		"Enhancement": regexp.MustCompile(`- \[[xX]\] Enhancement`),
		"Other":       regexp.MustCompile(`- \[[xX]\] Others`),
		"Backporting": regexp.MustCompile(`- \[[xX]\] Backporting`),
	}

	gitHubToken = ""
	latestHash  = ""
)

const gitHubRepo = "insights-operator"
const gitHubRepoOwner = "openshift"
const gitHubPath = "https://github.com/openshift/insights-operator"

// API reference: https://docs.github.com/en/rest/reference/pulls#get-a-pull-request
const gitHubAPIFormat = "https://api.github.com/repos/%s/%s/pulls/%s" //owner, repo, pull-number

type Change struct {
	pullId      string
	hash        string
	title       string
	description string
	category    string
	release     string
}

func (c Change) toMarkdown() string {
	return fmt.Sprintf("- [#%s](%s) %s\n", c.pullId, createPullRequestLink(c.pullId), c.title)
}

func createPullRequestLink(id string) string {
	return fmt.Sprintf("%s/pull/%s", gitHubPath, id)
}

func main() {
	log.SetFlags(0)
	if len(os.Args) != 1 && len(os.Args) != 3 {
		log.Fatalf("Either specify two date arguments, AFTER and UNTIL, to create a brand new CHANGELOG, or call it without arguments to update the current one with new changes.")
	}
	gitHubToken = os.Getenv("GITHUB_TOKEN")
	if len(gitHubToken) == 0 {
		log.Fatalf("Must set the GITHUB_TOKEN env variable to your GitHub access token.")
	}

	var gitLog []string
	var release_blocks map[string]MarkdownReleaseBlock
	if len(os.Args) == 3 {
		release_blocks = make(map[string]MarkdownReleaseBlock)
		after := os.Args[1]
		until := os.Args[2]
		gitLog = timeFrameReverseGitLog(after, until)
	} else {
		release_blocks = readCHANGELOG()
		if latestHash == "" {
			log.Fatalf("Latest hash is missing from CHANGELOG, can't update without it.")
		}
		gitLog = sinceHashReverseGitLog(latestHash)
	}
	pullRequestIds, pullRequestHashes := getPullRequestInfo(gitLog)
	numberOfChanges := len(pullRequestHashes)
	if numberOfChanges < 1 {
		log.Fatal("No new changes detected.")
	}
	latestHash = pullRequestHashes[numberOfChanges - 1]
	changes := pruneChanges(getChanges(pullRequestIds, pullRequestHashes))
	createCHANGELOG(updateToMarkdownReleaseBlock(release_blocks, changes))
}

type MarkdownReleaseBlock struct {
	backports    string
	enhancements string
	bugfixes     string
	others       string
	misc         string
}

func readCHANGELOG() map[string]MarkdownReleaseBlock {
	release_blocks := make(map[string]MarkdownReleaseBlock)
	rawBytes, _ := ioutil.ReadFile("./CHANGELOG.md")
	rawString := string(rawBytes)
	if match := latestHashRegexp.FindStringSubmatch(rawString); len(match) > 0 {
		latestHash = match[1]
	}
	versions := strings.Split(rawString, "\n## ")
	versions = versions[1:] // Trim 1. not relevant section

	for _, version_section := range versions {
		var release_block MarkdownReleaseBlock
		var version string
		if match := version_sectionRegExp.FindStringSubmatch(version_section); len(match) > 0 {
			version = match[1]
		}
		if match := backports_sectionRegExp.FindStringSubmatch(version_section); len(match) > 0 {
			release_block.backports = match[1]
		}
		if match := enhancement_sectionRegExp.FindStringSubmatch(version_section); len(match) > 0 {
			release_block.enhancements = match[1]
		}
		if match := bugfix_sectionRegExp.FindStringSubmatch(version_section); len(match) > 0 {
			release_block.bugfixes = match[1]
		}
		if match := other_sectionRegExp.FindStringSubmatch(version_section); len(match) > 0 {
			release_block.others = match[1]
		}
		if match := misc_sectionRegExp.FindStringSubmatch(version_section); len(match) > 0 {
			release_block.misc = match[1]
		}
		release_blocks[version] = release_block
	}

	return release_blocks
}

func updateToMarkdownReleaseBlock(release_blocks map[string]MarkdownReleaseBlock, changes []Change) map[string]MarkdownReleaseBlock {
	for _, ch := range changes {
		tmp := release_blocks[ch.release]
		if ch.category == "BugFix" {
			tmp.bugfixes += ch.toMarkdown()
			release_blocks[ch.release] = tmp
		} else if ch.category == "Other" {
			tmp.others += ch.toMarkdown()
			release_blocks[ch.release] = tmp
		} else if ch.category == "Enhancement" {
			tmp.enhancements += ch.toMarkdown()
			release_blocks[ch.release] = tmp
		} else if ch.category == "Backporting" {
			tmp.backports += ch.toMarkdown()
			release_blocks[ch.release] = tmp
		} else {
			tmp.misc += ch.toMarkdown()
			release_blocks[ch.release] = tmp
		}
	}
	return release_blocks
}

func createCHANGELOG(release_blocks map[string]MarkdownReleaseBlock) {
	file, _ := os.Create("CHANGELOG.md")
	defer file.Close()
	_, _ = file.WriteString("# Note: This CHANGELOG is only for the changes in insights operator. Please see OpenShift release notes for official changes\n")
	_, _ = file.WriteString(fmt.Sprintf("<!--Latest hash: %s-->\n", latestHash))
	var releases []string
	for k := range release_blocks {
		releases = append(releases, k)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(releases)))
	for _, release := range releases {
		_, _ = file.WriteString(fmt.Sprintf("## %s\n\n", release))

		backports := release_blocks[release].backports
		if len(backports) > 0 {
			_, _ = file.WriteString("### Backports\n")
			_, _ = file.WriteString(fmt.Sprintf("%s\n", backports))
		}
		enhancements := release_blocks[release].enhancements
		if len(enhancements) > 0 {
			_, _ = file.WriteString("### Enhancements\n")
			_, _ = file.WriteString(fmt.Sprintf("%s\n", enhancements))
		}
		bugfixes := release_blocks[release].bugfixes
		if len(bugfixes) > 0 {
			_, _ = file.WriteString("### Bug fixes\n")
			_, _ = file.WriteString(fmt.Sprintf("%s\n", bugfixes))
		}
		others := release_blocks[release].others
		if len(others) > 0 {
			_, _ = file.WriteString("### Others\n")
			_, _ = file.WriteString(fmt.Sprintf("%s\n", others))
		}
		misc := release_blocks[release].misc
		if len(misc) > 0 {
			_, _ = file.WriteString("### Misc\n")
			_, _ = file.WriteString(fmt.Sprintf("%s\n", misc))
		}
	}

}

func pruneChanges(changes []Change) []Change {
	// TODO: Somehow determine which change is changelog worthy
	return changes
}

func getChanges(pullRequestIds []string, pullRequestHashes []string) []Change {
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
		change.hash = pullRequestHashes[chosen]
		change = determineReleases(change)
		changes = append(changes, change)
	}
	return changes
}

func getPullRequestFromGitHub(id string, channel chan<- Change) {
	defer close(channel)
	// There is a limit for the GitHub API, if you use auth then its 5000/hour
	var bearer = "token " + gitHubToken

	req, err := http.NewRequest("GET", fmt.Sprintf(gitHubAPIFormat, gitHubRepoOwner, gitHubRepo, id), nil)
	if err != nil {
		log.Fatalf(err.Error())
	}
	req.Header.Add("Authorization", bearer)
	client := &http.Client{}
	resp, err := client.Do(req)
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
	if match := prefixRegexp.FindStringSubmatch(ch.title); len(match) > 0 {
		ch.title = match[1]
	}

	_ = json.Unmarshal(jsonMap["body"], &ch.description)
	// Figure out the change`s category
	var cats []string
	for c := range categories {
		cats = append(cats, c)
	}
	sort.Strings(cats) // Hacky way to make sure that Backports get matched first.
	for _, cat := range cats {
		if match := categories[cat].FindStringSubmatch(ch.description); len(match) > 0 {
			ch.category = cat
			break
		}
	}
	channel <- ch
}

func determineReleases(change Change) Change {
	releases := releaseBranchesContain(change.hash)
	earliest_release := findEarliestRelease(releases)
	change.release = strings.Trim(earliest_release, " \n*")
	return change
}

func releaseBranchesContain(hash string) []string {
	var releaseBranches []string
	out, err := exec.Command("git", "branch", "--contains", hash).CombinedOutput()
	if err != nil {
		log.Fatalf(err.Error())
	}
	branches := string(out)
	matches := releaseRegexp.FindAllStringSubmatch(branches, -1)
	for _, match := range matches {
		releaseBranches = append(releaseBranches, match[1])
	}
	return releaseBranches
}

func findEarliestRelease(releases []string) string {
	// Its hacky I KNOW.
	minStr := ""
	minMayor := 99
	minMinor := 99
	for _, release := range releases {
		release_number := strings.Split(release, "-")[1]
		release_numbers := strings.Split(release_number, ".")
		mayor_release, _ := strconv.Atoi(release_numbers[0])
		minor_release, _ := strconv.Atoi(release_numbers[1])
		if mayor_release < minMayor || mayor_release == minMayor && minor_release < minMinor {
			minStr = release_number
			minMayor = mayor_release
			minMinor = minor_release
		}
	}
	return minStr
}

func timeFrameReverseGitLog(after string, until string) []string {
	out, err := exec.Command("git", "log", "--topo-order", "--pretty=tformat:%f|%H", "--reverse", fmt.Sprintf("--after=%s", after), fmt.Sprintf("--until=%s", until)).CombinedOutput()
	if err != nil {
		log.Fatal(err)
	}
	return strings.Split(string(out), "\n")
}

func sinceHashReverseGitLog(hash string) []string {
	out, err := exec.Command("git", "log", "--topo-order", "--pretty=tformat:%f|%H", "--reverse", fmt.Sprintf("%s..HEAD", hash)).CombinedOutput()
	if err != nil {
		log.Fatal(err)
	}
	return strings.Split(string(out), "\n")
}

func getPullRequestInfo(gitLog []string) ([]string, []string) {
	var pullRequestIds []string
	var pullRequestHashes []string
	for _, line := range gitLog {
		split := strings.Split(line, "|")
		if match := mergeRequestRegexp.FindStringSubmatch(split[0]); len(match) > 0 {
			pullRequestIds = append(pullRequestIds, match[1])
			pullRequestHashes = append(pullRequestHashes, split[1])
		}
	}
	return pullRequestIds, pullRequestHashes
}
