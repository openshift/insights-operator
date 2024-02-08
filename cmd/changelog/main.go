package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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
	// the assumption here is that the PR id is the last number in the string
	squashRegexp       = regexp.MustCompile(`(.*)-(\d+)$`)
	mergeRequestRegexp = regexp.MustCompile(`Merge-pull-request-(\d+)`)
	prefixRegexp       = regexp.MustCompile(`^.+: (.+)`)
	releaseRegexp      = regexp.MustCompile(`(release-\d\.\d+)`)
	latestHashRegexp   = regexp.MustCompile(`<!--Latest hash: (.+)-->`)

	versionSectionRegExp     = regexp.MustCompile(`^(\d.\d+)`)
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

// OpenShift release version helper type
type ReleaseVersion struct {
	Major int
	Minor int
}

type Change struct {
	pullID      string
	hash        string
	title       string
	description string
	category    string
	release     ReleaseVersion
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
	var releaseBlocks map[ReleaseVersion]MarkdownReleaseBlock
	if len(os.Args) == 3 {
		releaseBlocks = make(map[ReleaseVersion]MarkdownReleaseBlock)
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

func readCHANGELOG() map[ReleaseVersion]MarkdownReleaseBlock {
	log.Print("Reading existing changelog")
	releaseBlocks := make(map[ReleaseVersion]MarkdownReleaseBlock)
	rawBytes, _ := os.ReadFile("./CHANGELOG.md")
	rawString := string(rawBytes)
	if match := latestHashRegexp.FindStringSubmatch(rawString); len(match) > 0 {
		latestHash = match[1]
	}
	versions := strings.Split(rawString, "\n## ")
	versions = versions[1:] // Trim 1. not relevant section

	for _, versionSection := range versions {
		var releaseBlock MarkdownReleaseBlock
		var version ReleaseVersion
		if match := versionSectionRegExp.FindStringSubmatch(versionSection); len(match) > 0 {
			version = stringToReleaseVersion(match[1])
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
		// We want only found versions - e.g master is not considered as a version
		if version != (ReleaseVersion{}) {
			releaseBlocks[version] = releaseBlock
		}
	}

	return releaseBlocks
}

func updateToMarkdownReleaseBlock(releaseBlocks map[ReleaseVersion]MarkdownReleaseBlock,
	changes []*Change) map[ReleaseVersion]MarkdownReleaseBlock {
	log.Print("Applying new changes")
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

func createCHANGELOG(releaseBlocks map[ReleaseVersion]MarkdownReleaseBlock) {
	log.Print("Writing new changes to CHANGELOG.md file")
	file, _ := os.Create("CHANGELOG.md")
	defer file.Close()
	_, _ = file.WriteString(`# Note: This CHANGELOG is only for the changes in insights operator.
	Please see OpenShift release notes for official changes\n`)
	_, _ = file.WriteString(fmt.Sprintf("<!--Latest hash: %s-->\n", latestHash))
	var releases ReleaseVersions
	for k := range releaseBlocks {
		releases = append(releases, k)
	}
	sort.Sort(sort.Reverse(releases))
	for _, release := range releases {
		_, _ = file.WriteString(fmt.Sprintf("## %d.%d\n\n", release.Major, release.Minor))

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
	log.Print("Reading changes from the GitHub API")
	for i, id := range pullRequestIds {
		// This regex checks that the ids passed as CLI arguments are valid.
		// This code cannot be encapsulated or Snyk will flag it as a defect.
		// This warning was originally raised in issue OCPBUGS-26937.
		if regexp.MustCompile(`^\d*$`).MatchString(id) {
			change := getPullRequestFromGitHub(id)
			change.hash = pullRequestHashes[i]
			if _, err := determineReleases(change); err != nil {
				continue
			}
			changes = append(changes, change)
		} else {
			log.Print("ERR :: could not validate entered Pull Request, ", id)
		}
	}
	return changes
}

func getPullRequestFromGitHub(id string) *Change {
	// There is a limit for the GitHub API, if you use auth then its 5000/hour
	var bearer = "token " + gitHubToken

	req, err := http.NewRequestWithContext(
		context.Background(),
		"GET",
		fmt.Sprintf(gitHubAPIFormat, gitHubRepoOwner, gitHubRepo, id),
		http.NoBody)
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
	body, err := io.ReadAll(resp.Body)
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

func determineReleases(change *Change) (*Change, error) {
	releases := releaseBranchesContain(change.hash)
	if len(releases) == 0 {
		log.Printf("Did not find release match for the commit %s. This is likely master branch.\n", change.hash)
		return nil, fmt.Errorf("can't determine release for commit %s", change.hash)
	}
	earliestRelease := findEarliestRelease(releases)
	change.release = earliestRelease
	return change, nil
}

func releaseBranchesContain(hash string) []ReleaseVersion {
	var releaseBranches []ReleaseVersion
	out, err := exec.Command("git", "branch", "--contains", hash).CombinedOutput()
	if err != nil {
		log.Fatalf(err.Error())
	}
	branches := string(out)
	matches := releaseRegexp.FindAllStringSubmatch(branches, -1)

	for _, match := range matches {
		version := strings.TrimPrefix(match[1], "release-")
		relVer := stringToReleaseVersion(version)
		releaseBranches = append(releaseBranches, relVer)
	}
	return releaseBranches
}

func findEarliestRelease(releases ReleaseVersions) ReleaseVersion {
	sort.Sort(releases)
	return releases[0]
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

func stringToReleaseVersion(s string) ReleaseVersion {
	var relVer ReleaseVersion
	versionNums := strings.Split(s, ".")
	major, err := strconv.Atoi(versionNums[0])
	if err != nil {
		log.Fatalf("Failed to parse %s: %v", versionNums[0], err.Error())
	}
	minor, err := strconv.Atoi(versionNums[1])
	if err != nil {
		log.Fatalf("Failed to parse %s: %v", versionNums[1], err.Error())
	}
	relVer.Major = major
	relVer.Minor = minor
	return relVer
}

type ReleaseVersions []ReleaseVersion

func (r ReleaseVersions) Len() int {
	return len(r)
}

func (r ReleaseVersions) Swap(i, j int) {
	r[i], r[j] = r[j], r[i]
}

func (r ReleaseVersions) Less(i, j int) bool {
	if r[i].Major == r[j].Major {
		return r[i].Minor < r[j].Minor
	}
	return r[i].Major < r[j].Major
}
