package history

import (
	"fmt"
	"strings"

	"gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing/object"
)

type GitReporter struct {
	repo *git.Repository
}

func NewGitReporter(repo *git.Repository) *GitReporter {
	return &GitReporter{repo: repo}
}

func (r GitReporter) Report() ([]byte, error) {
	iter, err := r.repo.Log(&git.LogOptions{
		Order: git.LogOrderCommitterTime,
		All:   true,
	})
	if err != nil {
		return nil, err
	}

	report := []string{}
	err = iter.ForEach(func(c *object.Commit) error {
		parent, err := c.Parent(0)
		if err != nil && err != object.ErrParentNotFound {
			return err
		}
		var content string
		if parent != nil {
			patch, err := c.Patch(parent)
			if err != nil {
				return err
			}
			content = patch.String()
		} else {
			fIter, err := c.Files()
			if err != nil {
				return err
			}
			if err := fIter.ForEach(func(file *object.File) error {
				c, err := file.Contents()
				if err != nil {
					return err
				}
				content += c
				return nil
			}); err != nil {
				return err
			}
		}
		message := fmt.Sprintf("[%.8s] [%s] %s\n%s\n", c.Hash.String(), c.Author.When.Format(object.DateFormat), c.Message, content)
		report = append(report, message)
		return nil
	})

	// make this look like a log/report (latest changes at bottom)
	reverse(report)

	return []byte(strings.Join(report, "\n")), nil
}

func reverse(a []string) {
	n := len(a)
	for i := 0; i < n/2; i++ {
		a[i], a[n-i-1] = a[n-i-1], a[i]
	}
}
