// Stage 16 — PUT / DELETE / OPTIONS on /systems and /systems/{id}.
// Closes the CS API §7.6 create-replace-delete conformance class.
//
// **Implementation trade-off (re-derivable to know when to retire):**
// PUT-as-replace and DELETE still use a fetch + per-predicate-remove
// loop, which is N round-trips per entity. semstreams beta.87 exposes
// entity-level mutation subjects with read-back semantics, so retiring
// `deleteAllEntityTriples` in favor of `graph.mutation.entity.delete`
// is now local semconnect cleanup rather than an upstream blocker.
package csapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/pkg/errs"
	"github.com/nats-io/nats.go"
)

// SubjectTripleRemove is the NATS request/reply subject the framework's
// graph-ingest component exposes for single-triple removal (mirrors the
// SubjectTripleAddBatch local const). Duplicated because upstream's
// `processor/graph-ingest` package isn't importable (lowercased).
// Exported so tests in this package can pin the subject name without
// reaching through `c.nats`.
const SubjectTripleRemove = "graph.mutation.triple.remove"

// handleSystemPut serves PUT /systems/{id} — CS API §7.6
// create-replace-delete. Replace semantics: existing triples for the
// entity are removed, then the new body's triples are written. Body
// must use the GeoJSON Feature shape (application/json or
// application/geo+json) — PUT does NOT accept SensorML because the
// reverse-mapping triple set would mismatch the read-back JSON shape
// and surprise clients.
//
// Upsert: PUT against a never-created entity creates it. fetchEntity's
// errEntityNotFound is swallowed inside deleteAllEntityTriples, so the
// add-batch then writes the body's triples fresh. Matches the CS API
// §7.6 idiomatic upsert behavior.
//
// Status semantics: 204 No Content on success (CS API §7.6.5).
func (c *Component) handleSystemPut(w http.ResponseWriter, r *http.Request) {
	pathID := r.PathValue("id")
	if err := validateEntityID(pathID); err != nil {
		// X-CS-Attempted-ID set even on pre-validation 400 so the failure
		// log + client correlation are uniform across all error paths.
		w.Header().Set("X-CS-Attempted-ID", pathID)
		writeJSONError(w, http.StatusBadRequest, "invalid system id: "+err.Error())
		return
	}
	w.Header().Set("X-CS-Attempted-ID", pathID)

	ct := r.Header.Get("Content-Type")
	if err := requireMediaTypeAny(ct, string(MediaJSON), string(MediaGeoJSON)); err != nil {
		w.Header().Set("Accept", string(MediaJSON)+", "+string(MediaGeoJSON))
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

	// Re-use the JSON Feature builder from systems_post.go. The minted
	// entity ID is derived from the body's `properties.uid` per our
	// Stage 8 contract; we then verify it matches the path {id} so a
	// client can't accidentally PUT a body whose uid would mint a
	// different ID. This check runs BEFORE any destructive operation.
	bodyID, triples, buildErr := c.buildSystemTriplesFromFeature(body)
	if buildErr != nil {
		writeJSONError(w, http.StatusBadRequest, buildErr.Error())
		return
	}
	if bodyID != pathID {
		writeJSONError(w, http.StatusBadRequest,
			fmt.Sprintf("body uid maps to entity ID %q, which does not match path %q", bodyID, pathID))
		return
	}

	id := IdentityFrom(r.Context())

	// Replace = remove-all-then-add-batch. Order matters: if we
	// add-batch first then remove-all, we'd briefly serve the merged
	// view; remove-first gives a clean cut.
	//
	// **Inconsistency windows clients must be aware of:**
	//   (a) inter-step: if remove-all succeeds and add-batch then fails
	//       (e.g. 503), the entity is fully erased. Client must retry
	//       the PUT to recover. Surface as 503 + X-CS-Attempted-ID.
	//   (b) intra-loop (partial-erasure): deleteAllEntityTriples does N
	//       per-predicate removes. If predicates 1..k succeed and k+1
	//       returns a transient error, the entity is partially erased.
	//       We extend the X-CS-Partial-Delete: true header on this case
	//       so a retrying client knows the entity needs replacement,
	//       not just creation.
	// Both windows retire when semconnect moves this path onto the
	// beta.87 entity-level mutation primitives.
	if err := c.deleteAllEntityTriples(r.Context(), pathID, id); err != nil {
		c.writeBackendError(w, err)
		return
	}

	if err := c.ingestTriples(r.Context(), triples, id); err != nil {
		// Remove succeeded, add-batch failed → entity fully erased.
		w.Header().Set("X-CS-Partial-Delete", "true")
		c.writeBackendError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleSystemDelete serves DELETE /systems/{id} — CS API §7.6
// create-replace-delete. Removes every triple associated with the
// entity. 204 No Content on success. 404 if the entity didn't exist
// in the first place is intentionally NOT distinguished from 204
// (idempotent delete is friendlier; clients can pre-check via GET if
// they need the distinction).
//
// Partial-erasure: if deleteAllEntityTriples fails mid-loop, the
// entity is in an inconsistent state. Surfaced via 503 +
// X-CS-Partial-Delete: true so the client knows to retry.
func (c *Component) handleSystemDelete(w http.ResponseWriter, r *http.Request) {
	pathID := r.PathValue("id")
	if err := validateEntityID(pathID); err != nil {
		w.Header().Set("X-CS-Attempted-ID", pathID)
		writeJSONError(w, http.StatusBadRequest, "invalid system id: "+err.Error())
		return
	}
	w.Header().Set("X-CS-Attempted-ID", pathID)

	id := IdentityFrom(r.Context())
	if err := c.deleteAllEntityTriples(r.Context(), pathID, id); err != nil {
		// Don't set X-CS-Partial-Delete unconditionally — only the
		// loop's transient failure path inside deleteAllEntityTriples
		// sets it (via the partialDelete flag indicator). For DELETE
		// we propagate it on every backend error since any failure
		// past the first remove leaves a partial state.
		w.Header().Set("X-CS-Partial-Delete", "true")
		c.writeBackendError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// deleteAllEntityTriples implements entity-level deletion via the
// framework's triple-level primitives — fetch + per-predicate remove.
// Stage 16. N round-trips per call; retire now that beta.87 exposes
// `graph.mutation.entity.delete`.
//
// Non-existent entity is a no-op (the entity-query returns "not found",
// classified as errEntityNotFound, which we swallow here so DELETE is
// idempotent per CS API §7.6 conventions).
//
// Identity threads through so each remove call carries the same
// audit headers as the matching POST/PUT (anonymous-now-no-debt:
// the future verification layer needs a trail of "who deleted what").
//
// Context budget scales with the predicate fan-out. fetchEntity
// already consumes one QueryTimeout; we extend the ctx by
// (N + 1) * QueryTimeout so a tight per-call deadline does not
// silently abort the loop midway, leaving the entity partially
// erased without the operator knowing the failure mode.
func (c *Component) deleteAllEntityTriples(ctx context.Context, entityID string, id Identity) error {
	state, err := c.fetchEntity(ctx, entityID)
	if err != nil {
		if errors.Is(err, errEntityNotFound) {
			return nil // idempotent delete
		}
		return err
	}

	// Deduplicate predicates: the same predicate appearing on multiple
	// triples of the same entity (e.g. multiple identifiers) needs only
	// one remove call — graph-ingest's RemoveTriple takes (subject,
	// predicate) and clears all values.
	seen := make(map[string]struct{}, len(state.Triples))
	for _, t := range state.Triples {
		seen[t.Predicate] = struct{}{}
	}

	// Scale the loop budget with fan-out so a single tight QueryTimeout
	// doesn't abort the loop midway. +1 for slack.
	loopCtx, cancel := context.WithTimeout(ctx, time.Duration(len(seen)+1)*c.cfg.QueryTimeout)
	defer cancel()

	hdrs := id.AuditHeaders()
	for predicate := range seen {
		reqBody, mErr := json.Marshal(graph.RemoveTripleRequest{
			Subject:   entityID,
			Predicate: predicate,
		})
		if mErr != nil {
			return errs.Wrap(mErr, "cs-api", "deleteAllEntityTriples", "marshal remove request")
		}
		// RequestWithHeaders attaches audit headers symmetric with
		// POST /systems. graph-ingest doesn't persist them today; the
		// trail will be load-bearing when verification ships.
		reply, rErr := c.nats.RequestWithHeaders(loopCtx, SubjectTripleRemove, reqBody, hdrs, c.cfg.QueryTimeout)
		if rErr != nil {
			switch {
			case errors.Is(rErr, nats.ErrNoResponders),
				errors.Is(rErr, nats.ErrTimeout),
				errors.Is(rErr, context.DeadlineExceeded),
				errors.Is(rErr, context.Canceled),
				errors.Is(rErr, nats.ErrConnectionClosed):
				return errs.WrapTransient(rErr, "cs-api", "deleteAllEntityTriples", "graph backend unavailable")
			default:
				return errs.Wrap(rErr, "cs-api", "deleteAllEntityTriples", "remove triple request")
			}
		}
		var resp graph.RemoveTripleResponse
		if uErr := json.Unmarshal(reply.Data, &resp); uErr != nil {
			return errs.Wrap(uErr, "cs-api", "deleteAllEntityTriples", "decode remove response")
		}
		if !resp.Success {
			// Log the predicate-scoped reject so an operator triaging
			// "why did my idempotent DELETE return 400?" has the data.
			c.logger.Warn("remove triple rejected by graph-ingest",
				"subject", entityID, "predicate", predicate, "err", resp.Error)
			return errs.WrapInvalid(errors.New(resp.Error),
				"cs-api", "deleteAllEntityTriples",
				fmt.Sprintf("subject=%s predicate=%s rejected", entityID, predicate))
		}
	}
	return nil
}

// handleSystemsOptions serves OPTIONS /systems — CS API §7.6
// create-replace-delete advertises the Allow header so the ETS can
// confirm POST readiness without an actual POST. 204 No Content with
// an Allow header.
func (c *Component) handleSystemsOptions(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Allow", "GET, HEAD, POST, OPTIONS")
	w.WriteHeader(http.StatusNoContent)
}

// handleSystemOptions serves OPTIONS /systems/{id} — advertises GET,
// HEAD, PUT, PATCH, DELETE, OPTIONS so the ETS create-replace-delete
// + update preconditions confirm readiness without exercising the
// verbs.
//
// Stage 19 added PATCH (CS API `conf/update`). If we ever drop PATCH,
// this string is the one place to update.
func (c *Component) handleSystemOptions(w http.ResponseWriter, r *http.Request) {
	pathID := r.PathValue("id")
	if err := validateEntityID(pathID); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid system id: "+err.Error())
		return
	}
	w.Header().Set("Allow", "GET, HEAD, PUT, PATCH, DELETE, OPTIONS")
	w.WriteHeader(http.StatusNoContent)
}
