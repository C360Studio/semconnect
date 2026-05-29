// Stage 19 — PATCH /systems/{id} for the CS API `conf/update`
// conformance class. Body shape mirrors POST/PUT (GeoJSON Feature),
// but semantics are partial-update: only fields present in the body
// get replaced; fields absent are left alone.
//
// We do NOT implement JSON Merge Patch (RFC 7396) null-as-delete
// semantics at v0.1. A future stage can add that — the ETS doesn't
// exercise it. Documented in the handler doc comment + OAS3.
package csapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/parser/sensorml"
)

// handleSystemPatch serves PATCH /systems/{id} — CS API §7.6.
//
// Partial-update semantics: each present field in the body replaces
// the corresponding triple on the entity; absent fields are left
// alone. Body schema is the same SystemFeature shape PUT accepts
// (JSON or geo+json), but the only required field is `type:
// "Feature"` — properties.uid is checked-if-present against the
// existing entity (mismatch → 400; consistent with PUT's safety
// gate) but not required (the path is authoritative).
//
// Implementation: read the existing entity, merge body fields onto
// its triple set, then route through the same delete-all + add-batch
// path PUT uses. N round-trips per call (entity-query + N
// per-predicate removes + 1 add-batch); retire alongside PUT when
// semconnect moves onto the beta.86 entity-level mutation subjects.
//
// **No `properties.geometry: null` support** — RFC 7396 says null
// removes the field, but CS API spec doesn't explicitly require
// JSON Merge Patch and the ETS doesn't exercise it. Treating null
// as a no-op (rather than a remove) is the conservative v0.1
// stance.
func (c *Component) handleSystemPatch(w http.ResponseWriter, r *http.Request) {
	pathID := r.PathValue("id")
	if err := validateEntityID(pathID); err != nil {
		w.Header().Set("X-CS-Attempted-ID", pathID)
		writeJSONError(w, http.StatusBadRequest, "invalid system id: "+err.Error())
		return
	}
	w.Header().Set("X-CS-Attempted-ID", pathID)

	if err := requireMediaTypeAny(r.Header.Get("Content-Type"),
		string(MediaJSON), string(MediaGeoJSON)); err != nil {
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

	// Decode into the same shape POST/PUT uses but apply more
	// permissive validation — properties.uid is optional on PATCH
	// (path is authoritative), and we ignore the `type` field's
	// strict equality if absent (some PATCH clients omit it).
	var feat systemFeatureBody
	if err := json.Unmarshal(body, &feat); err != nil {
		writeJSONError(w, http.StatusBadRequest,
			fmt.Sprintf("invalid JSON Feature: %s", err.Error()))
		return
	}
	// Allow type omission (PATCH is implicitly typed by the URI);
	// reject only an explicit non-Feature type to catch outright
	// shape errors.
	if feat.Type != "" && feat.Type != "Feature" {
		writeJSONError(w, http.StatusBadRequest,
			fmt.Sprintf("expected Feature, got %q", feat.Type))
		return
	}

	identity := IdentityFrom(r.Context())

	existing, err := c.fetchEntity(r.Context(), pathID)
	if err != nil {
		// 404 surfaces through the normal classifier — PATCH against
		// a non-existent entity is a client error, not an upsert
		// (PUT does upsert; PATCH does not — partial-update of
		// nothing is meaningless).
		c.writeBackendError(w, err)
		return
	}

	// If the body specifies a uid, it MUST match the entity's
	// preserved uid (framework uid triple). Two cases:
	//
	//   (a) existing entity has a uid triple → strict equality check
	//       (mismatch → 400 before any destructive op).
	//   (b) existing entity has NO uid triple (POSTed pre-Stage-18,
	//       or via a builder that didn't emit one) → reject. PATCH
	//       cannot establish an identity retroactively without
	//       proving the client knows what they're claiming; quietly
	//       appending a uid would let a malicious or confused client
	//       overwrite the entity's identity on a partial-update
	//       request. The honest path is to re-POST/PUT to rebuild
	//       the entity with a uid, or omit `properties.uid` from
	//       the PATCH.
	if feat.Properties.UID != "" {
		existingUID, _ := firstSystemUIDObject(existing.Triples)
		if existingUID == "" {
			writeJSONError(w, http.StatusBadRequest,
				"existing entity has no preserved uid; PATCH cannot establish one — re-POST or PUT to rebuild")
			return
		}
		if feat.Properties.UID != existingUID {
			writeJSONError(w, http.StatusBadRequest,
				fmt.Sprintf("body properties.uid %q does not match existing entity uid %q",
					feat.Properties.UID, existingUID))
			return
		}
	}

	merged := mergePatchSystemTriples(pathID, existing.Triples, feat)

	// Same two-step replace as PUT: delete-all-then-add-batch on the
	// merged triple set. Carries the same partial-erasure window —
	// surfaced via X-CS-Partial-Delete on the add-batch failure path.
	if err := c.deleteAllEntityTriples(r.Context(), pathID, identity); err != nil {
		c.writeBackendError(w, err)
		return
	}
	if err := c.ingestTriples(r.Context(), merged, identity); err != nil {
		w.Header().Set("X-CS-Partial-Delete", "true")
		c.writeBackendError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// mergePatchSystemTriples produces the new triple set for a PATCH:
// existing triples are preserved verbatim EXCEPT where the body
// specifies a value for a field we know how to map (name →
// PredLabel, description → PredDescription, geometry →
// PredSystemPosition). For fields the body addresses but the entity
// lacks, the triple is appended. Fields the body doesn't address
// are left exactly as they were.
//
// The uid triple is NEVER replaced by PATCH (we already validated
// that any body uid matches the existing one; PATCH cannot mutate
// the entity's identity).
func mergePatchSystemTriples(entityID string, existing []message.Triple, feat systemFeatureBody) []message.Triple {
	hasName := feat.Properties.Name != ""
	hasDescription := feat.Properties.Description != ""
	hasGeometry := len(feat.Geometry) > 0 && !bytes.Equal(feat.Geometry, jsonNull)

	out := make([]message.Triple, 0, len(existing)+3)
	var sawLabel, sawDescription, sawPosition bool
	for _, t := range existing {
		switch t.Predicate {
		case sensorml.PredLabel:
			sawLabel = true
			if hasName {
				out = append(out, message.Triple{
					Subject: entityID, Predicate: sensorml.PredLabel,
					Object: feat.Properties.Name,
				})
				continue
			}
		case sensorml.PredDescription:
			sawDescription = true
			if hasDescription {
				out = append(out, message.Triple{
					Subject: entityID, Predicate: sensorml.PredDescription,
					Object: feat.Properties.Description,
				})
				continue
			}
		case PredSystemPosition, legacyPredSystemPosition:
			sawPosition = true
			if hasGeometry {
				out = append(out, message.Triple{
					Subject: entityID, Predicate: PredSystemPosition,
					Object: string(feat.Geometry),
				})
				continue
			}
		}
		out = append(out, t)
	}
	// Body fields the entity didn't already have — append fresh.
	if hasName && !sawLabel {
		out = append(out, message.Triple{
			Subject: entityID, Predicate: sensorml.PredLabel,
			Object: feat.Properties.Name,
		})
	}
	if hasDescription && !sawDescription {
		out = append(out, message.Triple{
			Subject: entityID, Predicate: sensorml.PredDescription,
			Object: feat.Properties.Description,
		})
	}
	if hasGeometry && !sawPosition {
		out = append(out, message.Triple{
			Subject: entityID, Predicate: PredSystemPosition,
			Object: string(feat.Geometry),
		})
	}
	return out
}
