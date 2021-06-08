package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// norevive
const (
	BUGFIX      = "Bugfix"
	ENHANCEMENT = "Enhancement"
	OTHER       = "Others"
	BACKPORTING = "Backporting"
	MISC        = "Misc"
)

var (
	squashRegexp       = regexp.MustCompile(`(.*)-(\d+)`)
	mergeRequestRegexp = regexp.MustCompile(`Merge-pull-request-(\d+)`)
	prefixRegexp       = regexp.MustCompile(`^.+: (.+)`)
	releaseRegexp      = regexp.MustCompile(`(release-\d\.\d)`)
	latestHashRegexp   = regexp.MustCompile(`<!--Latest hash: (.+)-->`)

	versionSectionRegExp     = regexp.MustCompile(`^(\d.\d)`)
	backportsSectionRegExp   = regexp.MustCompile(fmt.Sprintf(`### %s\n((.+\n)+)`, BACKPORTING))
	enhancementSectionRegExp = regexp.MustCompile(fmt.Sprintf(`### %s\n((.+\n)+)`, ENHANCEMENT))
	bugfixSectionRegExp      = regexp.MustCompile(fmt.Sprintf(`### %s\n((.+\n)+)`, BUGFIX))
	otherSectionRegExp       = regexp.MustCompile(fmt.Sprintf(`### %s\n((.+\n)+)`, OTHER))
	miscSectionRegExp        = regexp.MustCompile(fmt.Sprintf(`### %s\n((.+\n)+)`, MISC))

	// PR categories
	categories = map[string]*regexp.Regexp{
		BUGFIX:      regexp.MustCompile(fmt.Sprintf(`- \[[xX]\] %s`, BUGFIX)),
		ENHANCEMENT: regexp.MustCompile(fmt.Sprintf(`- \[[xX]\] %s`, ENHANCEMENT)),
		OTHER:       regexp.MustCompile(fmt.Sprintf(`- \[[xX]\] %s`, OTHER)),
		BACKPORTING: regexp.MustCompile(fmt.Sprintf(`- \[[xX]\] %s`, BACKPORTING)),
	}

	gitHubToken = ""
	latestHash  = ""
)

const gitHubRepo = "insights-operator"
const gitHubRepoOwner = "openshift"
const gitHubPath = "https://github.com/openshift/insights-operator"

// API reference: https://docs.github.com/en/rest/reference/pulls#get-a-pull-request
const gitHubAPIFormat = "https://api.github.com/repos/%s/%s/pulls/%s" // owner, repo, pull-number

type Change struct {
	pullID      string
	hash        string
	title       string
	description string
	category    string
	release     string
}

func (c *Change) toMarkdown() string {
	return fmt.Sprintf("- [#%s](%s) %s\n", c.pullID, createPullRequestLink(c.pullID), c.title)
}

func createPullRequestLink(id string) string {
	return fmt.Sprintf("%s/pull/%s", gitHubPath, id)
}

