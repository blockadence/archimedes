package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/blockadence/archimedes/cli/internal/contextmap"
	"github.com/blockadence/archimedes/cli/internal/mcpserver"
	"github.com/blockadence/archimedes/cli/internal/spawn"
	"github.com/blockadence/archimedes/cli/internal/status"
)

// These tests are the second acceptance criterion of the MCP server: a tool
// call must report what the equivalent CLI command reports. Each one asks
// the same instance the same question both ways and compares the answers,
// so a future change to either path that only moves one of them fails here.

// mcpSession connects a client to a server over the same options a
// serve-mcp invocation would build, and returns the session.
func mcpSession(t *testing.T, opts mcpserver.Options) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := mcpserver.New(opts).Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("connecting the server: %v", err)
	}
	t.Cleanup(func() { _ = serverSession.Wait() })

	clientSession, err := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v0"}, nil).
		Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connecting the client: %v", err)
	}
	t.Cleanup(func() { _ = clientSession.Close() })

	return clientSession
}

// callTool invokes one tool and decodes its structured result into out.
func callTool(t *testing.T, cs *mcp.ClientSession, name string, args, out any) {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("calling %s: %v", name, err)
	}
	if res.IsError {
		t.Fatalf("tool %s reported an error: %#v", name, res.Content)
	}
	data, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("re-marshaling %s structured content: %v", name, err)
	}
	if err := json.Unmarshal(data, out); err != nil {
		t.Fatalf("decoding %s structured content: %v\n%s", name, err, data)
	}
}

func TestRootRegistersServeMCP(t *testing.T) {
	for _, c := range newRootCmd().Commands() {
		if c.Name() == "serve-mcp" {
			return
		}
	}
	t.Fatal("serve-mcp is not registered on the command tree")
}

// The CLI keeps its own flags and env vars; serve-mcp reads the same ones,
// so a tool call lands on the configuration the subcommands would have used.
func TestServeMCPOptionsCarryTheSameConfiguration(t *testing.T) {
	env := map[string]string{
		"ARCHIMEDES_MAX_STREAMS":  "7",
		"ARCHIMEDES_DRIVER":       "spec-kit",
		"ARCHIMEDES_DRIVERS_DIR":  "/elsewhere/drivers",
		"ARCHIMEDES_CONTEXT_FILE": "MAP.md",
	}
	opts := serveMCPOptions("/instance", func(k string) string { return env[k] }, io.Discard)

	if opts.Root != "/instance" {
		t.Errorf("root = %q, want the flag's value", opts.Root)
	}
	if opts.MaxStreams != "7" {
		t.Errorf("max streams = %q, want the environment's value handed over unparsed", opts.MaxStreams)
	}
	if want := contextMapOptions("/instance", false, func(k string) string { return env[k] }); opts.ContextMap != want {
		t.Errorf("context-map options mismatch\n got: %#v\nwant: %#v", opts.ContextMap, want)
	}
	if opts.Progress == nil {
		t.Error("progress stream is nil; git's output would land on the protocol stream")
	}
}

func TestRepoStatusToolMatchesTheStatusCommand(t *testing.T) {
	dir := writeInstanceFixture(t)
	t.Setenv(maxStreamsEnvVar, "1")

	var buf bytes.Buffer
	if err := runStatus(&buf, dir, "", true, mergedBaseSources()); err != nil {
		t.Fatalf("runStatus: %v", err)
	}
	var fromCLI status.Report
	if err := json.Unmarshal(buf.Bytes(), &fromCLI); err != nil {
		t.Fatalf("decoding the command's JSON: %v\n%s", err, buf.String())
	}

	cs := mcpSession(t, mcpserver.Options{Root: dir, Status: mergedBaseSources(), MaxStreams: os.Getenv(maxStreamsEnvVar)})
	var fromMCP status.Report
	callTool(t, cs, "repo_status", map[string]any{}, &fromMCP)

	if !reflect.DeepEqual(fromCLI, fromMCP) {
		t.Errorf("the tool and the command disagree about the same instance\n cli: %#v\n mcp: %#v", fromCLI, fromMCP)
	}
	// Guard against both being trivially empty.
	if fromCLI.Count != 2 || !fromCLI.GuardrailHit || !fromCLI.Rows[1].NeedsRebase {
		t.Fatalf("fixture no longer exercises rows, the guardrail and the rebase flag: %#v", fromCLI)
	}
}

