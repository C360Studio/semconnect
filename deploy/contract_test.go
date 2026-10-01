package deploy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	semconfig "github.com/c360studio/semstreams/config"
	"github.com/moby/patternmatcher"
	"github.com/moby/patternmatcher/ignorefile"
	"gopkg.in/yaml.v3"
)

const (
	targetVersion  = "v1.0.0-beta.162.0.20260930150212-8b99efe9c66a"
	targetCommit   = "8b99efe9c66a4faa4fa509f9f62cc6bad8392128"
	targetImageTag = "setup03a-8b99efe9c66a"
	natsDigest     = "sha256:f2123f533c2b0cada0a5c5ec434fb2b8cfe1cf220215ef9d7517e1372917ad66"
)

func TestSetup03AConfigurationsValidateWithPinnedSemStreams(t *testing.T) {
	for _, path := range []string{"semstreams.json"} {
		t.Run(filepath.Base(path), func(t *testing.T) {
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
				t.Fatalf("graph authority must preserve canonical CS resource IDs: %+v", loaded.Platform)
			}
			if err := loaded.Validate(); err != nil {
				t.Fatalf("validate SETUP 03A config: %v", err)
			}
		})
	}
}

func TestComposeIsGreenfieldProductionTopology(t *testing.T) {
	compose := readYAML(t, "compose.yml")
	services := mapping(t, compose, "services")
	wantServices := []string{"nats", "provision-identity", "semstreams", "semconnect", "canonical-smoke", "greenfield-preflight"}
	if len(services) != len(wantServices) {
		t.Fatalf("services = %v, want exactly %v", keys(services), wantServices)
	}
	for _, name := range wantServices {
		if _, ok := services[name]; !ok {
			t.Errorf("missing service %q", name)
		}
	}

	nats := services["nats"].(map[string]any)
	if got := nats["image"]; got != "nats:2.14.4-alpine@"+natsDigest {
		t.Errorf("NATS image = %v, want immutable digest", got)
	}
	if _, publishesNATS := nats["ports"]; publishesNATS {
		t.Error("NATS must not publish host ports")
	}
	if !containsString(slice(t, nats, "volumes"), "semconnect-nats-setup03a-8b99efe9c66a-data:/data") {
		t.Error("NATS does not use the explicit persistent volume")
	}

	volume := mapping(t, mapping(t, compose, "volumes"), "semconnect-nats-setup03a-8b99efe9c66a-data")
	if volume["name"] != "${SEMCONNECT_NATS_VOLUME:-semconnect-nats-setup03a-8b99efe9c66a-data}" {
		t.Errorf("target volume must default to its own fresh storage: %v", volume["name"])
	}
	semstreams := services["semstreams"].(map[string]any)
	if got := semstreams["image"]; got != "semconnect-graph-backend:"+targetImageTag {
		t.Errorf("graph backend image = %v, want frozen SETUP 03A tag", got)
	}
	if !containsString(slice(t, semstreams, "command"), "-cs-api-config") ||
		!containsString(slice(t, semstreams, "command"), "/etc/semconnect/config.json") ||
		!containsString(slice(t, semstreams, "volumes"), "./semconnect.json:/etc/semconnect/config.json:ro") {
		t.Error("graph host must share the gateway's resource-prefix and artifact configuration")
	}
	build := mapping(t, semstreams, "build")
	if build["context"] != ".." || build["dockerfile"] != "deploy/backend.Dockerfile" {
		t.Errorf("backend must build SemConnect's registered composition root: %v", build)
	}
	backendDockerfile := readFile(t, "backend.Dockerfile")
	for _, want := range []string{
		"386d475a660466863d9f8c766fec64d7fdad3edac2c6a05020c09534d71edb4b",
		"28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b",
		"ARG SEMSTREAMS_VERSION=" + targetVersion,
		"ARG SEMSTREAMS_COMMIT=" + targetCommit,
		"go list -m", "github.com/c360studio/semstreams", "./cmd/cs-graph-backend",
		"USER semstreams", "ENTRYPOINT [\"/app/cs-graph-backend\"]",
	} {
		if !strings.Contains(backendDockerfile, want) {
			t.Errorf("backend image pin/composition contract lacks %q", want)
		}
	}
	provision := mapping(t, services, "provision-identity")
	if provision["image"] != semstreams["image"] {
		t.Error("identity provisioning must use the exact graph host image")
	}
	if !containsString(slice(t, provision, "command"), "-provision-identity") ||
		!containsString(slice(t, provision, "command"), "semconnect") {
		t.Error("identity provisioning must explicitly adopt the stable semconnect platform")
	}
	dependencies := mapping(t, semstreams, "depends_on")
	if mapping(t, dependencies, "provision-identity")["condition"] != "service_completed_successfully" {
		t.Error("graph startup must wait for successful create-only identity provisioning")
	}
	for service, want := range map[string]string{
		"semconnect":           "semconnect-cs-api:" + targetImageTag,
		"canonical-smoke":      "semconnect-canonical-smoke:" + targetImageTag,
		"greenfield-preflight": "semconnect-canonical-smoke:" + targetImageTag,
	} {
		if got := services[service].(map[string]any)["image"]; got != want {
			t.Errorf("%s image = %v, want %s", service, got, want)
		}
	}
	if strings.Contains(strings.ToLower(readFile(t, "compose.yml")), "teamengine") {
		t.Error("production bundle includes TeamEngine/conformance authority")
	}
}

