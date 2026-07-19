package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	semtypes "github.com/c360studio/semstreams/pkg/types"
)

const (
	wantSeedSystemID         = "c360.semconnect.systems.csapi.system.weather-station-01"
	wantSeedDatastreamID     = "c360.semconnect.systems.csapi.datastream.weather-temperature-01"
	wantSeedSchemaArtifactID = "c360.semconnect.systems.csapi.schema." +
		"c360_semconnect_systems_csapi_datastream_weather-temperature-01-resultSchema"
	wantSeedObservationSubject      = "cs-api.observations." + wantSeedDatastreamID
	wantSeedObservationID           = "ets-observation-001"
	wantSeedSystemEventID           = "c360.semconnect.systems.csapi.systemevent.00Z"
	wantSeedControlStreamID         = "c360.semconnect.systems.csapi.controlstream.ptz-01"
	wantSeedCommandSchemaArtifactID = "c360.semconnect.systems.csapi.schema." +
		"c360_semconnect_systems_csapi_controlstream_ptz-01-commandSchema"
)

var nonSeedIDTokenChar = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

func TestRetainedStateSeedIdentitiesAreStableAndCanonical(t *testing.T) {
	t.Parallel()

	fixture := mustReadTestFile(t, "../../fixtures/system.sml.json")
	var systemFixture struct {
		ID       string `json:"id"`
		UniqueID string `json:"uniqueId"`
	}
	if err := json.Unmarshal(fixture, &systemFixture); err != nil {
		t.Fatalf("decode system fixture: %v", err)
	}
	if systemFixture.ID != "urn:semconnect:conformance:system:weather-station-01" {
		t.Fatalf("document id changed: %q", systemFixture.ID)
	}
	if systemFixture.UniqueID != "urn:ets:system:weather-station-01" {
		t.Fatalf("uniqueId = %q, want stable seed UID", systemFixture.UniqueID)
	}

	runScript := string(mustReadTestFile(t, "../../run.sh"))
	constants := parseSeedConstants(t, runScript)
	assertSeedConstant(t, constants, "SYSTEM_ID", wantSeedSystemID)
	assertSeedConstant(t, constants, "DATASTREAM_ID", wantSeedDatastreamID)
	assertSeedConstant(t, constants, "SCHEMA_ARTIFACT_ID", wantSeedSchemaArtifactID)
	assertSeedConstant(t, constants, "SCHEMA_ARTIFACT_OBJECT_KEY", wantSeedSchemaArtifactID+".json")
	assertSeedConstant(t, constants, "OBSERVATION_SUBJECT", wantSeedObservationSubject)
	assertSeedConstant(t, constants, "OBSERVATION_ID", wantSeedObservationID)
	assertSeedConstant(t, constants, "SYSTEM_EVENT_ID", wantSeedSystemEventID)
	assertSeedConstant(t, constants, "CONTROLSTREAM_ID", wantSeedControlStreamID)
	assertSeedConstant(t, constants, "COMMAND_SCHEMA_ARTIFACT_ID", wantSeedCommandSchemaArtifactID)
	assertSeedConstant(t, constants, "COMMAND_SCHEMA_ARTIFACT_OBJECT_KEY", wantSeedCommandSchemaArtifactID+".json")

	datastreamJSON := extractHeredoc(t, runScript, "ds_body")
	datastreamJSON = strings.ReplaceAll(datastreamJSON, "${SEED_DATASTREAM_ID}", wantSeedDatastreamID)
	datastreamJSON = strings.ReplaceAll(datastreamJSON, "${sys_id}", wantSeedSystemID)
	var datastream struct {
		ID     string          `json:"id"`
		System string          `json:"system"`
		Schema json.RawMessage `json:"schema"`
	}
	if err := json.Unmarshal([]byte(datastreamJSON), &datastream); err != nil {
		t.Fatalf("decode Datastream seed: %v", err)
	}
	if datastream.ID != wantSeedDatastreamID || datastream.System != wantSeedSystemID || len(datastream.Schema) == 0 {
		t.Fatalf("Datastream retained-state inputs are not explicit: %+v", datastream)
	}

	observationJSON := extractHeredoc(t, runScript, "obs_body")
	observationJSON = strings.ReplaceAll(observationJSON, "${SEED_OBSERVATION_ID}", wantSeedObservationID)
	var observation struct {
		ID         string `json:"id"`
		ResultTime string `json:"resultTime"`
	}
	if err := json.Unmarshal([]byte(observationJSON), &observation); err != nil {
		t.Fatalf("decode Observation seed: %v", err)
	}
	if observation.ID != wantSeedObservationID || observation.ResultTime == "" {
		t.Fatalf("Observation seed lacks stable identity/time inputs: %+v", observation)
	}

	controlStreamJSON := extractHeredoc(t, runScript, "ctrl_body")
	controlStreamJSON = strings.ReplaceAll(controlStreamJSON, "${SEED_CONTROLSTREAM_ID}", wantSeedControlStreamID)
	controlStreamJSON = strings.ReplaceAll(controlStreamJSON, "${sys_id}", wantSeedSystemID)
	var controlStream struct {
		ID       string          `json:"id"`
		SystemID string          `json:"system@id"`
		Schema   json.RawMessage `json:"schema"`
	}
	if err := json.Unmarshal([]byte(controlStreamJSON), &controlStream); err != nil {
		t.Fatalf("decode ControlStream seed: %v", err)
	}
	if controlStream.ID != wantSeedControlStreamID ||
		controlStream.SystemID != wantSeedSystemID || len(controlStream.Schema) == 0 {
		t.Fatalf("ControlStream retained-state inputs are not explicit: %+v", controlStream)
	}

	eventJSON := extractSingleQuotedJSON(t, runScript, "event_body")
	var event struct {
		EventTime string `json:"eventTime"`
		EventType string `json:"eventType"`
	}
	if err := json.Unmarshal([]byte(eventJSON), &event); err != nil {
		t.Fatalf("decode SystemEvent seed: %v", err)
	}
	if event.EventTime == "" || event.EventType == "" {
		t.Fatalf("SystemEvent seed lacks deterministic mint inputs: %+v", event)
	}

	derive := func() (map[string]string, bool) {
		systemID, systemFallback := mintSeedEntityID(
			"c360.semconnect.systems.csapi.system",
			systemFixture.UniqueID,
		)
		schemaID, schemaFallback := mintSeedEntityID(
			"c360.semconnect.systems.csapi.schema",
			datastream.ID+"-resultSchema",
		)
		eventID, eventFallback := mintSeedEntityID(
			"c360.semconnect.systems.csapi.systemevent",
			systemID+"-"+event.EventType+"-"+event.EventTime,
		)
		commandSchemaID, commandSchemaFallback := mintSeedEntityID(
			"c360.semconnect.systems.csapi.schema",
			controlStream.ID+"-commandSchema",
		)
		return map[string]string{
			"system":                  systemID,
			"datastream":              datastream.ID,
			"schema_artifact":         schemaID,
			"schema_artifact_key":     schemaID + ".json",
			"observation_subject":     "cs-api.observations." + datastream.ID,
			"observation_id":          observation.ID,
			"system_event":            eventID,
			"controlstream":           controlStream.ID,
			"command_schema_artifact": commandSchemaID,
			"command_schema_key":      commandSchemaID + ".json",
		}, systemFallback || schemaFallback || eventFallback || commandSchemaFallback
	}

	first, usedUUIDFallback := derive()
	second, usedUUIDFallbackAgain := derive()
	if usedUUIDFallback || usedUUIDFallbackAgain {
		t.Fatal("retained-state seed derivation entered UUID fallback")
	}
	if fmt.Sprint(first) != fmt.Sprint(second) {
		t.Fatalf("repeated derivation changed: first=%v second=%v", first, second)
	}
	want := map[string]string{
		"system":                  wantSeedSystemID,
		"datastream":              wantSeedDatastreamID,
		"schema_artifact":         wantSeedSchemaArtifactID,
		"schema_artifact_key":     wantSeedSchemaArtifactID + ".json",
		"observation_subject":     wantSeedObservationSubject,
		"observation_id":          wantSeedObservationID,
		"system_event":            wantSeedSystemEventID,
		"controlstream":           wantSeedControlStreamID,
		"command_schema_artifact": wantSeedCommandSchemaArtifactID,
		"command_schema_key":      wantSeedCommandSchemaArtifactID + ".json",
	}
	for name, wantID := range want {
		if first[name] != wantID {
			t.Errorf("%s identity = %q, want %q", name, first[name], wantID)
		}
	}
	for _, name := range []string{
		"system", "datastream", "schema_artifact", "system_event", "controlstream", "command_schema_artifact",
	} {
		if err := semtypes.ValidateEntityID(first[name]); err != nil {
			t.Errorf("%s identity is not canonical: %v", name, err)
		}
	}
	if strings.ContainsAny(first["observation_subject"], "*>") {
		t.Fatalf("observation subject is not literal: %q", first["observation_subject"])
	}
}