func main() {
	log.SetFlags(0)
	if len(os.Args) != 1 && len(os.Args) != 3 {
		log.Fatalf(`Either specify two date arguments, AFTER and UNTIL, 
		to create a brand new CHANGELOG, or call it without arguments to 
		update the current one with new changes.`)
	}
	gitHubToken = os.Getenv("GITHUB_TOKEN")
	if len(gitHubToken) == 0 {
		log.Fatalf("Must set the GITHUB_TOKEN env variable to your GitHub access token.")
	}

	var gitLog []string
	var releaseBlocks map[string]MarkdownReleaseBlock
	if len(os.Args) == 3 {
		releaseBlocks = make(map[string]MarkdownReleaseBlock)
		after := os.Args[1]
		until := os.Args[2]
		gitLog = timeFrameReverseGitLog(after, until)
	} else {
		releaseBlocks = readCHANGELOG()
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
	latestHash = pullRequestHashes[numberOfChanges-1]
	changes := getChanges(pullRequestIds, pullRequestHashes)
	createCHANGELOG(updateToMarkdownReleaseBlock(releaseBlocks, changes))
}

type MarkdownReleaseBlock struct {
	backports    string
	enhancements string
	bugfixes     string
	others       string
	misc         string
}

func readCHANGELOG() map[string]MarkdownReleaseBlock {
	releaseBlocks := make(map[string]MarkdownReleaseBlock)
	rawBytes, _ := ioutil.ReadFile("./CHANGELOG.md")
	rawString := string(rawBytes)
	if match := latestHashRegexp.FindStringSubmatch(rawString); len(match) > 0 {
		latestHash = match[1]
	}
	versions := strings.Split(rawString, "\n## ")
	versions = versions[1:] // Trim 1. not relevant section

	for _, versionSection := range versions {
		var releaseBlock MarkdownReleaseBlock
		var version string
		if match := versionSectionRegExp.FindStringSubmatch(versionSection); len(match) > 0 {
			version = match[1]
		}
		if match := backportsSectionRegExp.FindStringSubmatch(versionSection); len(match) > 0 {
			releaseBlock.backports = match[1]
		}
		if match := enhancementSectionRegExp.FindStringSubmatch(versionSection); len(match) > 0 {
			releaseBlock.enhancements = match[1]
		}
		if match := bugfixSectionRegExp.FindStringSubmatch(versionSection); len(match) > 0 {
			releaseBlock.bugfixes = match[1]
		}
		if match := otherSectionRegExp.FindStringSubmatch(versionSection); len(match) > 0 {
			releaseBlock.others = match[1]
		}
		if match := miscSectionRegExp.FindStringSubmatch(versionSection); len(match) > 0 {
			releaseBlock.misc = match[1]
		}
		releaseBlocks[version] = releaseBlock
	}

	return releaseBlocks
}

func updateToMarkdownReleaseBlock(releaseBlocks map[string]MarkdownReleaseBlock, changes []*Change) map[string]MarkdownReleaseBlock {
	for _, ch := range changes {
		tmp := releaseBlocks[ch.release]
		if ch.category == BUGFIX {
			tmp.bugfixes = ch.toMarkdown() + tmp.bugfixes
			releaseBlocks[ch.release] = tmp
		} else if ch.category == OTHER {
			tmp.others = ch.toMarkdown() + tmp.others
			releaseBlocks[ch.release] = tmp
		} else if ch.category == ENHANCEMENT {
			tmp.enhancements = ch.toMarkdown() + tmp.enhancements
			releaseBlocks[ch.release] = tmp
		} else if ch.category == BACKPORTING {
			tmp.backports = ch.toMarkdown() + tmp.backports
			releaseBlocks[ch.release] = tmp
		} else {
			tmp.misc = ch.toMarkdown() + tmp.misc
			releaseBlocks[ch.release] = tmp
		}
	}
	return releaseBlocks
}

func createCHANGELOG(releaseBlocks map[string]MarkdownReleaseBlock) {
	file, _ := os.Create("CHANGELOG.md")
	defer file.Close()
	_, _ = file.WriteString(`# Note: This CHANGELOG is only for the changes in insights operator. 
	Please see OpenShift release notes for official changes\n`)
	_, _ = file.WriteString(fmt.Sprintf("<!--Latest hash: %s-->\n", latestHash))
	var releases []string
	for k := range releaseBlocks {
		releases = append(releases, k)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(releases)))
	for _, release := range releases {
		_, _ = file.WriteString(fmt.Sprintf("## %s\n\n", release))

		backports := releaseBlocks[release].backports
		createReleaseBlock(file, backports, BACKPORTING)

		enhancements := releaseBlocks[release].enhancements
		createReleaseBlock(file, enhancements, ENHANCEMENT)

		bugfixes := releaseBlocks[release].bugfixes
		createReleaseBlock(file, bugfixes, BUGFIX)

		others := releaseBlocks[release].others
		createReleaseBlock(file, others, OTHER)

		misc := releaseBlocks[release].misc
		createReleaseBlock(file, misc, MISC)
	}
}

