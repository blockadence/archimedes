package contextmap_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/blockadence/archimedes/cli/internal/contextmap"
	"github.com/blockadence/archimedes/cli/internal/manifest"
)

func repos(specs ...[]string) []manifest.Repo {
	out := make([]manifest.Repo, len(specs))
	for i, s := range specs {
		out[i] = manifest.Repo{Name: s[0], DependsOn: s[1:]}
	}
	return out
}

func TestOrderPutsDependenciesFirst(t *testing.T) {
	order, warning := contextmap.Order(repos(
		[]string{"app", "lib", "shared"},
		[]string{"lib", "shared"},
		[]string{"shared"},
	))

	if warning != "" {
		t.Errorf("warning = %q, want none", warning)
	}
	want := []string{"shared", "lib", "app"}
	if !reflect.DeepEqual(order, want) {
		t.Errorf("order = %v, want %v", order, want)
	}
}

func TestOrderKeepsDeclaredOrderAmongIndependentRepos(t *testing.T) {
	order, warning := contextmap.Order(repos(
		[]string{"c"},
		[]string{"a"},
		[]string{"b"},
	))

	if warning != "" {
		t.Errorf("warning = %q, want none", warning)
	}
	want := []string{"c", "a", "b"}
	if !reflect.DeepEqual(order, want) {
		t.Errorf("order = %v, want %v (declared order preserved when nothing constrains it)", order, want)
	}
}

// The shell version marks a repo done as soon as it's ordered, within the
// same pass — so a repo declared after its dependency is ready immediately
// rather than waiting for the next pass. Same order, either way; this pins
// the behavior rather than a reimplementation's incidental one.
func TestOrderSatisfiesDependenciesOrderedEarlierInTheSamePass(t *testing.T) {
	order, _ := contextmap.Order(repos(
		[]string{"shared"},
		[]string{"lib", "shared"},
		[]string{"app", "lib"},
	))

	want := []string{"shared", "lib", "app"}
	if !reflect.DeepEqual(order, want) {
		t.Errorf("order = %v, want %v", order, want)
	}
}

func TestOrderFallsBackToDeclaredOrderOnACycle(t *testing.T) {
	order, warning := contextmap.Order(repos(
		[]string{"ok"},
		[]string{"a", "b"},
		[]string{"b", "a"},
	))

	want := []string{"ok", "a", "b"}
	if !reflect.DeepEqual(order, want) {
		t.Errorf("order = %v, want %v (cycle members appended in declared order)", order, want)
	}
	for _, fragment := range []string{"Cycle or unresolved dependency among", "a", "b", "Falling back to declared order"} {
		if !strings.Contains(warning, fragment) {
			t.Errorf("warning %q missing %q", warning, fragment)
		}
	}
	if strings.Contains(warning, "ok") {
		t.Errorf("warning %q names a repo that ordered fine", warning)
	}
}

func TestOrderTreatsAnUnknownDependencyAsUnresolved(t *testing.T) {
	order, warning := contextmap.Order(repos([]string{"app", "never-declared"}))

	if want := []string{"app"}; !reflect.DeepEqual(order, want) {
		t.Errorf("order = %v, want %v", order, want)
	}
	if warning == "" {
		t.Error("want a warning naming the unresolved dependency, got none")
	}
}

func TestOrderOfNothing(t *testing.T) {
	order, warning := contextmap.Order(nil)

	if len(order) != 0 {
		t.Errorf("order = %v, want empty", order)
	}
	if warning != "" {
		t.Errorf("warning = %q, want none", warning)
	}
}

func TestAssess(t *testing.T) {
	const current = "abcdef1234567890"

	tests := []struct {
		name       string
		stored     string
		fileExists bool
		wantStale  bool
		wantReason string
	}{
		{
			name:       "current sha with a context file on disk is up to date",
			stored:     current,
			fileExists: true,
			wantStale:  false,
		},
		{
			name:       "no recorded sha means never mapped",
			stored:     "",
			fileExists: false,
			wantStale:  true,
			wantReason: "never mapped",
		},
		{
			name:       "no recorded sha wins even if a context file happens to exist",
			stored:     "",
			fileExists: true,
			wantStale:  true,
			wantReason: "never mapped",
		},
		{
			name:       "a recorded sha with the context file deleted needs remapping",
			stored:     current,
			fileExists: false,
			wantStale:  true,
			wantReason: "CONTEXT.md missing",
		},
		{
			name:       "an older recorded sha is stale",
			stored:     "0123456789abcdef",
			fileExists: true,
			wantStale:  true,
			wantReason: "stale, 01234567 -> abcdef12",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := contextmap.Assess(tt.stored, current, "CONTEXT.md", tt.fileExists)
			if got.Stale != tt.wantStale {
				t.Errorf("Stale = %v, want %v", got.Stale, tt.wantStale)
			}
			if got.Reason != tt.wantReason {
				t.Errorf("Reason = %q, want %q", got.Reason, tt.wantReason)
			}
		})
	}
}

func TestSelectDriver(t *testing.T) {
	tests := []struct {
		name                        string
		repo, instance, environment string
		want                        string
	}{
		{name: "nothing configured stays interactive"},
		{name: "environment default when repos.yaml sets none", environment: "env-driver", want: "env-driver"},
		{name: "instance-wide default from repos.yaml", instance: "instance-driver", want: "instance-driver"},
		{
			name:        "repos.yaml's instance default wins over the environment",
			instance:    "instance-driver",
			environment: "env-driver",
			want:        "instance-driver",
		},
		{
			name:        "a repo's own driver overrides the instance default",
			repo:        "repo-driver",
			instance:    "instance-driver",
			environment: "env-driver",
			want:        "repo-driver",
		},
		{name: "a repo's own driver applies with no default set", repo: "repo-driver", want: "repo-driver"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := contextmap.SelectDriver(tt.repo, tt.instance, tt.environment); got != tt.want {
				t.Errorf("SelectDriver(%q, %q, %q) = %q, want %q", tt.repo, tt.instance, tt.environment, got, tt.want)
			}
		})
	}
}
