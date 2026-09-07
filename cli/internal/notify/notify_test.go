package notify_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blockadence/archimedes/cli/internal/notify"
)

var (
	staleApp    = notify.Event{Kind: notify.ContextStale, Subject: "app", Detail: "never mapped", Remedy: "archimedes context-map"}
	staleShared = notify.Event{Kind: notify.ContextStale, Subject: "shared", Detail: "stale, aaaaaaaa -> bbbbbbbb", Remedy: "archimedes context-map"}
	prunable    = notify.Event{Kind: notify.PruneEligible, Subject: "app:widget-fix", Detail: "MERGED", Remedy: "archimedes prune widget-fix --force"}
)

func keys(events []notify.Event) []string {
	out := make([]string, len(events))
	for i, e := range events {
		out[i] = e.Key()
	}
	return out
}

func TestKeyIgnoresDetailSoOneConditionNotifiesOnce(t *testing.T) {
	moved := staleApp
	moved.Detail = "stale, aaaaaaaa -> cccccccc"

	if staleApp.Key() != moved.Key() {
		t.Errorf("keys differ (%q vs %q); a map that is still stale for a newer commit is the same condition",
			staleApp.Key(), moved.Key())
	}
	if staleApp.Key() == prunable.Key() {
		t.Error("different kinds share a key")
	}
}

func TestSinceReturnsOnlyWhatWasNotAlreadyFiring(t *testing.T) {
	previous := notify.StateOf([]notify.Event{staleApp})

	fired := notify.Since(previous, []notify.Event{staleApp, staleShared, prunable})

	if got, want := keys(fired), []string{staleShared.Key(), prunable.Key()}; !equal(got, want) {
		t.Errorf("fired = %v, want %v (app was already firing)", got, want)
	}
}

func TestSinceRefiresAConditionThatClearedAndCameBack(t *testing.T) {
	wasFiring := notify.StateOf([]notify.Event{staleApp})

	// The operator maps app: nothing is firing, so nothing is recorded.
	cleared := notify.StateOf(notify.Since(wasFiring, nil))
	// Its base branch moves again.
	fired := notify.Since(cleared, []notify.Event{staleApp})

	if got := keys(fired); len(got) != 1 || got[0] != staleApp.Key() {
		t.Errorf("fired = %v, want app to notify again after going stale a second time", got)
	}
}

func TestStateRoundTripsThroughDisk(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notify.json")
	if err := notify.SaveState(path, notify.StateOf([]notify.Event{staleApp, prunable})); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	loaded, err := notify.LoadState(path)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}

	if fired := notify.Since(loaded, []notify.Event{staleApp, prunable}); len(fired) != 0 {
		t.Errorf("fired = %v across a restart, want nothing: the file is the memory a daemon would hold", keys(fired))
	}
	if got := notify.Since(loaded, []notify.Event{staleShared}); len(got) != 1 {
		t.Errorf("fired = %v, want the one condition the recorded state hadn't seen", keys(got))
	}
}

func TestLoadStateOfAMissingFileIsAFirstRunNotAnError(t *testing.T) {
	state, err := notify.LoadState(filepath.Join(t.TempDir(), "never-written.json"))
	if err != nil {
		t.Fatalf("LoadState of a missing file: %v", err)
	}

	if fired := notify.Since(state, []notify.Event{staleApp}); len(fired) != 1 {
		t.Errorf("fired = %v, want a first run to report the backlog it finds", keys(fired))
	}
}

func TestLoadStateRefusesAFileItCannotRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notify.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := notify.LoadState(path); err == nil {
		t.Error("an unreadable state file must not be mistaken for a first run, which would re-notify everything")
	}
}

func TestDropForgetsOneConditionSoItFiresAgain(t *testing.T) {
	state := notify.StateOf([]notify.Event{staleApp, prunable})

	state.Drop(staleApp.Key())

	if got := keys(notify.Since(state, []notify.Event{staleApp, prunable})); len(got) != 1 || got[0] != staleApp.Key() {
		t.Errorf("fired = %v, want only the dropped condition", got)
	}
}

func TestToWriterPrintsWhatHappenedAndWhatToRun(t *testing.T) {
	var buf bytes.Buffer

	if err := notify.ToWriter(&buf)(prunable); err != nil {
		t.Fatalf("ToWriter: %v", err)
	}

	out := buf.String()
	for _, want := range []string{"app:widget-fix", "MERGED", "archimedes prune widget-fix --force"} {
		if !strings.Contains(out, want) {
			t.Errorf("printed notification %q missing %q", out, want)
		}
	}
}

func TestToCommandPassesTheEventThroughTheEnvironmentNotTheCommandString(t *testing.T) {
	var gotCommand, gotStdin string
	var gotEnv []string
	run := func(command string, env []string, stdin string) error {
		gotCommand, gotEnv, gotStdin = command, env, stdin
		return nil
	}

	if err := notify.ToCommand(run, "notify-send \"$ARCHIMEDES_EVENT_TITLE\"")(staleApp); err != nil {
		t.Fatalf("ToCommand: %v", err)
	}

	if gotCommand != "notify-send \"$ARCHIMEDES_EVENT_TITLE\"" {
		t.Errorf("command = %q, want the operator's hook verbatim", gotCommand)
	}
	env := map[string]string{}
	for _, kv := range gotEnv {
		k, v, _ := strings.Cut(kv, "=")
		env[k] = v
	}
	if env[notify.EnvKind] != string(notify.ContextStale) || env[notify.EnvSubject] != "app" || env[notify.EnvDetail] != "never mapped" {
		t.Errorf("environment = %v, want the event's fields", env)
	}
	if !strings.Contains(env[notify.EnvTitle], "app") || !strings.Contains(env[notify.EnvMessage], "archimedes context-map") {
		t.Errorf("environment = %v, want a ready-made title and message", env)
	}
	if !strings.Contains(gotStdin, "app") {
		t.Errorf("stdin = %q, want the notification's text for hooks that read it", gotStdin)
	}
}

func TestToCommandReportsAHookThatFailed(t *testing.T) {
	run := func(string, []string, string) error { return os.ErrPermission }

	if err := notify.ToCommand(run, "false")(staleApp); err == nil {
		t.Error("a hook that failed must be reported, so the condition can be delivered again")
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