func createReleaseBlock(file *os.File, release, title string) {
	if len(release) > 0 {
		_, _ = file.WriteString(fmt.Sprintf("### %s\n", title))
		_, _ = file.WriteString(fmt.Sprintf("%s\n", release))
	}
}

func getChanges(pullRequestIds, pullRequestHashes []string) []*Change {
	var changes []*Change
	for i, id := range pullRequestIds {
		change := getPullRequestFromGitHub(id)
		change.hash = pullRequestHashes[i]
		change = determineReleases(change)
		changes = append(changes, change)
	}
	return changes
}

func getPullRequestFromGitHub(id string) *Change {
	// There is a limit for the GitHub API, if you use auth then its 5000/hour
	var bearer = "token " + gitHubToken

	req, err := http.NewRequestWithContext(context.Background(), "GET", fmt.Sprintf(gitHubAPIFormat, gitHubRepoOwner, gitHubRepo, id), nil)
	if err != nil {
		log.Fatalf(err.Error())
	}
	req.Header.Add("Authorization", bearer)
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf(err.Error())
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		defer log.Fatalf(err.Error())
		return nil
	}
	var jsonMap map[string]json.RawMessage
	_ = json.Unmarshal(body, &jsonMap)

	var ch Change
	ch.pullID = id
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
	return &ch
}

func determineReleases(change *Change) *Change {
	releases := releaseBranchesContain(change.hash)
	earliestRelease := findEarliestRelease(releases)
	change.release = strings.Trim(earliestRelease, " \n*")
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
	minMajor := 99
	minMinor := 99
	for _, release := range releases {
		releaseNumber := strings.Split(release, "-")[1]
		releaseNumbers := strings.Split(releaseNumber, ".")
		majorRelease, _ := strconv.Atoi(releaseNumbers[0])
		minorRelease, _ := strconv.Atoi(releaseNumbers[1])
		if majorRelease < minMajor || majorRelease == minMajor && minorRelease < minMinor {
			minStr = releaseNumber
			minMajor = majorRelease
			minMinor = minorRelease
		}
	}
	return minStr
}

func timeFrameReverseGitLog(after, until string) []string {
	// nolint: gosec
	out, err := exec.Command(
		"git",
		"log",
		"--topo-order",
		"--pretty=tformat:%f|%H",
		"--reverse",
		fmt.Sprintf("--after=%s", after),
		fmt.Sprintf("--until=%s", until)).CombinedOutput()
	if err != nil {
		log.Fatal(err)
	}
	return strings.Split(string(out), "\n")
}

func sinceHashReverseGitLog(hash string) []string {
	// nolint: gosec
	out, err := exec.Command(
		"git",
		"log",
		"--topo-order",
		"--pretty=tformat:%f|%H",
		"--reverse",
		fmt.Sprintf("%s..HEAD", hash)).CombinedOutput()
	if err != nil {
		log.Fatal(err)
	}
	return strings.Split(string(out), "\n")
}

func getPullRequestInfo(gitLog []string) (ids, hashes []string) {
	var pullRequestIds []string
	var pullRequestHashes []string
	for _, line := range gitLog {
		split := strings.Split(line, "|")
		if match := mergeRequestRegexp.FindStringSubmatch(split[0]); len(match) > 0 {
			pullRequestIds = append(pullRequestIds, match[1])
			pullRequestHashes = append(pullRequestHashes, split[1])
		} else if match := squashRegexp.FindStringSubmatch(split[0]); len(match) > 0 {
			pullRequestIds = append(pullRequestIds, match[2])
			pullRequestHashes = append(pullRequestHashes, split[1])
		}
	}
	return pullRequestIds, pullRequestHashes
}