func mintSeedEntityID(prefix, exactSource string) (string, bool) {
	token := exactSource
	for {
		index := strings.IndexByte(token, ':')
		if index < 0 {
			break
		}
		token = token[index+1:]
	}
	token = strings.Trim(nonSeedIDTokenChar.ReplaceAllString(token, "_"), "_-")
	if token == "" {
		return "", true
	}
	candidate := prefix + "." + token
	if semtypes.ValidateEntityID(candidate) == nil {
		return candidate, false
	}
	digest := sha256.Sum256([]byte(exactSource))
	return fmt.Sprintf("%s.h-%x", prefix, digest), false
}

func parseSeedConstants(t *testing.T, script string) map[string]string {
	t.Helper()
	pattern := regexp.MustCompile(`(?m)^SEED_([A-Z_]+)="([^"]+)"$`)
	constants := make(map[string]string)
	for _, match := range pattern.FindAllStringSubmatch(script, -1) {
		constants[match[1]] = match[2]
	}
	for range constants {
		for name, value := range constants {
			for reference, replacement := range constants {
				value = strings.ReplaceAll(value, "${SEED_"+reference+"}", replacement)
			}
			constants[name] = value
		}
	}
	return constants
}

func assertSeedConstant(t *testing.T, constants map[string]string, name, want string) {
	t.Helper()
	if got := constants[name]; got != want {
		t.Fatalf("SEED_%s = %q, want %q", name, got, want)
	}
}

func extractHeredoc(t *testing.T, script, variable string) string {
	t.Helper()
	pattern := regexp.MustCompile(`(?s)local ` + regexp.QuoteMeta(variable) + `\n\s*` +
		regexp.QuoteMeta(variable) + `=\$\(cat <<EOF\n(.*?)\nEOF\n\)`)
	match := pattern.FindStringSubmatch(script)
	if len(match) != 2 {
		t.Fatalf("could not parse %s heredoc from run.sh", variable)
	}
	return match[1]
}

func extractSingleQuotedJSON(t *testing.T, script, variable string) string {
	t.Helper()
	pattern := regexp.MustCompile(`(?m)local ` + regexp.QuoteMeta(variable) + `='([^']+)'`)
	match := pattern.FindStringSubmatch(script)
	if len(match) != 2 {
		t.Fatalf("could not parse %s JSON from run.sh", variable)
	}
	return match[1]
}

func mustReadTestFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}
