package csapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"slices"
	"strings"

	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/parser/sensorml"
	"github.com/c360studio/semstreams/pkg/errs"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
)

// SubjectTripleAddBatch is the NATS request/reply subject the framework's
// graph-ingest component exposes for batched triple writes (verified at
// processor/graph-ingest/mutations.go in pinned v1.0.0-beta.73). The same
// subject is the constant `graphingest.SubjectTripleAddBatch` upstream;
// duplicated locally because the upstream package is not importable
// (lowercased name).
const SubjectTripleAddBatch = "graph.mutation.triple.add_batch"

// PredSystemPosition is the predicate name cs-api uses to store the
// SensorML `position` field as a triple — a sister-side workaround for
// the framework's missing position-preservation. Stage 14. Three-part
// dotted form matches framework convention. The Object value is the
// raw GeoJSON-shaped JSON bytes (as a string) from the SensorML input.
//
// Retires when the upstream ask
// (docs/upstream-asks/semstreams-sensorml-position-preservation.md)
// lands and Asset.Triples() emits a `sensorml.process.position` triple
// (or similar) natively. The two predicate names can coexist during
// migration; readers should look for both until cutover.
const PredSystemPosition = "cs-api.system.position"

// jsonNull is the byte-equality target for detecting a literal JSON
// null in extractPositionTriple. Declared above the function so a
// top-down reader sees it before its first reference.
var jsonNull = []byte("null")

// extractPositionTriple peeks the raw POST body for a top-level
// `position` field and returns it as a sister-side workaround triple.
// We unmarshal into a struct with `Position json.RawMessage` (not
// full JSON-to-map) so the bytes preserve client-side number precision
// and field ordering — critical for GeoJSON consumers that compare
// coordinates strictly. Returns ok=false when the field is absent,
// empty, or the literal JSON null.
func extractPositionTriple(entityID string, body []byte) (message.Triple, bool) {
	var probe struct {
		Position json.RawMessage `json:"position,omitempty"`
	}
	if err := json.Unmarshal(body, &probe); err != nil || len(probe.Position) == 0 {
		return message.Triple{}, false
	}
	// JSON literal `null` decodes to a 4-byte RawMessage `[]byte("null")`.
	// Skip it — there's no geometry to store and downstream consumers
	// shouldn't have to special-case the string "null".
	if bytes.Equal(probe.Position, jsonNull) {
		return message.Triple{}, false
	}
	return message.Triple{
		Subject:   entityID,
		Predicate: PredSystemPosition,
		Object:    string(probe.Position),
	}, true
}

