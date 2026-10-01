package csapi

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/vocabulary"
)

// These fixtures are consumer-owned transfer cases, independent of framework
// implementation details. Revisions are relative: a global KV sequence is not
// a portable golden value.
type setup03AExpectations struct {
	RootID         string           `json:"root_id"`
	AbsentTargetID string           `json:"absent_target_id"`
	Triples        []message.Triple `json:"triples"`
	Vocabulary     []struct {
		Predicate string `json:"predicate"`
		IRI       string `json:"iri"`
		Datatype  string `json:"datatype"`
		Inverse   string `json:"inverse"`
	} `json:"vocabulary"`
	Failures []struct {
		Code       string `json:"code"`
		Injection  string `json:"injection"`
		HTTPStatus int    `json:"http_status"`
		Uncertain  bool   `json:"uncertain"`
		Attempts   int32  `json:"attempts"`
	} `json:"failures"`
	StorageInstance string `json:"storage_instance"`
	ContentType     string `json:"content_type"`
}

func setup03AFixture(t *testing.T, name string) []byte {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", "setup03a", name))
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func setup03AExpected(t *testing.T) setup03AExpectations {
	t.Helper()
	var expected setup03AExpectations
	if err := json.Unmarshal(setup03AFixture(t, "expectations.json"), &expected); err != nil {
		t.Fatal(err)
	}
	return expected
}

func TestSetup03AReferenceSensorML(t *testing.T) {
	expected := setup03AExpected(t)
	c := newTestComponent(t, nil)
	id, triples, err := c.buildSystemTriplesFromSensorML(setup03AFixture(t, "system.json"))
	if err != nil {
		t.Fatal(err)
	}
	if id != expected.RootID || len(triples) != len(expected.Triples) {
		t.Fatalf("root/projection mismatch: id=%s triples=%+v", id, triples)
	}
	for _, want := range expected.Triples {
		found := false
		for _, got := range triples {
			if got.Subject != expected.RootID {
				t.Fatalf("foreign subject escaped root projection: %+v", got)
			}
			found = found || (got.Predicate == want.Predicate && reflect.DeepEqual(got.Object, want.Object) && got.Datatype == want.Datatype)
		}
		if !found {
			t.Errorf("missing expected triple: %+v", want)
		}
	}
	for _, want := range expected.Vocabulary {
		metadata := vocabulary.GetPredicateMetadata(want.Predicate)
		if metadata == nil || metadata.StandardIRI != want.IRI || (want.Datatype != "" && metadata.DataType != want.Datatype) {
			t.Errorf("vocabulary mapping %s: got %+v want %+v", want.Predicate, metadata, want)
		}
		if want.Inverse != "" && vocabulary.GetInversePredicate(want.Predicate) != want.Inverse {
			t.Errorf("inverse mapping %s: got %s want %s", want.Predicate, vocabulary.GetInversePredicate(want.Predicate), want.Inverse)
		}
	}
}
