// Stage 17 — PUT / DELETE / OPTIONS on /datastreams and
// /datastreams/{id}. Brings /datastreams parity with /systems
// (Stage 16) so the `create-replace-delete` conformance class
// claim is honest across both resource types the IUT implements.
//
// Implementation re-uses the entity mutation helpers from the systems
// write path so write semantics and audit headers stay symmetric.
package csapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/c360studio/semstreams/pkg/errs"
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
	if len(in.Schema) > 0 {
		rel, err := c.createSchemaArtifact(r.Context(), pathID, PredDatastreamSchema, in.Schema, identity)
		if err != nil {
			c.writeBackendError(w, err)
			return
		}
		triples = append(triples, rel)
	}

	if err := c.putEntityTriples(r.Context(), pathID, triples, identity); err != nil {
		c.writeBackendError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleDatastreamDelete serves DELETE /datastreams/{id} — CS API
// §10.6. Idempotent (graph.mutation.entity.delete returns success even
// when the entity never existed).
//
// Stage 36 also purges observations published on the datastream's exact
// JetStream subject after graph deletion succeeds. If graph deletion
// succeeds but observation purge fails, the response is 503 with
// X-CS-Observation-Purge-Failed and X-CS-Partial-Delete so clients and
// operators know the resource graph is gone but stream state remains.
func (c *Component) handleDatastreamDelete(w http.ResponseWriter, r *http.Request) {
	pathID := r.PathValue("id")
	if err := validateEntityID(pathID); err != nil {
		w.Header().Set("X-CS-Attempted-ID", pathID)
		writeJSONError(w, http.StatusBadRequest, "invalid datastream id: "+err.Error())
		return
	}
	w.Header().Set("X-CS-Attempted-ID", pathID)

	identity := IdentityFrom(r.Context())
	if err := c.deleteEntity(r.Context(), pathID, identity); err != nil {
		c.writeBackendError(w, err)
		return
	}

	if err := c.purgeDatastreamObservations(r.Context(), pathID); err != nil {
		w.Header().Set("X-CS-Partial-Delete", "true")
		w.Header().Set("X-CS-Observation-Purge-Failed", "true")
		c.writeBackendError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (c *Component) purgeDatastreamObservations(ctx context.Context, datastreamID string) error {
	cleanerPtr := c.cleaner.Load()
	if cleanerPtr == nil {
		return errs.WrapTransient(errors.New("observation stream cleaner not initialized"),
			"cs-api", "handleDatastreamDelete", "purge observations")
	}
	cleaner := *cleanerPtr
	cctx, cancel := context.WithTimeout(ctx, c.cfg.PublishTimeout)
	defer cancel()

	subject := c.cfg.ObservationsSubjectPrefix + "." + datastreamID
	if err := cleaner.PurgeSubject(cctx, subject); err != nil {
		return classifyJetStreamErr(err, "handleDatastreamDelete", "purge observations")
	}
	return nil
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