// handleSystemPost serves POST /systems — CS API §7.6.
//
// Why request-reply on `graph.mutation.triple.add_batch` and not the
// JetStream fire-and-forget path on `entity.>`: the framework defines
// graph.CreateEntityRequest / CreateEntityWithTriplesRequest in
// graph/mutation_requests.go but no NATS handler is wired for them in
// v1.0.0-beta.73. The JetStream path would force 202 Accepted (no
// synchronous storage confirmation); add_batch's per-Subject CAS upsert
// gives us a real Success/Error reply to return 201 Created honestly.
// See docs/upstream-asks/semstreams-entity-create-handlers-unwired.md.
func (c *Component) handleSystemPost(w http.ResponseWriter, r *http.Request) {
	// Stage 14: accept both spec form (sml+json) and legacy long form
	// (sensorml+json). Symmetric with FamilySystemItem's read-side set.
	// Accept-Post advertises both so clients sending the wrong one know
	// what's on offer.
	if err := requireMediaTypeAny(r.Header.Get("Content-Type"),
		string(MediaSensorML), string(MediaSensorMLLegacy)); err != nil {
		w.Header().Set("Accept-Post", string(MediaSensorML)+", "+string(MediaSensorMLLegacy))
		writeJSONError(w, http.StatusUnsupportedMediaType, err.Error())
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeJSONError(w, http.StatusRequestEntityTooLarge,
				fmt.Sprintf("request body exceeds %d bytes", maxErr.Limit))
			return
		}
		writeJSONError(w, http.StatusBadRequest, "could not read request body")
		return
	}

	process, err := sensorml.UnmarshalProcess(body)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid SensorML JSON: "+err.Error())
		return
	}
	if process == nil || process.Base() == nil {
		writeJSONError(w, http.StatusBadRequest, "empty SensorML process")
		return
	}

	entityID := c.mintSystemEntityID(process.Base().UniqueID)
	asset := sensorml.NewAsset(entityID, process)
	triples := asset.Triples()
	if len(triples) == 0 {
		// Asset.Triples() returns nil for malformed processes (missing
		// Base, etc.). The Base check above catches most; this is the
		// belt to that suspenders so we never publish a 0-triple batch.
		writeJSONError(w, http.StatusBadRequest, "SensorML process produced no representable triples")
		return
	}

	// Stage 14 sister-side workaround for the framework's missing
	// SensorML position preservation. `parser/sensorml`'s type model
	// has no Position field on AbstractProcess (verified at framework
	// v1.0.0-beta.75 types_process.go:40-55) so `position` in the input
	// JSON is silently dropped at unmarshal. We peek the raw body for
	// a top-level `position` here and append a triple under our own
	// predicate name (`cs-api.system.position`) so /systems/{id} can
	// surface geometry — without that, the Botts ETS
	// `systemItemHasGeometryOrValidTime` test SkipExceptions and
	// cascade-gates the entire sensorml + geojson groups.
	//
	// Retire this block when the upstream ask
	// (docs/upstream-asks/semstreams-sensorml-position-preservation.md)
	// lands a Position field on AbstractProcess + emits the triple
	// natively. Migration: triple-rewrite existing entities from
	// `cs-api.system.position` to the new framework predicate.
	if posTriple, ok := extractPositionTriple(entityID, body); ok {
		triples = append(triples, posTriple)
	}

	id := IdentityFrom(r.Context())
	if err := c.ingestTriples(r.Context(), triples, id); err != nil {
		// Echo the minted ID into the error path so a 400/503 body
		// names the resource the client tried to create. Otherwise the
		// client has no correlation back to their POST.
		w.Header().Set("X-CS-Attempted-ID", entityID)
		c.writeBackendError(w, err)
		return
	}

	w.Header().Set("Content-Type", string(MediaJSON))
	w.Header().Set("Location", "/systems/"+entityID)
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(struct {
		Status string `json:"status"`
		ID     string `json:"id"`
		Type   string `json:"type"`
	}{Status: "created", ID: entityID, Type: "System"})
}

// mintSystemEntityID builds a 6-part SemStreams entity ID by appending a
// sanitized form of uniqueID to the operator-configured 5-part prefix.
// The prefix is validated at config time (Validate() in config.go), so
// this function trusts it.
func (c *Component) mintSystemEntityID(uniqueID string) string {
	return c.cfg.SystemIDPrefix + "." + uniqueIDToToken(uniqueID)
}

// nonIDTokenChar matches anything outside the SemStreams entity ID
// per-token character set ([a-zA-Z0-9_-]).
var nonIDTokenChar = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

// uniqueIDToToken collapses a SensorML uniqueId (or any client-supplied
// identifier string) into a single entity-ID token. Strips URI schemes
// (urn:uuid:, http:, https:), replaces disallowed characters with `_`,
// trims leading/trailing separators, and falls back to a fresh UUID if
// the result is empty.
//
// UUIDs survive the pipeline intact (their hyphens are in the allowed
// set). The Trim step guarantees the result either starts with
// alphanumeric or is empty — so the regex-match invariant required by
// entityIDTokenRegex is upheld without further substitution.
func uniqueIDToToken(uniqueID string) string {
	s := uniqueID
	for {
		i := strings.IndexByte(s, ':')
		if i < 0 {
			break
		}
		s = s[i+1:]
	}
	s = nonIDTokenChar.ReplaceAllString(s, "_")
	s = strings.Trim(s, "_-")
	if s == "" {
		// uuid.NewString() produces "8-4-4-4-12" with hyphens, which the
		// token regex permits, so no further substitution is needed.
		return uuid.NewString()
	}
	return s
}