func TestRepoStatusToolMatchesTheStatusCommandForOneSlug(t *testing.T) {
	dir := writeInstanceFixture(t)
	writeFile(t, filepath.Join(dir, "work", "other-slug", "status.md"),
		"# other-slug\n\n| repo | branch | worktree | note | pr |\n|---|---|---|---|---|\n"+
			"| service-a | other-slug | /wt/service-a-2 | based on main | - |\n")

	var buf bytes.Buffer
	if err := runStatus(&buf, dir, "other-slug", true, plainSources()); err != nil {
		t.Fatalf("runStatus: %v", err)
	}
	var fromCLI status.Report
	if err := json.Unmarshal(buf.Bytes(), &fromCLI); err != nil {
		t.Fatalf("decoding the command's JSON: %v", err)
	}

	cs := mcpSession(t, mcpserver.Options{Root: dir, Status: plainSources()})
	var fromMCP status.Report
	callTool(t, cs, "repo_status", map[string]any{"slug": "other-slug"}, &fromMCP)

	if !reflect.DeepEqual(fromCLI, fromMCP) {
		t.Errorf("slug-filtered reports disagree\n cli: %#v\n mcp: %#v", fromCLI, fromMCP)
	}
	if fromCLI.Count != 1 {
		t.Fatalf("fixture no longer narrows to one row: %#v", fromCLI)
	}
}

// mcpInstance builds an instance with two real repos, one depending on the
// other, so the tools that fetch and spawn have remote state to work with.
func mcpInstance(t *testing.T) (root string, repos map[string]string) {
	t.Helper()
	tmp := t.TempDir()

	repos = map[string]string{}
	for _, name := range []string{"app", "shared"} {
		repo := filepath.Join(tmp, name)
		run(t, tmp, "git", "init", "-q", "--bare", "-b", "main", filepath.Join(tmp, name+".git"))
		run(t, tmp, "git", "clone", "-q", filepath.Join(tmp, name+".git"), repo)
		writeFile(t, filepath.Join(repo, "README.md"), "# "+name+"\n")
		run(t, repo, "git", "add", "-A")
		run(t, repo, "git", "-c", "user.email=t@t", "-c", "user.name=t", "commit", "-q", "-m", "init")
		run(t, repo, "git", "push", "-q", "origin", "main")
		repos[name] = repo
	}

	root = filepath.Join(tmp, "instance")
	writeFile(t, filepath.Join(root, "repos.yaml"),
		"repos:\n"+
			"  - name: app\n    path: ../app\n    base_branch: main\n    depends_on: [shared]\n"+
			"  - name: shared\n    path: ../shared\n    base_branch: main\n")

	return root, repos
}

func TestContextMapStatusToolMatchesTheDryRun(t *testing.T) {
	root, repos := mcpInstance(t)
	// shared is mapped and current; app has never been mapped.
	writeFile(t, filepath.Join(repos["shared"], "CONTEXT.md"), "# shared\n")
	sha := strings.TrimSpace(runOut(t, repos["shared"], "git", "rev-parse", "origin/main"))
	writeFile(t, filepath.Join(root, "repos.yaml"),
		"repos:\n"+
			"  - name: app\n    path: ../app\n    base_branch: main\n    depends_on: [shared]\n"+
			"  - name: shared\n    path: ../shared\n    base_branch: main\n    context_modeled_sha: "+sha+"\n")

	fromCLI := execute(t, "context-map", "--root", root, "--dry-run")

	cs := mcpSession(t, mcpserver.Options{Root: root})
	var fromMCP contextmap.Plan
	callTool(t, cs, "context_map_status", map[string]any{}, &fromMCP)

	if want := "Planned order: " + strings.Join(fromMCP.Order, " "); !strings.Contains(fromCLI, want) {
		t.Errorf("the tool's order %v isn't the order the dry run planned:\n%s", fromMCP.Order, fromCLI)
	}
	for _, r := range fromMCP.Repos {
		want := "Up to date: " + r.Name + " (@ " + contextmap.Short(r.CurrentSHA) + ")"
		if r.Stale {
			want = "=== " + r.Name + " (" + r.Reason + ") ==="
		}
		if !strings.Contains(fromCLI, want) {
			t.Errorf("the tool reports %#v, which the dry run doesn't say:\nwant line %q in\n%s", r, want, fromCLI)
		}
	}
	// Guard against both agreeing on nothing.
	if len(fromMCP.Repos) != 2 || !fromMCP.Stale() || fromMCP.Repos[0].Stale {
		t.Fatalf("fixture no longer contrasts a current repo with a stale one: %#v", fromMCP.Repos)
	}
}