func TestComposeLayersCleanPreflightAndHealth(t *testing.T) {
	compose := readYAML(t, "compose.yml")
	services := mapping(t, compose, "services")
	for _, service := range []string{"nats", "semstreams", "semconnect"} {
		if _, ok := services[service].(map[string]any)["healthcheck"]; !ok {
			t.Errorf("%s lacks a healthcheck", service)
		}
	}
	if !containsDeepString(services["canonical-smoke"], "service_healthy") {
		t.Error("canonical smoke does not wait for healthy application services")
	}
	if !containsDeepString(services["greenfield-preflight"], "service_healthy") {
		t.Error("greenfield preflight does not wait for healthy NATS")
	}
}

func TestConfigsAndSmokeAreVersionedAndNonsecret(t *testing.T) {
	for _, path := range []string{
		"nats.conf", "semstreams.json", "semconnect.json", "canonical-system.v1.json",
		"canonical-datastream.v1.json", "canonical-observation.v1.json",
	} {
		content := readFile(t, path)
		for _, forbidden := range []string{"${", "TBD", "REQUIRED", "password", "token"} {
			if strings.Contains(content, forbidden) {
				t.Errorf("%s contains forbidden nonliteral/secret token %q", path, forbidden)
			}
		}
	}
	var seed map[string]any
	if err := json.Unmarshal([]byte(readFile(t, "canonical-system.v1.json")), &seed); err != nil {
		t.Fatal(err)
	}
	properties := seed["properties"].(map[string]any)
	if properties["uid"] != "urn:c360:semconnect:deployment-smoke:system:v1" {
		t.Errorf("canonical smoke uid = %v", properties["uid"])
	}
	var datastream map[string]any
	if err := json.Unmarshal([]byte(readFile(t, "canonical-datastream.v1.json")), &datastream); err != nil {
		t.Fatal(err)
	}
	if datastream["id"] != "c360.semconnect.systems.csapi.datastream.v1" ||
		datastream["system"] != "c360.semconnect.systems.csapi.system.v1" || datastream["schema"] == nil {
		t.Errorf("canonical Datastream does not bind the System and schema: %#v", datastream)
	}
	var observation map[string]any
	if err := json.Unmarshal([]byte(readFile(t, "canonical-observation.v1.json")), &observation); err != nil {
		t.Fatal(err)
	}
	if observation["id"] != "deployment-smoke-observation-v1" || observation["result"] != 21.5 {
		t.Errorf("canonical Observation identity/content = %#v", observation)
	}
	readme := readFile(t, "README.md")
	if !strings.Contains(readme, "internal-only") || !strings.Contains(readme, "no NATS credentials") {
		t.Error("README lacks the explicit internal-only NATS credential boundary")
	}
}

func TestOperationalScriptsNeverDeleteOrTranslateState(t *testing.T) {
	for _, path := range []string{"verify-persistence.sh", "probe/main.go"} {
		content := strings.ToLower(readFile(t, path))
		for _, forbidden := range []string{
			"down -v", "volume rm", "kv purge", "stream purge", "rm -rf", "compatibility", "translate",
		} {
			if strings.Contains(content, forbidden) {
				t.Errorf("%s contains forbidden destructive/legacy behavior %q", path, forbidden)
			}
		}
	}
	script := readFile(t, "verify-persistence.sh")
	for _, required := range []string{
		"docker compose", "stop", "start", "sha256", "greenfield-preflight", "verify-only", "volume-before",
		"canonical-datastream.v1.json", "canonical-observation.v1.json",
		"capture_index_readiness before-restart", "capture_index_readiness after-restart",
		`[ "$restarted_revision" -lt "$captured_revision" ]`,
		".dockerignore",
	} {
		if !strings.Contains(script, required) {
			t.Errorf("persistence verification lacks %q", required)
		}
	}
	preflight := strings.Index(script, "compose --profile smoke run --rm greenfield-preflight")
	startup := strings.Index(script, "compose up -d --build semstreams semconnect")
	if preflight < 0 || startup <= preflight {
		t.Error("empty-storage preflight must run before identity provisioning and graph startup")
	}
	probeImage := readFile(t, "probe/Dockerfile")
	for _, required := range []string{
		"conformance/cmd/index-readiness/main.go", "/usr/local/bin/index-readiness",
		"canonical-datastream.v1.json", "canonical-observation.v1.json",
	} {
		if !strings.Contains(probeImage, required) {
			t.Errorf("persistence probe image lacks %q", required)
		}
	}
	probe := readFile(t, "probe/main.go")
	for _, required := range []string{
		"/datastreams/" + "\"+expectedDatastreamID+\"" + "/schema",
		"/observations?limit=10",
		"GlobalObservationSHA256",
		"ScopedObservationSHA256",
	} {
		if !strings.Contains(probe, required) {
			t.Errorf("persistence probe lacks %q", required)
		}
	}
}