// ingestTriples publishes a batch of triples to
// graph.mutation.triple.add_batch via NATS request-reply, attaching
// audit headers from the request identity. Returns a classified error
// so writeBackendError maps cleanly to HTTP status.
//
// Timeout: QueryTimeout, NOT PublishTimeout. This is a request/reply
// (we wait for a reply), so it lives on the same budget as /systems and
// /systems/{id} reads. PublishTimeout is for fire-and-forget JetStream
// publishes (observations.go); using it here would couple two
// independently-tuned latency budgets.
func (c *Component) ingestTriples(ctx context.Context, triples []message.Triple, id Identity) error {
	req := graph.AddTriplesBatchRequest{Triples: triples}
	reqBody, err := json.Marshal(req)
	if err != nil {
		return errs.Wrap(err, "cs-api", "ingestTriples", "marshal batch request")
	}

	// Audit headers + trace context are attached on the request envelope.
	// graph-ingest doesn't capture these in the stored EntityState today,
	// but a NATS-level audit subscriber (or trace-context propagation)
	// needs them — and the symmetry with observations.go's audited
	// publish path keeps the operator runbook uniform.
	hdrs := id.AuditHeaders()

	// RequestWithHeaders applies its own context.WithTimeout from the
	// timeout argument; we pass ctx through unwrapped so cancellation
	// from the HTTP request still propagates without double-budgeting.
	reply, err := c.nats.RequestWithHeaders(ctx, SubjectTripleAddBatch, reqBody, hdrs, c.cfg.QueryTimeout)
	if err != nil {
		switch {
		case errors.Is(err, nats.ErrNoResponders),
			errors.Is(err, nats.ErrTimeout),
			errors.Is(err, context.DeadlineExceeded),
			errors.Is(err, nats.ErrConnectionClosed):
			return errs.WrapTransient(err, "cs-api", "ingestTriples", "graph backend unavailable")
		default:
			return errs.Wrap(err, "cs-api", "ingestTriples", "add_batch request")
		}
	}

	var resp graph.AddTriplesBatchResponse
	if err := json.Unmarshal(reply.Data, &resp); err != nil {
		return errs.Wrap(err, "cs-api", "ingestTriples", "decode batch response")
	}
	if resp.Success {
		return nil
	}
	// Per graph/mutation_responses.go AddTriplesBatchResponse contract:
	//   - len(FailedSubjects) > 0  → per-entity CAS / validation failure
	//                                (caller-correctable input → 400)
	//   - len(FailedSubjects) == 0 → pre-CAS batch-level validation
	//                                (empty Subject/Predicate, malformed
	//                                envelope — caller-correctable → 400)
	// In both cases the framework's Error / per-subject message names a
	// real input problem; mapping either to 503 would mask client bugs
	// as infrastructure flake.
	if len(resp.FailedSubjects) > 0 {
		c.logger.Warn("ingest partial failure",
			"failed_subjects", resp.FailedSubjects,
			"written_count", resp.WrittenCount)
		pickKey := firstSortedKey(resp.FailedSubjects)
		return errs.WrapInvalid(
			fmt.Errorf("entity %s rejected: %s", pickKey, resp.FailedSubjects[pickKey]),
			"cs-api", "ingestTriples", "graph rejected entity",
		)
	}
	c.logger.Warn("ingest batch-level validation failure", "err", resp.Error)
	return errs.WrapInvalid(errors.New(resp.Error), "cs-api", "ingestTriples", "graph rejected batch")
}

// firstSortedKey picks the lexically-first key of m. Used to surface a
// deterministic per-Subject failure in the error body — map iteration
// is non-deterministic and we don't want the error message to flap
// across retries of the same failing request.
func firstSortedKey(m map[string]string) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	if len(keys) == 0 {
		return ""
	}
	return keys[0]
}
