// Stage 16 — PUT / DELETE / OPTIONS on /systems and /systems/{id}.
// Closes the CS API §7.6 create-replace-delete conformance class.
//
// Stage 37 moved the write path onto semstreams entity-level mutation
// subjects, retiring the prior delete-all + add-batch partial-erasure
// window.
package csapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
)

// handleSystemPut serves PUT /systems/{id} — CS API §7.6
// create-replace-delete. Replace semantics: existing triples for the
// entity are replaced by the new body's triples through
// exact-revision representation reconcile. Body must use the GeoJSON
// Feature shape (application/json or
// application/geo+json) — PUT does NOT accept SensorML because the
// reverse-mapping triple set would mismatch the read-back JSON shape
// and surprise clients.
//
// Upsert: PUT against a never-created entity creates it via
// typed strict create. Matches the CS API §7.6
// idiomatic upsert behavior.
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

	if err := c.putEntityTriples(r.Context(), pathID, triples, id); err != nil {
		c.writeBackendError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleSystemDelete serves DELETE /systems/{id} — CS API §7.6
// create-replace-delete. Deletes the entity at its exact authority revision.
// 204 No Content on success; unknown IDs are still 204 because the
// framework primitive is idempotent.
func (c *Component) handleSystemDelete(w http.ResponseWriter, r *http.Request) {
	pathID := r.PathValue("id")
	if err := validateEntityID(pathID); err != nil {
		w.Header().Set("X-CS-Attempted-ID", pathID)
		writeJSONError(w, http.StatusBadRequest, "invalid system id: "+err.Error())
		return
	}
	w.Header().Set("X-CS-Attempted-ID", pathID)

	id := IdentityFrom(r.Context())
	if err := c.deleteEntity(r.Context(), pathID, id); err != nil {
		c.writeBackendError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (c *Component) putEntityTriples(ctx context.Context, entityID string, triples []message.Triple, id Identity) error {
	current, err := c.fetchEntityExact(ctx, entityID)
	if err != nil {
		if errors.Is(err, errEntityNotFound) {
			return c.ingestTriples(ctx, triples, id)
		}
		return err
	}
	return c.replaceEntityTriples(ctx, current, triples, id)
}

func (c *Component) replaceEntityTriples(
	ctx context.Context,
	current graph.ExactEntity,
	triples []message.Triple,
	id Identity,
) error {
	return c.reconcileEntity(ctx, current, triples, id, "replaceEntityTriples")
}

func (c *Component) deleteEntity(ctx context.Context, entityID string, id Identity) error {
	exact, err := c.fetchEntityExact(ctx, entityID)
	if err != nil {
		if errors.Is(err, errEntityNotFound) {
			return nil
		}
		return err
	}
	return c.deleteExactEntity(ctx, exact, id)
}

func uniquePredicates(triples []message.Triple) []string {
	seen := make(map[string]struct{}, len(triples))
	out := make([]string, 0, len(triples))
	for _, tr := range triples {
		if _, ok := seen[tr.Predicate]; ok {
			continue
		}
		seen[tr.Predicate] = struct{}{}
		out = append(out, tr.Predicate)
	}
	return out
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
