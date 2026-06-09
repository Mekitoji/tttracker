package tui

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// repoFinder lists local git repositories (used by the manual repo-path search).
type repoFinder interface {
	allRepos() []string
}

// execRepoFinder finds git repos under root by locating ".git" directories with
// fd (preferred) or find. rg is intentionally not used: it searches file
// contents, not directory names, so it is unsuited to locating repositories.
//
// The scan is bounded (timeout, depth limit, excluded heavy directories) and is
// always run off the UI thread by the caller.
type execRepoFinder struct {
	root  string
	fdBin string
}

func newExecRepoFinder() execRepoFinder {
	home, _ := os.UserHomeDir()
	f := execRepoFinder{root: home}
	for _, bin := range []string{"fd", "fdfind"} {
		if p, err := exec.LookPath(bin); err == nil {
			f.fdBin = p
			break
		}
	}
	return f
}

func (f execRepoFinder) allRepos() []string {
	if f.root == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	var cmd *exec.Cmd
	if f.fdBin != "" {
		cmd = exec.CommandContext(ctx, f.fdBin,
			"--hidden", "--type", "d", "--absolute-path", "--glob", ".git",
			"--max-depth", "10", "--max-results", "2000",
			"--exclude", "Library", "--exclude", "node_modules",
			"--exclude", ".cache", "--exclude", "Caches",
			f.root)
	} else {
		cmd = exec.CommandContext(ctx, "find", f.root,
			"-maxdepth", "10",
			"(", "-name", "Library", "-o", "-name", "node_modules", "-o", "-name", ".cache", ")", "-prune",
			"-o", "-type", "d", "-name", ".git", "-print")
	}
	out, _ := cmd.Output() // best effort: timeout/non-zero exit still yields partial output
	return reposFromGitDirs(string(out))
}

// reposFromGitDirs turns finder output (one ".git" path per line) into repository
// roots: strip the trailing "/.git", de-duplicate, and sort.
func reposFromGitDirs(output string) []string {
	seen := make(map[string]bool)
	var repos []string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// fd prints directories with a trailing slash; trim it so Dir yields the
		// repo root (".../repo/.git/" and ".../repo/.git" both -> ".../repo").
		repo := filepath.Dir(strings.TrimRight(line, "/"))
		if !seen[repo] {
			seen[repo] = true
			repos = append(repos, repo)
		}
	}
	sort.Strings(repos)
	return repos
}

// filterRepos returns repos whose path contains query (case-insensitive),
// capped to a sensible number of rows. An empty query returns the first rows.
func filterRepos(repos []string, query string) []string {
	const maxRows = 30
	query = strings.ToLower(strings.TrimSpace(query))
	var out []string
	for _, r := range repos {
		if query == "" || strings.Contains(strings.ToLower(r), query) {
			out = append(out, r)
			if len(out) >= maxRows {
				break
			}
		}
	}
	return out
}
