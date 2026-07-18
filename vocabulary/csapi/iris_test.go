package csapi

import (
	"strings"
	"testing"

	"github.com/c360studio/semstreams/vocabulary"
)

func TestIRIsCoverConstants(t *testing.T) {
	wantPairs := map[string]string{
		Prefix + ":Datastream":        Datastream,
		Prefix + ":ControlStream":     ControlStream,
		Prefix + ":Command":           Command,
		Prefix + ":SystemEvent":       SystemEvent,
		Prefix + ":Feasibility":       Feasibility,
		Prefix + ":SensorMLDocument":  SensorMLDocument,
		Prefix + ":SWESchemaDocument": SWESchemaDocument,

		Prefix + ":producedBy":           ProducedByIRI,
		Prefix + ":resultTimeRange":      ResultTimeRangeIRI,
		Prefix + ":phenomenonTimeRange":  PhenomenonTimeRangeIRI,
		Prefix + ":resultType":           ResultTypeIRI,
		Prefix + ":controlsSystem":       ControlsSystemIRI,
		Prefix + ":partOfControlStream":  PartOfControlStreamIRI,
		Prefix + ":eventForSystem":       EventForSystemIRI,
		Prefix + ":hasSource":            HasSourceIRI,
		Prefix + ":hasResultSchema":      HasResultSchemaIRI,
		Prefix + ":hasCommandSchema":     HasCommandSchemaIRI,
		Prefix + ":observedProperty":     ObservedPropertyIRI,
		Prefix + ":deployedSystems":      DeploymentDeployedSystemsIRI,
		Prefix + ":parent":               DeploymentParentIRI,
		Prefix + ":hostedProcedure":      SamplingFeatureHostedProcedureIRI,
		Prefix + ":inputName":            ControlStreamInputNameIRI,
		Prefix + ":async":                ControlStreamAsyncIRI,
		Prefix + ":commandFormat":        ControlStreamCommandFormatIRI,
		Prefix + ":controlledProperties": ControlStreamControlledPropertiesIRI,
		Prefix + ":issueTime":            ControlStreamIssueTimeIRI,
		Prefix + ":executionTime":        ControlStreamExecutionTimeIRI,
		Prefix + ":status":               CommandStatusIRI,
		Prefix + ":sender":               CommandSenderIRI,
		Prefix + ":params":               CommandParamsIRI,
		Prefix + ":phenomenonTime":       DatastreamPhenomenonTimeIRI,
		Prefix + ":resultTime":           DatastreamResultTimeIRI,
		Prefix + ":definition":           PropertyDefinitionIRI,
		Prefix + ":baseProperty":         PropertyBasePropertyIRI,
		Prefix + ":time":                 SystemEventTimeIRI,
		Prefix + ":eventType":            SystemEventTypeIRI,
		Prefix + ":message":              SystemEventMessageIRI,
		Prefix + ":severity":             SystemEventSeverityIRI,
		Prefix + ":source":               SystemEventSourceIRI,
		Prefix + ":payload":              SystemEventPayloadIRI,
		Prefix + ":keywords":             SystemEventKeywordsIRI,
		Prefix + ":controlstream":        FeasibilityControlStreamIRI,
		Prefix + ":result":               FeasibilityResultIRI,
	}
	got := IRIs()
	if len(got) != len(wantPairs) {
		t.Fatalf("IRIs(): want %d entries, got %d", len(wantPairs), len(got))
	}
	for compact, iri := range wantPairs {
		if gotIRI, ok := got[compact]; !ok {
			t.Errorf("IRIs() missing %q", compact)
		} else if gotIRI != iri {
			t.Errorf("IRIs()[%q] = %q, want %q", compact, gotIRI, iri)
		}
	}
}

// TestIRIConstantsLiveInDeclaredNamespace ensures every `*IRI`
// constant lands under the spec-rooted Namespace. The dotted
// predicate constants (ProducedBy, HasSource, etc.) DO NOT live in
// the IRI namespace — they're internal dotted-notation strings —
// so they're excluded from this check (gh#182).
func TestIRIConstantsLiveInDeclaredNamespace(t *testing.T) {
	all := []string{
		Datastream, ControlStream, Command, SystemEvent,
		SensorMLDocument, SWESchemaDocument,
		ProducedByIRI, ResultTimeRangeIRI, PhenomenonTimeRangeIRI, ResultTypeIRI,
		ControlsSystemIRI, PartOfControlStreamIRI, EventForSystemIRI,
		HasSourceIRI, HasResultSchemaIRI, HasCommandSchemaIRI,
	}
	for _, c := range all {
		if !strings.HasPrefix(c, Namespace) {
			t.Errorf("%q does not start with CS API namespace %q", c, Namespace)
		}
	}
}

// TestPredicateConstantsUseDottedNotation ensures the dotted predicate
// constants follow the framework convention `domain.category.property`
// (vocabulary/predicates.go) — i.e. NOT IRI-shaped (gh#182). Three
// dotted segments, lowercase domain, no IRI-namespace prefix.
func TestPredicateConstantsUseDottedNotation(t *testing.T) {
	predicates := []string{
		ProducedBy, ResultTimeRange, PhenomenonTimeRange, ResultType,
		ControlsSystem, PartOfControlStream, EventForSystem,
		HasSource, HasResultSchema, HasCommandSchema,
		ObservedProperty,
		DeploymentDeployedSystems, DeploymentParent, SamplingFeatureHostedProcedure,
		ControlStreamInputName, ControlStreamAsync, ControlStreamCommandFormat,
		ControlStreamControlledProperties, ControlStreamIssueTime, ControlStreamExecutionTime,
		CommandIssueTime, CommandExecutionTime, CommandStatus, CommandSender, CommandParams,
		DatastreamPhenomenonTime, DatastreamResultTime,
		PropertyDefinition, PropertyBaseProperty,
		SystemEventTime, SystemEventType, SystemEventMessage, SystemEventSeverity,
		SystemEventSource, SystemEventPayload, SystemEventKeywords,
		FeasibilityControlStream, FeasibilityStatus, FeasibilityParams, FeasibilityResult,
	}
	for _, p := range predicates {
		if strings.HasPrefix(p, Namespace) {
			t.Errorf("%q must be dotted-notation, not IRI-shaped", p)
		}
		if _, err := vocabulary.ParsePredicate(p); err != nil {
			t.Errorf("ParsePredicate(%q): %v", p, err)
		}
	}
}

func TestIsKnown(t *testing.T) {
	cases := []struct {
		iri  string
		want bool
	}{
		{Datastream, true},
		{ProducedByIRI, true},
		{ResultTimeRangeIRI, true},
		{Namespace + "unmappedButValidCSAPI", false},
		{"http://example.org/not-csapi", false},
		{"", false},
	}
	for _, c := range cases {
		if got := IsKnown(c.iri); got != c.want {
			t.Errorf("IsKnown(%q) = %v, want %v", c.iri, got, c.want)
		}
	}
}

func TestLocalName(t *testing.T) {
	cases := []struct {
		iri  string
		want string
	}{
		{Datastream, "Datastream"},
		{ProducedByIRI, "producedBy"},
		{ResultTimeRangeIRI, "resultTimeRange"},
		{Namespace + "unmappedButValidCSAPI", "unmappedButValidCSAPI"},
		{"http://example.org/not-csapi", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := LocalName(c.iri); got != c.want {
			t.Errorf("LocalName(%q) = %q, want %q", c.iri, got, c.want)
		}
	}
}