func TestSpawnWorktreeToolMatchesTheSpawnCommand(t *testing.T) {
	root, repos := mcpInstance(t)

	execute(t, "spawn", "--root", root, "via-cli", "app")

	cs := mcpSession(t, mcpserver.Options{Root: root})
	var result spawn.Result
	callTool(t, cs, "spawn_worktree", map[string]any{"slug": "via-mcp", "repo": "app"}, &result)

	// Same branch, same worktree layout, same recorded row — the only
	// difference between the two units of work is the name they were asked
	// for under.
	if got := runOut(t, repos["app"], "git", "worktree", "list"); !strings.Contains(got, result.Worktree) {
		t.Errorf("the tool's worktree %q isn't one git knows about:\n%s", result.Worktree, got)
	}
	cliRow := readStatus(t, root, "via-cli")
	mcpRow := readStatus(t, root, "via-mcp")
	if normalized := strings.ReplaceAll(mcpRow, "via-mcp", "via-cli"); normalized != cliRow {
		t.Errorf("the two ways in recorded different rows\n cli: %q\n mcp: %q", cliRow, normalized)
	}
	if !strings.Contains(cliRow, "based on main") {
		t.Fatalf("fixture no longer records a start point: %q", cliRow)
	}
}

func readStatus(t *testing.T, root, slug string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, "work", slug, "status.md"))
	if err != nil {
		t.Fatalf("reading %s's status file: %v", slug, err)
	}
	return string(data)
}

// runOut runs a command in dir and returns its stdout.
func runOut(t *testing.T, dir string, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("%s %v (in %s): %v", name, args, dir, err)
	}
	return string(out)
}

// A guardrail of zero is a real setting, not an unset one: every open
// worktree trips it. Parsing it in one place is what keeps the two ways in
// from reading the same environment as two different thresholds.
func TestRepoStatusToolMatchesTheStatusCommandAtAZeroGuardrail(t *testing.T) {
	dir := writeInstanceFixture(t)
	t.Setenv(maxStreamsEnvVar, "0")

	var buf bytes.Buffer
	if err := runStatus(&buf, dir, "", true, plainSources()); err != nil {
		t.Fatalf("runStatus: %v", err)
	}
	var fromCLI status.Report
	if err := json.Unmarshal(buf.Bytes(), &fromCLI); err != nil {
		t.Fatalf("decoding the command's JSON: %v", err)
	}

	cs := mcpSession(t, mcpserver.Options{Root: dir, Status: plainSources(), MaxStreams: os.Getenv(maxStreamsEnvVar)})
	var fromMCP status.Report
	callTool(t, cs, "repo_status", map[string]any{}, &fromMCP)

	if !reflect.DeepEqual(fromCLI, fromMCP) {
		t.Errorf("a zero guardrail means different things to the two ways in\n cli: %#v\n mcp: %#v", fromCLI, fromMCP)
	}
	if fromCLI.GuardrailMax != 0 || !fromCLI.GuardrailHit {
		t.Fatalf("the command no longer treats 0 as a threshold of zero: %#v", fromCLI)
	}
}

// The server is started by an agent tool from a working directory it never
// agreed on, so a relative --root must not leak into the paths it reports:
// a client can't open "../app".
func TestToolsReportPathsAClientCanOpenFromARelativeRoot(t *testing.T) {
	root, repos := mcpInstance(t)
	t.Chdir(filepath.Dir(root))

	cs := mcpSession(t, mcpserver.Options{Root: filepath.Base(root)})

	var listed mcpserver.Repos
	callTool(t, cs, "list_repos", map[string]any{}, &listed)
	if !filepath.IsAbs(listed.Root) {
		t.Errorf("instance root %q is relative", listed.Root)
	}
	for _, r := range listed.Repos {
		if r.Checkout != repos[r.Name] {
			t.Errorf("%s checkout = %q, want the absolute path %q", r.Name, r.Checkout, repos[r.Name])
		}
	}

	// The other tools resolve the same root, so they must agree with it.
	var plan contextmap.Plan
	callTool(t, cs, "context_map_status", map[string]any{}, &plan)
	for _, r := range plan.Repos {
		if r.Path != repos[r.Name] {
			t.Errorf("%s path = %q, want the absolute path %q", r.Name, r.Path, repos[r.Name])
		}
	}

	var spawned spawn.Result
	callTool(t, cs, "spawn_worktree", map[string]any{"slug": "new-thing", "repo": "app"}, &spawned)
	if !filepath.IsAbs(spawned.Worktree) {
		t.Errorf("worktree %q is relative", spawned.Worktree)
	}
}
