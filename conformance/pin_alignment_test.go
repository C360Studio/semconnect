package conformance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	semconfig "github.com/c360studio/semstreams/config"
)

const (
	semstreamsModule     = "github.com/c360studio/semstreams"
	semstreamsRepository = "https://github.com/C360Studio/semstreams.git"
	semstreamsVersion    = "v1.0.0-beta.162.0.20260930150212-8b99efe9c66a"
	semstreamsTagObject  = "8b99efe9c66a4faa4fa509f9f62cc6bad8392128"
	semstreamsCommit     = "8b99efe9c66a4faa4fa509f9f62cc6bad8392128"
	semstreamsTree       = "605f83cb8492eda3bd347ac30a211b9babf3931f"
)

func TestSetup03AConformanceConfigurationValidates(t *testing.T) {
	const path = "compose.semstreams.config.json"
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var direct semconfig.Config
	if err := json.Unmarshal(data, &direct); err != nil {
		t.Fatalf("decode SETUP 03A config: %v", err)
	}
	loaded, err := semconfig.NewLoader().LoadFile(path)
	if err != nil {
		t.Fatalf("load SETUP 03A config: %v", err)
	}
	if loaded.Platform.Org != "c360" || loaded.Platform.ID != "semconnect" {
		t.Fatalf("graph authority must preserve existing conformance IDs: %+v", loaded.Platform)
	}
	if err := loaded.Validate(); err != nil {
		t.Fatalf("validate SETUP 03A config: %v", err)
	}
}

func TestConformanceUsesConsumerRegisteredGraphHost(t *testing.T) {
	var compose struct {
		Services map[string]struct {
			Command   []string `yaml:"command"`
			Volumes   []string `yaml:"volumes"`
			DependsOn map[string]struct {
				Condition string `yaml:"condition"`
			} `yaml:"depends_on"`
			Build struct {
				Context    string `yaml:"context"`
				Dockerfile string `yaml:"dockerfile"`
			} `yaml:"build"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal([]byte(readFile(t, "compose.yml")), &compose); err != nil {
		t.Fatal(err)
	}
	backend := compose.Services["semstreams-backend"]
	if strings.Join(backend.Command, " ") != "-config /etc/semstreams/config.json -cs-api-config /etc/cs-api-server/config.json" {
		t.Fatalf("conformance host must share CS API resource configuration: %v", backend.Command)
	}
	if !strings.Contains(strings.Join(backend.Volumes, "\n"), "./compose.cs-api.config.json:/etc/cs-api-server/config.json:ro") {
		t.Fatal("conformance graph host lacks shared CS API config mount")
	}
	if backend.DependsOn["provision-identity"].Condition != "service_completed_successfully" {
		t.Fatal("conformance graph startup must wait for explicit identity provisioning")
	}
	provision := compose.Services["provision-identity"]
	if strings.Join(provision.Command, " ") != "-config /etc/semstreams/config.json -provision-identity semconnect" {
		t.Fatalf("conformance identity provisioning command = %v", provision.Command)
	}
	if backend.Build.Context != ".." || backend.Build.Dockerfile != "deploy/backend.Dockerfile" {
		t.Fatalf("conformance graph host must include consumer payload registrations: %+v", backend.Build)
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
		"SEMSTREAMS_COMMIT_DATE": "2026-09-30",
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
	github.com/c360studio/semstreams v1.0.0-beta.162.0.20260930150212-8b99efe9c66a
)
require github.com/c360studio/semstreams v1.0.0-beta.149 // duplicate active requirement
`

	got := activeModuleRequirements(contents, semstreamsModule)
	if len(got) != 2 || got[0] != semstreamsVersion || got[1] != "v1.0.0-beta.149" {
		t.Fatalf("active requirements = %q, want frozen target and beta.149 without commented occurrence", got)
	}
}

func TestShellAssignmentsPreserveDuplicateEffectivePins(t *testing.T) {
	t.Parallel()

	assignments := shellAssignments(`
# SEMSTREAMS_VERSION=v1.0.0-beta.149
SEMSTREAMS_VERSION=v1.0.0-beta.162.0.20260930150212-8b99efe9c66a
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
