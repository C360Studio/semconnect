// Stage 17 — PUT / DELETE / OPTIONS on /datastreams and
// /datastreams/{id}. Brings /datastreams parity with /systems
// (Stage 16) so the `create-replace-delete` conformance class
// claim is honest across both resource types the IUT implements.
//
// Implementation re-uses deleteAllEntityTriples + ingestTriples
// from the Stage 16 fan-out path (same per-predicate N round-trips
// + same partial-erasure window + same audit-headers symmetry).
// Retires alongside Stage 16 when semconnect migrates to the
// beta.87 entity-level mutation subjects.
package csapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// handleDatastreamPut serves PUT /datastreams/{id} — CS API §10.6
// create-replace-delete. Replace-or-upsert semantics: existing
// triples for the entity are removed, then the request body's
// triples are written. If the entity does not exist, the body's
// triples are written fresh (standard PUT-as-upsert).
//
// Body media type is `application/json` only — the CS API §10
// Datastream JSON shape (no SensorML wrapper, mirrors POST). The
// body's `id` field (if present) must match the path `{id}` —
// mismatch yields 400 *before* any destructive operation.
//
// Status semantics: 204 No Content on success.
func (c *Component) handleDatastreamPut(w http.ResponseWriter, r *http.Request) {
	pathID := r.PathValue("id")
	if err := validateEntityID(pathID); err != nil {
		w.Header().Set("X-CS-Attempted-ID", pathID)
		writeJSONError(w, http.StatusBadRequest, "invalid datastream id: "+err.Error())
		return
	}
	w.Header().Set("X-CS-Attempted-ID", pathID)

	if err := requireMediaType(r.Header.Get("Content-Type"), string(MediaJSON)); err != nil {
		w.Header().Set("Accept", string(MediaJSON))
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

	var in Datastream
	if err := json.Unmarshal(body, &in); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid Datastream JSON: "+err.Error())
		return
	}

	// Same required-field validation POST applies — an upsert that
	// strips required fields would surprise read-back clients.
	if in.System == "" {
		writeJSONError(w, http.StatusBadRequest, "system required")
		return
	}
	if err := validateEntityIDStrict(in.System); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid system reference: "+err.Error())
		return
	}
	if in.ObservedProperty == "" {
		writeJSONError(w, http.StatusBadRequest, "observedProperty required")
		return
	}
	if schema, err := normalizeDatastreamSchema(in.Schema); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid Datastream schema: "+err.Error())
		return
	} else {
		in.Schema = schema
	}

	// Pre-flight ID check: if the body carries an `id`, it must equal
	// the path. An empty body `id` is permitted (path is authoritative).
	// Runs BEFORE any destructive operation so a mistaken body never
	// erases the targeted entity.
	if in.ID != "" && in.ID != pathID {
		writeJSONError(w, http.StatusBadRequest,
			fmt.Sprintf("body id %q does not match path %q", in.ID, pathID))
		return
	}

	triples := datastreamToTriples(pathID, &in)

	identity := IdentityFrom(r.Context())

	// Same two-step replace as Stage 16's PUT /systems/{id}: remove
	// all triples, then add the new batch. Same X-CS-Partial-Delete
	// signal when the add-batch fails after a successful remove.
	if err := c.deleteAllEntityTriples(r.Context(), pathID, identity); err != nil {
		c.writeBackendError(w, err)
		return
	}

	if err := c.ingestTriples(r.Context(), triples, identity); err != nil {
		// Remove succeeded, add-batch failed → entity fully erased.
		w.Header().Set("X-CS-Partial-Delete", "true")
		c.writeBackendError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleDatastreamDelete serves DELETE /datastreams/{id} — CS API
// §10.6. Idempotent (errEntityNotFound swallowed inside
// deleteAllEntityTriples → returns 204 even if the entity never
// existed). Removes every triple via per-predicate
// graph.mutation.triple.remove calls. A mid-loop transient failure
// surfaces as 503 + X-CS-Partial-Delete: true.
//
// **Important orphan note:** v0.1 does NOT cascade-delete the
// observations that reference this datastream. Observations live in
// the `cs-api.observations.{datastreamID}` JetStream, NOT in the
// triple graph; the framework's stream lifecycle is operator-managed.
// A future stage (probably Stage 18+) wires per-datastream JetStream
// `Consumer` cleanup. Documented in the OAS3 description so a
// client SDK doesn't ship with the wrong assumption.
func (c *Component) handleDatastreamDelete(w http.ResponseWriter, r *http.Request) {
	pathID := r.PathValue("id")
	if err := validateEntityID(pathID); err != nil {
		w.Header().Set("X-CS-Attempted-ID", pathID)
		writeJSONError(w, http.StatusBadRequest, "invalid datastream id: "+err.Error())
		return
	}
	w.Header().Set("X-CS-Attempted-ID", pathID)

	identity := IdentityFrom(r.Context())
	if err := c.deleteAllEntityTriples(r.Context(), pathID, identity); err != nil {
		w.Header().Set("X-CS-Partial-Delete", "true")
		c.writeBackendError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleDatastreamsOptions serves OPTIONS /datastreams — advertises
// the collection-level Allow header. PATCH intentionally absent.
func (c *Component) handleDatastreamsOptions(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Allow", "GET, HEAD, POST, OPTIONS")
	w.WriteHeader(http.StatusNoContent)
}

// handleDatastreamOptions serves OPTIONS /datastreams/{id} —
// advertises the item-level Allow header.
func (c *Component) handleDatastreamOptions(w http.ResponseWriter, r *http.Request) {
	pathID := r.PathValue("id")
	if err := validateEntityID(pathID); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid datastream id: "+err.Error())
		return
	}
	w.Header().Set("Allow", "GET, HEAD, PUT, PATCH, DELETE, OPTIONS")
	w.WriteHeader(http.StatusNoContent)
}
