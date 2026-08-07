package app

import "testing"

func TestParseRepository(t *testing.T) {
	owner, repo, err := parseRepository("owner/repo")
	if err != nil || owner != "owner" || repo != "repo" {
		t.Fatalf("owner = %q, repo = %q, err = %v", owner, repo, err)
	}
	if _, _, err := parseRepository("invalid"); err == nil {
		t.Fatal("expected invalid repository error")
	}
}

func TestParsePullRequestNumber(t *testing.T) {
	number, err := ParsePullRequestNumber("42")
	if err != nil || number != 42 {
		t.Fatalf("number = %d, err = %v", number, err)
	}
	if _, err := ParsePullRequestNumber("0"); err == nil {
		t.Fatal("expected invalid pull request number error")
	}
}
