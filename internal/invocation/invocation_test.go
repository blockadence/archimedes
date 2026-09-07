package invocation

import "testing"

func TestNameIsTheBinaryByDefault(t *testing.T) {
	t.Setenv("GH_EXTENSION", "")

	if got := Name(); got != "archimedes" {
		t.Errorf("Name() = %q, want archimedes", got)
	}
}

func TestNameGoesThroughGhWhenGhDispatchedUs(t *testing.T) {
	t.Setenv("GH_EXTENSION", "1")

	if got := Name(); got != "gh archimedes" {
		t.Errorf("Name() = %q, want gh archimedes", got)
	}
}
