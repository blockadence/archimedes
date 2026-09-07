package dossier

import (
	"fmt"
	"os"
)

// stubTemplate is the dossier bootstrap scaffolds for a newly discovered
// repo: a skeleton of the sections a context-mapping pass fills in, with
// "House rules" (standing directives) and "Known gotchas" (surprising
// facts) deliberately kept as two distinct sections.
//
// Kept byte-identical to lib.sh's write_dossier_stub heredoc;
// TestStubMatchesLibSh fails if the two drift apart.
const stubTemplate = `# %s

**Path:** %s
**Base branch:** %s
**Depends on:** TBD
**Depended on by:** TBD

## Branching
TBD, fill in during the context-mapping / dossier pass.

## Release procedure
TBD

%s
%s

## Known gotchas
TBD
`

// Stub is the little bootstrap knows about a repo at discovery time, and
// so all a scaffolded dossier can state before anyone has looked at it.
type Stub struct {
	Name       string
	Path       string
	BaseBranch string
}

// WriteStub scaffolds s's dossier under dossierDir, creating that directory
// if needed. A dossier that already exists is left untouched — it is
// hand-written prose, and bootstrap revisits every discovered repo on every
// run — so the return value reports whether a file was actually written.
func WriteStub(dossierDir string, s Stub) (bool, error) {
	path := Path(dossierDir, s.Name)
	if _, err := os.Stat(path); err == nil {
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, err
	}

	if err := os.MkdirAll(dossierDir, 0o755); err != nil {
		return false, err
	}

	body := fmt.Sprintf(stubTemplate, s.Name, s.Path, s.BaseBranch, HouseRulesHeading, houseRulesStubBody)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return false, err
	}

	return true, nil
}
