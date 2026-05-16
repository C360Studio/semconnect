package csapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/message/oms"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/pkg/errs"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
)

const (
	// CS API Part 2 §11.9 — OMS Observation JSON encoding.
	mediaOMSObservation = string(MediaOMS)
	// Source recorded on the BaseMessage envelope. Audit-trail consumers
	// filter on this to find publishes originated by the cs-api gateway.
	publishSource = "cs-api-ingest"
)

// handleObservationsPost serves POST /datastreams/{datastreamID}/observations
// (CS API Part 2 §11.5).
//
// Flow:
//  1. Path-validate datastreamID (non-empty, NATS-token-safe).
//  2. Content-Type must be application/om+json.
//  3. Decode body → oms.Observation; Validate enforces the OMS-required
//     fields (procedure / observedProperty / resultTime).
//  4. Wrap in a BaseMessage envelope (the framework discipline note in
//     docs/000-getting-started.md §"Memory / discipline notes": every
//     publish wraps, even when the obvious consumer reads raw).
//  5. Build *nats.Msg with the BaseMessage bytes + audit headers from
//     the request Identity + the trace context.
//  6. Publish to the cs-api observations stream via js.PublishMsg.
//  7. Reply 201 Created with Location: /datastreams/{id}/observations/{obs.id}.
//
// Errors map to CS API status codes:
//   - 400 if path / body / OMS validation fails
//   - 413 if MaxBytesReader trips (handled by middleware)
//   - 415 if Content-Type is not application/om+json
//   - 503 if JetStream is unavailable
func (c *Component) handleObservationsPost(w http.ResponseWriter, r *http.Request) {
	datastreamID := r.PathValue("datastreamID")
	if err := validateEntityID(datastreamID); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid datastream id: "+err.Error())
		return
	}

	if err := requireMediaType(r.Header.Get("Content-Type"), mediaOMSObservation); err != nil {
		w.Header().Set("Accept-Post", mediaOMSObservation)
		writeJSONError(w, http.StatusUnsupportedMediaType, err.Error())
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		// MaxBytesReader returns *http.MaxBytesError when the cap is
		// exceeded — surface as 413, not 500.
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeJSONError(w, http.StatusRequestEntityTooLarge,
				fmt.Sprintf("request body exceeds %d bytes", maxErr.Limit))
			return
		}
		writeJSONError(w, http.StatusBadRequest, "could not read request body")
		return
	}

	var obs oms.Observation
	if err := json.Unmarshal(body, &obs); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid OMS Observation JSON: "+err.Error())
		return
	}
	if err := obs.Validate(); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	// Mint an ID when the client did not supply one — CS API §11.5 expects
	// the server to guarantee an addressable identity for the new
	// resource, and we cannot return a meaningful Location header without
	// one. Backfilled into obs before envelope wrap so consumers see it.
	if obs.ID == "" {
		obs.ID = uuid.NewString()
	}

	id := IdentityFrom(r.Context())
	subject := c.cfg.ObservationsSubjectPrefix + "." + datastreamID

	if err := c.publishObservation(r.Context(), subject, &obs, id); err != nil {
		c.writeBackendError(w, err)
		return
	}

	w.Header().Set("Content-Type", string(MediaJSON))
	w.Header().Set("Location", "/datastreams/"+datastreamID+"/observations/"+obs.ID)
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(struct {
		Status  string `json:"status"`
		ID      string `json:"id"`
		Subject string `json:"subject"`
	}{Status: "accepted", ID: obs.ID, Subject: subject})
}

