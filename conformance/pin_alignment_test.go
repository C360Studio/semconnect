package conformance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	semconfig "github.com/c360studio/semstreams/config"
)

const (
	semstreamsModule     = "github.com/c360studio/semstreams"
	semstreamsRepository = "https://github.com/C360Studio/semstreams.git"
	semstreamsVersion    = "v1.0.0-beta.160"
	semstreamsTagObject  = "8403a2218000e45a31c5132fbfe01af42ed04f14"
	semstreamsCommit     = "8403a2218000e45a31c5132fbfe01af42ed04f14"
	semstreamsTree       = "9ed5dd3792bca63ce87ebf449a180add918f59ed"
)

func TestBeta160ConformanceConfigurationValidates(t *testing.T) {
	const path = "compose.semstreams.config.json"
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var direct semconfig.Config
	if err := json.Unmarshal(data, &direct); err != nil {
		t.Fatalf("decode beta.160 config: %v", err)
	}
	loaded, err := semconfig.NewLoader().LoadFile(path)
	if err != nil {
		t.Fatalf("load beta.160 config: %v", err)
	}
	if err := loaded.Validate(); err != nil {
		t.Fatalf("validate beta.160 config: %v", err)
	}
}

func TestSemStreamsPinsAreAligned(t *testing.T) {
	t.Parallel()

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source path")
	}
	repositoryRoot := filepath.Dir(filepath.Dir(filename))

	goMod := readFile(t, filepath.Join(repositoryRoot, "go.mod"))
	requirements := activeModuleRequirements(goMod, semstreamsModule)
	if len(requirements) != 1 || requirements[0] != semstreamsVersion {
		t.Errorf("go.mod active requirements for %q = %q, want exactly [%q]",
			semstreamsModule, requirements, semstreamsVersion)
	}

	goSum := readFile(t, filepath.Join(repositoryRoot, "go.sum"))
	semstreamsChecksums := 0
	for _, line := range strings.Split(goSum, "\n") {
		if strings.HasPrefix(line, semstreamsModule+" ") {
			semstreamsChecksums++
			if !strings.HasPrefix(line, semstreamsModule+" "+semstreamsVersion) {
				t.Errorf("go.sum contains checksum for a different SemStreams version: %q", line)
			}
		}
	}
	if semstreamsChecksums != 2 {
		t.Errorf("go.sum contains %d SemStreams checksum entries, want module and go.mod entries only", semstreamsChecksums)
	}
	for _, suffix := range []string{" ", "/go.mod "} {
		wantPrefix := semstreamsModule + " " + semstreamsVersion + suffix
		if !containsLinePrefix(goSum, wantPrefix) {
			t.Errorf("go.sum does not contain checksum entry beginning %q", wantPrefix)
		}
	}

	etsPin := readFile(t, filepath.Join(repositoryRoot, "conformance", ".ets-pin"))
	assignments := shellAssignments(etsPin)
	for key, want := range map[string]string{
		"SEMSTREAMS_GIT_URL":     semstreamsRepository,
		"SEMSTREAMS_VERSION":     semstreamsVersion,
		"SEMSTREAMS_TAG_OBJECT":  semstreamsTagObject,
		"SEMSTREAMS_COMMIT":      semstreamsCommit,
		"SEMSTREAMS_TREE":        semstreamsTree,
		"SEMSTREAMS_COMMIT_DATE": "2026-08-12",
	} {
		values := assignments[key]
		if len(values) != 1 || values[0] != want {
			t.Errorf("conformance/.ets-pin assignments for %s = %q, want exactly [%q]", key, values, want)
		}
	}
}

func TestActiveModuleRequirementsRejectTextualFalsePositives(t *testing.T) {
	t.Parallel()

	contents := `
// require github.com/c360studio/semstreams v1.0.0-beta.159
require (
	github.com/c360studio/semstreams v1.0.0-beta.160
)
require github.com/c360studio/semstreams v1.0.0-beta.149 // duplicate active requirement
`

	got := activeModuleRequirements(contents, semstreamsModule)
	if len(got) != 2 || got[0] != semstreamsVersion || got[1] != "v1.0.0-beta.149" {
		t.Fatalf("active requirements = %q, want beta.160 and beta.149 without commented occurrence", got)
	}
}

func TestShellAssignmentsPreserveDuplicateEffectivePins(t *testing.T) {
	t.Parallel()

	assignments := shellAssignments(`
# SEMSTREAMS_VERSION=v1.0.0-beta.149
SEMSTREAMS_VERSION=v1.0.0-beta.160
SEMSTREAMS_VERSION=v1.0.0-beta.150
`)
	got := assignments["SEMSTREAMS_VERSION"]
	if len(got) != 2 || got[0] != semstreamsVersion || got[1] != "v1.0.0-beta.150" {
		t.Fatalf("assignments = %q, want both active values without commented occurrence", got)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(contents)
}

func containsLinePrefix(contents, prefix string) bool {
	for _, line := range strings.Split(contents, "\n") {
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}
	return false
}

func activeModuleRequirements(contents, module string) []string {
	var versions []string
	inRequireBlock := false
	for _, rawLine := range strings.Split(contents, "\n") {
		line := strings.TrimSpace(strings.SplitN(rawLine, "//", 2)[0])
		switch line {
		case "require (":
			inRequireBlock = true
			continue
		case ")":
			inRequireBlock = false
			continue
		case "":
			continue
		}

		fields := strings.Fields(line)
		if inRequireBlock && len(fields) >= 2 && fields[0] == module {
			versions = append(versions, fields[1])
			continue
		}
		if !inRequireBlock && len(fields) >= 3 && fields[0] == "require" && fields[1] == module {
			versions = append(versions, fields[2])
		}
	}
	return versions
}

func shellAssignments(contents string) map[string][]string {
	assignments := make(map[string][]string)
	for _, line := range strings.Split(contents, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		assignments[key] = append(assignments[key], strings.TrimSpace(value))
	}
	return assignments
}
