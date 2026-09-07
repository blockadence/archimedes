package mcpserver_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/blockadence/gh-archimedes/internal/manifest"
	"github.com/blockadence/gh-archimedes/internal/mcpserver"
	"github.com/blockadence/gh-archimedes/internal/testrepo"
)

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// makeRepo builds a bare "origin" plus a clone with one commit on main, so
// the tools that fetch and spawn have real remote state to work against.
func makeRepo(t *testing.T, tmp, name string) string {
	t.Helper()
	return testrepo.New(t, testrepo.Spec{Dir: tmp, Name: name}).Clone
}

const reposYAML = `repos:
  - name: app
    path: ../app
    base_branch: main
    depends_on: [shared]
    context_modeled_sha: null
    convention_pack: go
  - name: shared
    path: ../shared
    base_branch: main
    depends_on: []
    context_modeled_sha: null
`

// instance is an Archimedes instance root plus the sibling repos its
// repos.yaml points at — the "../<name>" layout bootstrap produces — with
// one unit of work already spawned across both.
type instance struct {
	root      string
	repoPaths map[string]string
}

func newInstance(t *testing.T) instance {
	t.Helper()
	tmp := t.TempDir()

	inst := instance{root: filepath.Join(tmp, "instance"), repoPaths: map[string]string{}}
	for _, name := range []string{"app", "shared"} {
		inst.repoPaths[name] = makeRepo(t, tmp, name)
	}
	mustWriteFile(t, filepath.Join(inst.root, "repos.yaml"), reposYAML)
	mustWriteFile(t, filepath.Join(inst.root, "work", "widget-fix", "status.md"),
		"# widget-fix\n\n| repo | branch | worktree | note | pr |\n|---|---|---|---|---|\n"+
			"| app | widget-fix | /wt/app | based on main | - |\n"+
			"| shared | widget-fix | /wt/shared | stacked on app:auth-api | - |\n")

	return inst
}

// connect starts a server over an in-memory transport and returns a client
// session speaking to it, the way a real MCP client would.
func connect(t *testing.T, opts mcpserver.Options) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := mcpserver.New(opts).Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("connecting the server: %v", err)
	}
	t.Cleanup(func() { _ = serverSession.Wait() })

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v0"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connecting the client: %v", err)
	}
	t.Cleanup(func() { _ = clientSession.Close() })

	return clientSession
}

// call invokes one tool and decodes its structured result into out, failing
// the test if the tool reported an error.
func call(t *testing.T, cs *mcp.ClientSession, name string, args, out any) {
	t.Helper()
	res := callRaw(t, cs, name, args)
	if res.IsError {
		t.Fatalf("tool %s reported an error: %s", name, resultText(res))
	}
	if out == nil {
		return
	}
	data, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("re-marshaling %s structured content: %v", name, err)
	}
	if err := json.Unmarshal(data, out); err != nil {
		t.Fatalf("decoding %s structured content: %v\n%s", name, err, data)
	}
}

// callRaw invokes one tool without asserting anything about the outcome,
// for the tests that are about the failure path.
func callRaw(t *testing.T, cs *mcp.ClientSession, name string, args any) *mcp.CallToolResult {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("calling %s: %v", name, err)
	}
	return res
}

func resultText(res *mcp.CallToolResult) string {
	var joined string
	for _, c := range res.Content {
		if text, ok := c.(*mcp.TextContent); ok {
			joined += text.Text
		}
	}
	return joined
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(data)
}

// setSHA records a repo as having been mapped at its base branch's current
// commit, the bookkeeping a completed mapping pass leaves behind.
func setSHA(t *testing.T, i instance, name string) {
	t.Helper()
	sha := testrepo.GitOut(t, i.repoPaths[name], "rev-parse", "origin/main")
	if err := manifest.SetRepoField(filepath.Join(i.root, "repos.yaml"), name, manifest.FieldContextModeledSHA, sha); err != nil {
		t.Fatal(err)
	}
}
