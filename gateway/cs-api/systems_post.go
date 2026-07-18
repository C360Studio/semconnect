package csapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"regexp"
	"strings"

	"github.com/c360studio/semconnect/parser/sensorml"
	"github.com/c360studio/semconnect/vocabulary/sosa"
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/graph/geo/geojson"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/pkg/errs"
	semtypes "github.com/c360studio/semstreams/pkg/types"
	"github.com/c360studio/semstreams/vocabulary"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
)

// Entity-level mutation subjects exposed by semstreams graph-ingest.
// Duplicated locally because the upstream processor package is not a
// public API surface for gateways.
const (
	SubjectEntityCreateWithTriples = "graph.mutation.entity.create_with_triples"
	SubjectEntityUpdateWithTriples = "graph.mutation.entity.update_with_triples"
	SubjectEntityDelete            = "graph.mutation.entity.delete"
)

// PredSystemPosition and PredSystemUID are the framework-owned SensorML
// predicates semconnect uses for CS API uid / geometry round-trips. They
// are kept behind gateway-local names because Feature-shaped resources
// (deployments, sampling features, etc.) use the same graph predicates
// even when their HTTP representation is not SensorML.
const (
	PredSystemPosition = sensorml.PredPosition
	PredSystemUID      = sensorml.PredUniqueID
)

// jsonNull is the byte-equality target for detecting a literal JSON
// null in GeoJSON Feature geometry.
var jsonNull = []byte("null")

// Matches SemStreams' ADR-056 cs-api System projection contract. SensorML
// System writes can emit child->parent foreign edges, so graph-ingest needs a
// concrete producer key to classify the edge claim instead of "_invalid".
var systemProjectionMessageType = message.Type{Domain: "c360", Category: "csapi.system", Version: "v1"}

func firstSystemPositionObject(triples []message.Triple) (string, bool) {
	return firstStringObject(triples, PredSystemPosition)
}

func firstSystemUIDObject(triples []message.Triple) (string, bool) {
	return firstStringObject(triples, PredSystemUID)
}

