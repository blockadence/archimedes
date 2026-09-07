package bootstrap

import (
	"reflect"
	"testing"
)

func TestParseRepoList(t *testing.T) {
	got, err := parseRepoList([]byte(`[
	  {"name":"service-a","sshUrl":"git@github.com:acme/service-a.git","defaultBranchRef":{"name":"trunk"},"isArchived":false,"isFork":false},
	  {"name":"service-b","sshUrl":"git@github.com:acme/service-b.git","defaultBranchRef":null,"isArchived":true,"isFork":true}
	]`))
	if err != nil {
		t.Fatalf("parseRepoList: %v", err)
	}

	want := []OrgRepo{
		{Name: "service-a", SSHURL: "git@github.com:acme/service-a.git", DefaultBranchRef: &branchRef{Name: "trunk"}},
		{Name: "service-b", SSHURL: "git@github.com:acme/service-b.git", IsArchived: true, IsFork: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestParseRepoListMalformed(t *testing.T) {
	if _, err := parseRepoList([]byte("not json")); err == nil {
		t.Fatal("expected an error for malformed gh output, got nil")
	}
}

// A repo with no default branch (an empty repo, or a gh response that
// omits the field) still needs a base branch to record.
func TestBaseBranch(t *testing.T) {
	if got := (OrgRepo{DefaultBranchRef: &branchRef{Name: "develop"}}).BaseBranch(); got != "develop" {
		t.Errorf("got %q, want develop", got)
	}
	if got := (OrgRepo{}).BaseBranch(); got != "main" {
		t.Errorf("got %q, want main", got)
	}
}

func TestFilter(t *testing.T) {
	all := []OrgRepo{
		{Name: "plain"},
		{Name: "archived", IsArchived: true},
		{Name: "fork", IsFork: true},
		{Name: "archived-fork", IsArchived: true, IsFork: true},
	}

	cases := []struct {
		name            string
		includeArchived bool
		want            []string
	}{
		{"forks are never ours to orchestrate", false, []string{"plain"}},
		{"archived repos are opt-in", true, []string{"plain", "archived"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got []string
			for _, r := range filter(all, tc.includeArchived) {
				got = append(got, r.Name)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}
