package tui

import (
	"reflect"
	"testing"
)

func TestReposFromGitDirs(t *testing.T) {
	in := "/Users/x/workspace/tea-server/.git\n" + // no trailing slash (find style)
		"/Users/x/workspace/tea-timer/.git/\n" + // trailing slash (fd style)
		"\n" + // blank line ignored
		"/Users/x/workspace/tea-server/.git\n" // duplicate ignored

	got := reposFromGitDirs(in)
	want := []string{
		"/Users/x/workspace/tea-server",
		"/Users/x/workspace/tea-timer",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("reposFromGitDirs:\n got=%v\nwant=%v", got, want)
	}
}

func TestFilterRepos(t *testing.T) {
	repos := []string{
		"/Users/x/workspace/tea-server",
		"/Users/x/workspace/tea-timer",
		"/Users/x/other/app",
	}
	got := filterRepos(repos, "tea-")
	want := []string{
		"/Users/x/workspace/tea-server",
		"/Users/x/workspace/tea-timer",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("filterRepos: got=%v want=%v", got, want)
	}
	if all := filterRepos(repos, "  "); len(all) != 3 {
		t.Fatalf("empty query should return all, got %d", len(all))
	}
}