// handleSystemPost serves POST /systems — CS API §7.6.
func (c *Component) handleSystemPost(w http.ResponseWriter, r *http.Request) {
	// Stage 14: accept SensorML in both spec form (sml+json) and legacy
	// long form (sensorml+json).
	// Stage 16: also accept application/json + application/geo+json for
	// the CS API §7.6 GeoJSON Feature body shape — what
	// CreateReplaceDelete ETS tests POST.
	ct := r.Header.Get("Content-Type")
	if err := requireMediaTypeAny(ct,
		string(MediaSensorML), string(MediaSensorMLLegacy),
		string(MediaJSON), string(MediaGeoJSON)); err != nil {
		w.Header().Set("Accept-Post", strings.Join([]string{
			string(MediaSensorML), string(MediaSensorMLLegacy),
			string(MediaJSON), string(MediaGeoJSON),
		}, ", "))
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

	// Branch by Content-Type. mime.ParseMediaType already validated the
	// header so we strip params here for a clean exact match.
	mt, _, _ := mime.ParseMediaType(ct)
	var (
		entityID string
		triples  []message.Triple
		buildErr error
	)
	switch mt {
	case string(MediaSensorML), string(MediaSensorMLLegacy):
		entityID, triples, buildErr = c.buildSystemTriplesFromSensorML(body)
	default:
		// application/json or application/geo+json — the GeoJSON Feature
		// body shape (Stage 16). CS API §7.6 explicitly lists both.
		entityID, triples, buildErr = c.buildSystemTriplesFromFeature(body)
	}
	if buildErr != nil {
		writeJSONError(w, http.StatusBadRequest, buildErr.Error())
		return
	}

	id := IdentityFrom(r.Context())
	if err := c.ingestProjectedTriples(r.Context(), entityID, triples, systemProjectionMessageType, id); err != nil {
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
	return mintEntityID(c.cfg.SystemIDPrefix, []byte(uniqueID))
}

// buildSystemTriplesFromSensorML — Stage 8/14 path. Parses body via
// `parser/sensorml.UnmarshalProcess` + `sensorml.NewAsset(...).Triples()`.
// semstreams beta.79+ emits uniqueId and position triples natively.
func (c *Component) buildSystemTriplesFromSensorML(body []byte) (string, []message.Triple, error) {
	process, err := sensorml.UnmarshalProcess(body)
	if err != nil {
		return "", nil, fmt.Errorf("invalid SensorML JSON: %w", err)
	}
	if process == nil || process.Base() == nil {
		return "", nil, errors.New("empty SensorML process")
	}
	entityID := c.mintSystemEntityID(process.Base().UniqueID)
	asset := sensorml.NewAsset(entityID, process)
	asset.ChildIDFn = func(localID string) string {
		return mintNestedSensorMLEntityID(entityID, localID)
	}
	triples := asset.Triples()
	if len(triples) == 0 {
		return entityID, nil, errors.New("SensorML process produced no representable triples")
	}
	if process.Base().Position != nil {
		triples = append(triples, pointProjectionTriplesFromGeometry(entityID, process.Base().Position.Raw)...)
	}
	return entityID, triples, nil
}

// systemFeatureBody is the minimum GeoJSON Feature shape CS API §7.6
// requires for a JSON System POST. ETS's CRD test sends exactly this
// (uid + name + description). Stage 16.
type systemFeatureBody struct {
	Type       string          `json:"type"`
	ID         string          `json:"id,omitempty"`
	Geometry   json.RawMessage `json:"geometry,omitempty"`
	Properties struct {
		UID                  string `json:"uid"`
		Name                 string `json:"name,omitempty"`
		Description          string `json:"description,omitempty"`
		Definition           string `json:"definition,omitempty"`
		DeployedSystemsLinks []link `json:"deployedSystems@link,omitempty"`
		HostedProcedureLink  *link  `json:"hostedProcedure@link,omitempty"`
		ParentID             string `json:"parent@id,omitempty"`
		ParentLink           *link  `json:"parent@link,omitempty"`
	} `json:"properties"`
}

// buildSystemTriplesFromFeature — Stage 16 path for POST /systems with
// Content-Type application/json or application/geo+json. The body is a
// GeoJSON Feature with the System fields under `properties`. We map:
//
//   - properties.uid → uniqueId source for entity-ID mint
//   - properties.name → sensorml.PredLabel
//   - properties.description → sensorml.PredDescription
//   - top-level geometry → sensorml.process.position triple, the same
//     framework predicate SensorML bodies emit
//   - properties.parent@id / parent@link.href → sensorml.PredIsHostedBy
//     so /systems/{id}/subsystems can expose composition without a gateway
//     local predicate
//   - rdf:type (sensorml.PredType) = sosa.SSNSystem so /systems
//     predicate query finds it
//
// The CS API §7.6 full GeoJSON schema (api/upstream/part1/schemas/geojson/system.json)
// has more properties (featureType, assetType, validTime, etc.). v0.1
// surfaces only what the ETS exercises + what we already round-trip via
// triples; widening the shape is a follow-up when a real client asks.
func (c *Component) buildSystemTriplesFromFeature(body []byte) (string, []message.Triple, error) {
	var feat systemFeatureBody
	if err := json.Unmarshal(body, &feat); err != nil {
		return "", nil, fmt.Errorf("invalid JSON Feature: %w", err)
	}
	if feat.Type != "Feature" {
		return "", nil, fmt.Errorf("expected Feature, got %q", feat.Type)
	}
	if feat.Properties.UID == "" {
		return "", nil, errors.New("properties.uid required")
	}
	entityID := c.mintSystemEntityID(feat.Properties.UID)
	triples := []message.Triple{
		{Subject: entityID, Predicate: sensorml.PredType, Object: sosa.SSNSystem},
		// Stage 18 — preserve the submitted properties.uid so a
		// follow-up GET (json / sml+json / geo+json reconstruction)
		// can echo it back. UID is required by the Feature builder so
		// this is unconditional here (unlike the SensorML path which
		// permits an empty uniqueId).
		{Subject: entityID, Predicate: PredSystemUID, Object: feat.Properties.UID},
	}
	if feat.Properties.Name != "" {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: sensorml.PredLabel, Object: feat.Properties.Name,
		})
	}
	if feat.Properties.Description != "" {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: sensorml.PredDescription, Object: feat.Properties.Description,
		})
	}
	triples = append(triples, locationTriplesFromGeometry(entityID, feat.Geometry)...)
	if parentID := parentIDFromSystemFeature(feat); parentID != "" {
		if err := validateEntityID(parentID); err != nil {
			return "", nil, fmt.Errorf("properties.parent@id invalid: %w", err)
		}
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: sensorml.PredIsHostedBy, Object: parentID, Datatype: message.EntityReferenceDatatype,
		})
	}
	return entityID, triples, nil
}

// locationTriplesFromGeometry preserves the submitted geometry for CS API /
// SensorML round-trip and separately projects GeoJSON Points into the public
// beta.147 predicates consumed by graph-index-spatial. Non-Point geometries
// remain representable but do not invent a centroid.
func locationTriplesFromGeometry(entityID string, geometry json.RawMessage) []message.Triple {
	if len(geometry) == 0 || bytes.Equal(geometry, jsonNull) {
		return nil
	}
	triples := []message.Triple{{
		Subject: entityID, Predicate: PredSystemPosition, Object: string(geometry),
	}}
	return append(triples, pointProjectionTriplesFromGeometry(entityID, geometry)...)
}

