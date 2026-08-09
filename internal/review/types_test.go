package review

import "testing"

func TestPullRequestIsTeamAuthored(t *testing.T) {
	tests := []struct {
		association string
		want        bool
	}{
		{association: "MEMBER", want: true},
		{association: "OWNER", want: true},
		{association: "COLLABORATOR", want: false},
		{association: "CONTRIBUTOR", want: false},
		{association: "FIRST_TIME_CONTRIBUTOR", want: false},
		{association: "NONE", want: false},
		{association: "", want: false},
	}

	for _, test := range tests {
		t.Run(test.association, func(t *testing.T) {
			pull := PullRequest{AuthorAssociation: test.association}
			if got := pull.IsTeamAuthored(); got != test.want {
				t.Fatalf("IsTeamAuthored() = %v, want %v", got, test.want)
			}
		})
	}
}
