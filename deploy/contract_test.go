package deploy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const (
	beta153Version = "v1.0.0-beta.153"
	beta153Commit  = "d2654e5a027138b8a9056863da5ed463ef767f37"
	natsDigest     = "sha256:b83efabe3e7def1e0a4a31ec6e078999bb17c80363f881df35edc70fcb6bb927"
)

func TestComposeIsGreenfieldProductionTopology(t *testing.T) {
	compose := readYAML(t, "compose.yml")
	services := mapping(t, compose, "services")
	wantServices := []string{"nats", "semstreams", "semconnect", "canonical-smoke", "greenfield-preflight"}
	if len(services) != len(wantServices) {
		t.Fatalf("services = %v, want exactly %v", keys(services), wantServices)
	}
	for _, name := range wantServices {
		if _, ok := services[name]; !ok {
			t.Errorf("missing service %q", name)
		}
	}

	nats := services["nats"].(map[string]any)
	if got := nats["image"]; got != "nats:2.10-alpine@"+natsDigest {
		t.Errorf("NATS image = %v, want immutable digest", got)
	}
	if _, publishesNATS := nats["ports"]; publishesNATS {
		t.Error("NATS must not publish host ports")
	}
	if !containsString(slice(t, nats, "volumes"), "semconnect-nats-data:/data") {
		t.Error("NATS does not use the explicit persistent volume")
	}

	semstreams := services["semstreams"].(map[string]any)
	if got := semstreams["image"]; got != "semconnect-semstreams:"+beta153Version {
		t.Errorf("SemStreams image = %v, want beta.153 release tag", got)
	}
	build := mapping(t, semstreams, "build")
	wantContext := "https://github.com/C360Studio/semstreams.git#" + beta153Commit
	if build["context"] != wantContext {
		t.Errorf("SemStreams build context = %v, want %s", build["context"], wantContext)
	}
	inline, ok := build["dockerfile_inline"].(string)
	if !ok || !strings.Contains(inline, "0178a641fbb4858c5f1b48e34bdaabe0350a330a1b1149aabd498d0699ff5fb2") ||
		!strings.Contains(inline, "28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b") {
		t.Error("SemStreams build does not pin both base images by digest")
	}
	for _, want := range []string{"-X main.Version=" + beta153Version, "-X main.GitCommit=" + beta153Commit} {
		if !strings.Contains(inline, want) {
			t.Errorf("SemStreams build metadata lacks %q", want)
		}
	}
	for service, want := range map[string]string{
		"semconnect":           "semconnect-cs-api:beta.153",
		"canonical-smoke":      "semconnect-canonical-smoke:beta.153",
		"greenfield-preflight": "semconnect-canonical-smoke:beta.153",
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
	for _, path := range []string{"nats.conf", "semstreams.json", "semconnect.json", "canonical-system.v1.json"} {
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
	} {
		if !strings.Contains(script, required) {
			t.Errorf("persistence verification lacks %q", required)
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