func pointProjectionTriplesFromGeometry(entityID string, geometry json.RawMessage) []message.Triple {
	geom, err := geojson.UnmarshalGeometry(geometry)
	if err != nil {
		return nil
	}
	point, ok := geom.(geojson.Point)
	if !ok || !point.Coordinates.Valid() {
		return nil
	}
	triples := []message.Triple{
		{Subject: entityID, Predicate: vocabulary.GeoLocationLongitude, Object: point.Coordinates.Lon()},
		{Subject: entityID, Predicate: vocabulary.GeoLocationLatitude, Object: point.Coordinates.Lat()},
	}
	if point.Coordinates.Has3D() {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: vocabulary.GeoLocationAltitude, Object: point.Coordinates.Alt(),
		})
	}
	return triples
}

func parentIDFromSystemFeature(feat systemFeatureBody) string {
	return parentIDFromFeature(feat, "/systems/")
}

func parentIDFromDeploymentFeature(feat systemFeatureBody) string {
	return parentIDFromFeature(feat, "/deployments/")
}

func parentIDFromFeature(feat systemFeatureBody, pathMarkers ...string) string {
	if feat.Properties.ParentID != "" {
		return feat.Properties.ParentID
	}
	if feat.Properties.ParentLink == nil {
		return ""
	}
	href := strings.TrimSpace(feat.Properties.ParentLink.Href)
	for _, marker := range pathMarkers {
		if i := strings.Index(href, marker); i >= 0 {
			href = href[i+len(marker):]
			break
		}
	}
	for _, sep := range []string{"?", "#", "/"} {
		if i := strings.Index(href, sep); i >= 0 {
			href = href[:i]
		}
	}
	return href
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

const (
	nestedSensorMLDigestDomain = "semconnect:sensorml-child:v1\x00"
	schemaArtifactDigestDomain = "semconnect:schema-artifact:v1\x00"
)

// mintEntityID preserves the existing sanitized instance whenever the full
// six-part ID fits. Overflow uses a full SHA-256 digest of the exact source
// bytes; no truncation or normalized input participates in the digest.
func mintEntityID(prefix string, exactSource []byte) string {
	return mintEntityIDWithToken(prefix, uniqueIDToToken(string(exactSource)), exactSource)
}

func mintEntityIDWithToken(prefix, ordinaryToken string, digestSource []byte) string {
	candidate := prefix + "." + ordinaryToken
	if semtypes.ValidateEntityID(candidate) == nil {
		return candidate
	}
	digest := sha256.Sum256(digestSource)
	return fmt.Sprintf("%s.h-%x", prefix, digest)
}

func mintNestedSensorMLEntityID(parentID, localID string) string {
	separator := strings.LastIndexByte(parentID, '.')
	if separator < 0 {
		return parentID + "_" + uniqueIDToToken(localID)
	}
	prefix := parentID[:separator]
	parentInstance := parentID[separator+1:]
	ordinaryToken := parentInstance + "_" + uniqueIDToToken(localID)
	digestSource := []byte(nestedSensorMLDigestDomain + parentID + "\x00" + localID)
	return mintEntityIDWithToken(prefix, ordinaryToken, digestSource)
}

func mintSchemaArtifactID(prefix, parentID, role string) string {
	ordinarySource := parentID + "-" + role
	digestSource := []byte(schemaArtifactDigestDomain + parentID + "\x00" + role)
	return mintEntityIDWithToken(prefix, uniqueIDToToken(ordinarySource), digestSource)
}

// ingestTriples creates one entity with its triples through
// graph.mutation.entity.create_with_triples via NATS request-reply,
// attaching audit headers from the request identity. Returns a
// classified error so writeBackendError maps cleanly to HTTP status.
//
// Timeout: QueryTimeout, NOT PublishTimeout. This is a request/reply
// (we wait for a reply), so it lives on the same budget as /systems and
// /systems/{id} reads. PublishTimeout is for fire-and-forget JetStream
// publishes (observations.go); using it here would couple two
// independently-tuned latency budgets.
func (c *Component) ingestTriples(ctx context.Context, triples []message.Triple, id Identity) error {
	entityID, err := singleSubject(triples)
	if err != nil {
		return errs.WrapInvalid(err, "cs-api", "ingestTriples", "invalid triple set")
	}
	return c.ingestProjectedTriples(ctx, entityID, triples, message.Type{}, id)
}

func (c *Component) ingestProjectedTriples(ctx context.Context, entityID string, triples []message.Triple, mt message.Type, id Identity) error {
	if err := validateProjectedTriples(entityID, triples); err != nil {
		return errs.WrapInvalid(err, "cs-api", "ingestProjectedTriples", "invalid triple set")
	}
	return c.createEntityWithTriples(ctx, &graph.EntityState{
		ID:          entityID,
		Triples:     triples,
		MessageType: mt,
	}, triples, id, "ingestTriples")
}

func (c *Component) createEntityWithTriples(
	ctx context.Context,
	entity *graph.EntityState,
	triples []message.Triple,
	id Identity,
	op string,
) error {
	if entity == nil {
		return errs.WrapInvalid(errors.New("entity state required"), "cs-api", op, "build entity")
	}
	if len(entity.Triples) == 0 {
		entity.Triples = triples
	}
	if err := validateProjectedTriples(entity.ID, entity.Triples); err != nil {
		return errs.WrapInvalid(err, "cs-api", op, "validate final entity state")
	}
	if err := validateProjectedTriples(entity.ID, triples); err != nil {
		return errs.WrapInvalid(err, "cs-api", op, "validate mutation triples")
	}
	req := graph.CreateEntityWithTriplesRequest{
		Entity:  entity,
		Triples: triples,
	}
	reqBody, err := json.Marshal(req)
	if err != nil {
		return errs.Wrap(err, "cs-api", op, "marshal entity create request")
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
	reply, err := c.nats.RequestWithHeaders(ctx, SubjectEntityCreateWithTriples, reqBody, hdrs, c.cfg.QueryTimeout)
	if err != nil {
		switch {
		case errors.Is(err, nats.ErrNoResponders),
			errors.Is(err, nats.ErrTimeout),
			errors.Is(err, context.DeadlineExceeded),
			errors.Is(err, nats.ErrConnectionClosed):
			return errs.WrapTransient(err, "cs-api", op, "graph backend unavailable")
		default:
			return errs.Wrap(err, "cs-api", op, "entity create request")
		}
	}

	data, err := classifyMutationReply(reply, op)
	if err != nil {
		return err
	}

	var resp graph.CreateEntityWithTriplesResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return errs.Wrap(err, "cs-api", op, "decode entity create response")
	}
	if resp.Degraded {
		c.logger.Warn("entity create committed with degraded read-back", "entity", entity.ID, "err", resp.DegradedReason)
	}
	return nil
}

func singleSubject(triples []message.Triple) (string, error) {
	if len(triples) == 0 {
		return "", errors.New("no triples to ingest")
	}
	subject := triples[0].Subject
	if subject == "" {
		return "", errors.New("triple subject is empty")
	}
	for i, tr := range triples[1:] {
		if tr.Subject != subject {
			return "", fmt.Errorf("triple[%d] subject %q does not match %q", i+1, tr.Subject, subject)
		}
	}
	return subject, nil
}

func validateProjectedTriples(entityID string, triples []message.Triple) error {
	if entityID == "" {
		return errors.New("entity id is empty")
	}
	if len(triples) == 0 {
		return errors.New("no triples to ingest")
	}
	hasPrimary := false
	for i, tr := range triples {
		if tr.Subject == "" {
			return fmt.Errorf("triple[%d] subject is empty", i)
		}
		if tr.Subject == entityID {
			hasPrimary = true
		}
	}
	if !hasPrimary {
		return fmt.Errorf("no triples target primary entity %q", entityID)
	}
	if _, err := graph.MarshalEntityState(&graph.EntityState{ID: entityID, Triples: triples}); err != nil {
		return fmt.Errorf("authoritative entity-state validation: %w", err)
	}
	return nil
}

func classifyMutationReply(reply *nats.Msg, op string) ([]byte, error) {
	data, err := natsclient.ClassifyReply(reply)
	if err != nil {
		return nil, mutationFailure(op, err)
	}
	return data, nil
}

func mutationFailure(op string, err error) error {
	if err == nil {
		return nil
	}
	var ce *errs.ClassifiedError
	if errors.As(err, &ce) {
		switch ce.Code {
		case graph.ErrorCodeEntityExists:
			return fmt.Errorf("%w: %s", errEntityConflict, err.Error())
		case graph.ErrorCodeEntityNotFound:
			return fmt.Errorf("%w: %s", errEntityNotFound, err.Error())
		case graph.ErrorCodeInvalidRequest, graph.ErrorCodeRevisionMismatch, graph.ErrorCodeOwnerLeaseStale:
			return errs.WrapInvalid(err, "cs-api", op, "graph rejected entity mutation")
		case graph.ErrorCodeInternal:
			return errs.WrapTransient(err, "cs-api", op, "graph backend mutation failed")
		}
	}
	if errs.IsInvalid(err) {
		return errs.WrapInvalid(err, "cs-api", op, "graph rejected entity mutation")
	}
	if errs.IsTransient(err) {
		return errs.WrapTransient(err, "cs-api", op, "graph backend mutation failed")
	}
	return errs.Wrap(err, "cs-api", op, "graph rejected entity mutation")
}
