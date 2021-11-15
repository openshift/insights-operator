package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"sort"
	"strings"

	"github.com/openshift/insights-operator/pkg/gatherers/workloads"
	"github.com/spf13/cobra"
)

type shapeHashStats struct {
	NoName   int
	Replaced int
	Total    int
}

func (s shapeHashStats) String() string {
	return fmt.Sprintf("%.01f%%(%d+%d/%d)", float64(s.Replaced+s.NoName)/float64(s.Total)*100, s.Replaced, s.NoName, s.Total)
}

func main() {
	var namesFilename string
	var extractNames bool
	var verbose bool

	cmd := &cobra.Command{
		Use:   "workloadinfo <WORKLOAD.JSON> [...]",
		Short: "Perform basic analysis of a workload info file",

		SilenceUsage:  true,
		SilenceErrors: true,

		RunE: func(cmd *cobra.Command, args []string) error {
			var out, errOut io.Writer = cmd.OutOrStdout(), cmd.ErrOrStderr()

			if extractNames {
				if len(args) > 0 {
					return fmt.Errorf("when --extract-names is specified arguments are not allowed")
				}
				if err := convertLinesToNames(cmd.InOrStdin(), out); err != nil {
					return err
				}
				return nil
			}

			if len(args) != 1 {
				return fmt.Errorf("expected one argument listing the name of a workload info file")
			}
			data, err := ioutil.ReadFile(args[0])
			if err != nil {
				return err
			}
			var info workloads.WorkloadPods
			if err := json.Unmarshal(data, &info); err != nil {
				return err
			}
			var nameHashes map[string]string
			if len(namesFilename) == 0 {
				hashes, err := newDefaultDictionary()
				if err != nil {
					return err
				}
				nameHashes = hashes
			} else {
				f, err := os.Open(namesFilename)
				if err != nil {
					return err
				}
				hashes, collisions, err := dictionaryFromReader(f)
				if len(collisions) > 0 {
					sort.Strings(collisions)
					if verbose {
						fmt.Fprintf(errOut, "info: names seen: %s\n", strings.Join(collisions, " "))
					}
				}
				nameHashes = hashes
			}

			namesSeen := make(map[string]struct{})
			var imageStats, initContainerStats, containerStats, namespaceStats shapeHashStats
			for name, image := range info.Images {
				imageStats.Total++
				if len(image.FirstArg) == 0 && len(image.FirstCommand) == 0 {
					imageStats.NoName++
					continue
				}
				var argSet, cmdSet bool
				if image.FirstArg, argSet = replaceIfSet(nameHashes, image.FirstArg); argSet {
					namesSeen[image.FirstArg] = struct{}{}
				}
				if image.FirstCommand, cmdSet = replaceIfSet(nameHashes, image.FirstCommand); cmdSet {
					namesSeen[image.FirstCommand] = struct{}{}
				}
				if argSet || cmdSet {
					info.Images[name] = image
					imageStats.Replaced++
				}
			}
			for namespaceName, ns := range info.Namespaces {
				namespaceStats.Total++
				for _, shape := range ns.Shapes {
					for i, container := range shape.InitContainers {
						initContainerStats.Total++
						if len(container.FirstArg) == 0 && len(container.FirstCommand) == 0 {
							initContainerStats.NoName++
							continue
						}
						var argSet, cmdSet bool
						if container.FirstArg, argSet = replaceIfSet(nameHashes, container.FirstArg); argSet {
							namesSeen[container.FirstArg] = struct{}{}
						}
						if container.FirstCommand, cmdSet = replaceIfSet(nameHashes, container.FirstCommand); cmdSet {
							namesSeen[container.FirstCommand] = struct{}{}
						}
						if argSet || cmdSet {
							shape.InitContainers[i] = container
							initContainerStats.Replaced++
						}
					}
					for i, container := range shape.Containers {
						containerStats.Total++
						if len(container.FirstArg) == 0 && len(container.FirstCommand) == 0 {
							containerStats.NoName++
							continue
						}
						var argSet, cmdSet bool
						if container.FirstArg, argSet = replaceIfSet(nameHashes, container.FirstArg); argSet {
							namesSeen[container.FirstArg] = struct{}{}
						}
						if container.FirstCommand, cmdSet = replaceIfSet(nameHashes, container.FirstCommand); cmdSet {
							namesSeen[container.FirstCommand] = struct{}{}
						}
						if argSet || cmdSet {
							shape.Containers[i] = container
							containerStats.Replaced++
						}
					}
				}
				if nsName, ok := replaceIfSet(nameHashes, namespaceName); ok {
					namesSeen[namespaceName] = struct{}{}
					delete(info.Namespaces, namespaceName)
					info.Namespaces[nsName] = ns
					namespaceStats.Replaced++
				}
			}
			data, err = json.MarshalIndent(info, "", "  ")
			if err != nil {
				return err
			}
			fmt.Fprintln(out, string(data))

			if verbose {
				fmt.Fprintf(errOut, "stats: namespaces=%s images=%s initContainers=%s containers=%s\n", namespaceStats.String(), imageStats.String(), initContainerStats.String(), containerStats.String())
				var sawNames []string
				for name := range namesSeen {
					sawNames = append(sawNames, name)
				}
				sort.Strings(sawNames)
				fmt.Fprintf(errOut, "info: names seen: %s\n", strings.Join(sawNames, " "))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&namesFilename, "names", "", "A file containing a list of names that may be hashed in this workload info.")
	cmd.Flags().BoolVar(&extractNames, "extract-names", false, "If specified, read standard input and convert each line into an equivalent hashed name.")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Display additional information about replacements performed to stderr")

	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		defer os.Exit(1)
	}
}

func replaceIfSet(hashes map[string]string, input string) (string, bool) {
	value, ok := hashes[input]
	if !ok {
		return input, false
	}
	return value, true
}

//go:generate sh -c "echo 'package main; var defaultDictionaryNames = `' > names.go; cat names.txt >> names.go; echo '`' >> names.go; gofmt -s -w names.go"

func newDefaultDictionary() (map[string]string, error) {
	out, collisions, err := dictionaryFromReader(strings.NewReader(defaultDictionaryNames))
	if err != nil {
		return nil, err
	}
	if len(collisions) > 0 {
		return nil, fmt.Errorf("hash collisions encountered during dictionary hashing: %v", collisions)
	}
	return out, nil
}

func dictionaryFromReader(r io.Reader) (map[string]string, []string, error) {
	var names []string
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimSpace(line)
		if len(line) == 0 || strings.HasPrefix(line, "#") {
			continue
		}
		names = append(names, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, err
	}
	return calculateDictionary(names)
}

func calculateDictionary(names []string) (map[string]string, []string, error) {
	h := workloads.NewDefaultHash()
	out := make(map[string]string)
	var collisions []string
	for _, name := range names {
		hashed := workloads.WorkloadHashString(h, name)
		if _, ok := out[hashed]; ok {
			collisions = append(collisions, name)
			continue
		}
		out[hashed] = name
	}
	return out, collisions, nil
}

// convertLinesToNames reads input line by line and outputs the workload argument string
// calculated for each line. Content after a '#' is ignored in the line, and empty
// lines are dropped. An error is returned if line reading fails.
func convertLinesToNames(r io.Reader, w io.Writer) error {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		if i := strings.Index(line, "#"); i != -1 {
			line = line[:i]
		}
		name := workloads.WorkloadArgumentString(line)
		if len(name) == 0 {
			continue
		}
		fmt.Fprintln(w, name)
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return nil
}