// publishObservation wraps obs in a BaseMessage and publishes it to subject,
// attaching audit headers from id. The publish path bypasses
// natsclient.PublishToStream so we can add audit headers — natsclient's
// helper does not expose a headers parameter.
//
// Errors are classified at this boundary so writeBackendError maps cleanly.
func (c *Component) publishObservation(ctx context.Context, subject string, obs *oms.Observation, id Identity) error {
	pubPtr := c.publisher.Load()
	if pubPtr == nil {
		// Start() has not run yet, or EnsureStream failed and we're in a
		// degraded state. Either way this is server-side, not the client's
		// fault — surface as Transient → 503.
		return errs.WrapTransient(errors.New("publisher not initialized"), "cs-api", "publishObservation", "stream handle")
	}
	publisher := *pubPtr

	envelope := message.NewBaseMessage(oms.SchemaType(), obs, publishSource)
	envelopeJSON, err := json.Marshal(envelope)
	if err != nil {
		return errs.Wrap(err, "cs-api", "publishObservation", "marshal envelope")
	}

	msg := &nats.Msg{
		Subject: subject,
		Data:    envelopeJSON,
		Header:  nats.Header{},
	}
	for k, v := range id.AuditHeaders() {
		msg.Header.Set(k, v)
	}
	// Tag the publish with the BaseMessage type for downstream filters
	// that route on header alone (faster than payload introspection).
	msg.Header.Set("X-CS-Payload-Type", oms.SchemaType().String())

	pubCtx, cancel := context.WithTimeout(ctx, c.cfg.PublishTimeout)
	defer cancel()
	// Match natsclient.PublishToStream's auto-trace behavior. Dropping
	// to raw js.PublishMsg for audit headers would otherwise strip
	// `traceparent` / `X-Trace-ID`, breaking cross-component correlation
	// (every other producer in the framework injects these for free).
	if _, ok := natsclient.TraceContextFromContext(pubCtx); !ok {
		pubCtx = natsclient.ContextWithTrace(pubCtx, natsclient.NewTraceContext())
	}
	natsclient.InjectTrace(pubCtx, msg)

	if _, err := publisher.PublishMsg(pubCtx, msg); err != nil {
		switch {
		case errors.Is(err, nats.ErrNoResponders),
			errors.Is(err, nats.ErrTimeout),
			errors.Is(err, context.DeadlineExceeded),
			errors.Is(err, nats.ErrConnectionClosed):
			return errs.WrapTransient(err, "cs-api", "publishObservation", "jetstream unavailable")
		default:
			return errs.Wrap(err, "cs-api", "publishObservation", "publish to "+subject)
		}
	}
	return nil
}

// validateEntityID enforces the NATS-token-safe shape that SemStreams 6-part
// entity IDs follow. Reused for both datastream IDs and system IDs (both
// are SemStreams entity IDs at the wire level). Empty IDs are rejected;
// IDs containing NATS subject reserved characters (`*`, `>`, ` `) or empty
// tokens between dots (`acme..ops`, `.foo`, `foo.`) would break filter
// patterns and are rejected so a typo cannot poison the stream's wildcard
// semantics.
//
// Error messages refer to "id" generically; call sites prepend their own
// context ("invalid system id: ...", "invalid datastream id: ...").
func validateEntityID(id string) error {
	if id == "" {
		return errors.New("id required")
	}
	if strings.ContainsAny(id, " \t\r\n*>") {
		return errors.New("id contains reserved characters")
	}
	if len(id) > 256 {
		return errors.New("id exceeds 256 bytes")
	}
	for _, tok := range strings.Split(id, ".") {
		if tok == "" {
			return errors.New("id has empty token (leading/trailing/consecutive dots not allowed)")
		}
	}
	return nil
}

// requireMediaType verifies the Content-Type header against want. Returns an
// error suitable for a 415 response when it does not match.
func requireMediaType(got, want string) error {
	if got == "" {
		return fmt.Errorf("Content-Type required (expected %s)", want)
	}
	mt, _, err := mime.ParseMediaType(got)
	if err != nil {
		return fmt.Errorf("malformed Content-Type: %v", err)
	}
	if !strings.EqualFold(mt, want) {
		return fmt.Errorf("Content-Type %q not supported (expected %s)", mt, want)
	}
	return nil
}