func TestPersistenceVerifierPinsNATS214CleanStopContract(t *testing.T) {
	script := readFile(t, "verify-persistence.sh")
	for _, required := range []string{
		"NATS_214_CLEAN_STOP_STATE='0 false'",
		`[ "$stop_state" != "$NATS_214_CLEAN_STOP_STATE" ]`,
		"JetStream Shutdown",
		"Server Exiting",
		"oom_killed=true",
	} {
		if !strings.Contains(script, required) {
			t.Errorf("persistence verifier lacks pinned NATS 2.14.4 stop guard %q", required)
		}
	}
	if strings.Contains(script, `"1 false"`) || strings.Contains(script, "'1 false'") {
		t.Error("persistence verifier retains the historical NATS exit-1 expectation")
	}
	readme := readFile(t, "README.md")
	if !strings.Contains(readme, "NATS 2.14.4 stop") || !strings.Contains(readme, "exit code 0") {
		t.Error("deployment README does not document the pinned NATS 2.14.4 clean exit contract")
	}
}

func TestPersistenceReadinessUsesHostOwnershipForEvidenceBindMount(t *testing.T) {
	script := readFile(t, "verify-persistence.sh")
	for _, required := range []string{
		`--user "$(id -u):$(id -g)"`,
		`-v "$evidence_dir:/evidence"`,
	} {
		if !strings.Contains(script, required) {
			t.Errorf("persistence readiness mount lacks host ownership contract %q", required)
		}
	}
	for _, forbidden := range []string{"chmod 777", "chmod -r", "chown -r"} {
		if strings.Contains(strings.ToLower(script), forbidden) {
			t.Errorf("persistence verifier broadens evidence permissions with %q", forbidden)
		}
	}
}

func TestDockerBuildContextIncludesOnlyReadinessHelperAndExcludesEvidence(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test path")
	}
	file, err := os.Open(filepath.Join(filepath.Dir(source), "..", ".dockerignore"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	patterns, err := ignorefile.ReadAll(file)
	if err != nil {
		t.Fatal(err)
	}
	matcher, err := patternmatcher.New(patterns)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		path        string
		wantIgnored bool
	}{
		{"conformance/cmd/index-readiness/main.go", false},
		{"conformance/cmd/index-readiness/main_test.go", true},
		{"conformance/run.sh", true},
		{"conformance/fixtures/system-hosted.sml.json", true},
		{"openspec/changes/migrate-semstreams-beta160/evidence/conformance-beta160/testng-report-2026-08-12T12-39-21Z.xml", true},
		{"openspec/changes/migrate-semstreams-beta160/evidence/conformance-beta160/cs-api-server-container-2026-08-12T12-39-21Z.log", true},
		{"openspec/changes/migrate-semstreams-beta160/evidence/persistence-r2/first-start-jsz.json", true},
		{"openspec/changes/migrate-semstreams-beta160/evidence/persistence-r3/before-restart.json", true},
	} {
		ignored, err := matcher.MatchesOrParentMatches(tc.path)
		if err != nil {
			t.Fatalf("match %s: %v", tc.path, err)
		}
		if ignored != tc.wantIgnored {
			t.Errorf("Docker context ignored(%q) = %v, want %v", tc.path, ignored, tc.wantIgnored)
		}
	}
}

func readYAML(t *testing.T, name string) map[string]any {
	t.Helper()
	var result map[string]any
	if err := yaml.Unmarshal([]byte(readFile(t, name)), &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func readFile(t *testing.T, name string) string {
	t.Helper()
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test path")
	}
	content, err := os.ReadFile(filepath.Join(filepath.Dir(source), name))
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func mapping(t *testing.T, value map[string]any, key string) map[string]any {
	t.Helper()
	result, ok := value[key].(map[string]any)
	if !ok {
		t.Fatalf("%q is not a mapping", key)
	}
	return result
}

func slice(t *testing.T, value map[string]any, key string) []any {
	t.Helper()
	result, ok := value[key].([]any)
	if !ok {
		t.Fatalf("%q is not a sequence", key)
	}
	return result
}

func keys(value map[string]any) []string {
	result := make([]string, 0, len(value))
	for key := range value {
		result = append(result, key)
	}
	return result
}

func containsString(values []any, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func containsDeepString(value any, want string) bool {
	switch typed := value.(type) {
	case string:
		return strings.Contains(typed, want)
	case []any:
		for _, item := range typed {
			if containsDeepString(item, want) {
				return true
			}
		}
	case map[string]any:
		for _, item := range typed {
			if containsDeepString(item, want) {
				return true
			}
		}
	}
	return false
}
