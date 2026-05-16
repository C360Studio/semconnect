package csapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/c360studio/semstreams/pkg/errs"
	"github.com/nats-io/nats.go"
)

// observationCollection is the CS API §11.3 ObservationCollection JSON
// shape. Mirrors systemCollection / datastreamCollection — same `items`
// field name per OGC Common §7.14 (see Stage 10 rationale).
//
// v0.1 paging signal:
//
//   - NumberMatched / NumberReturned are equal and reflect what came back
//     in this fetch (not the absolute count past the cursor). The proper
//     "messages remaining past lastSeq" answer requires
//     consumer.Info().NumPending after FetchNoWait — deferred follow-up.
//   - Truncated is a heuristic: true when len(items) == limit AND
//     lastSeq > 0. Two failure modes worth knowing:
//     (a) MaxAge purge between cursor and tail — page returns < limit
//         items even though there might be later sequences. Caller
//         walks the cursor on next request and finds them. False
//         negative (truncated=false but more pages exist).
//     (b) The genuine tail happened to fill the page — next link
//         points past the tail and the follow-up request returns
//         200 with empty items. Legal but wasteful. False positive
//         (truncated=true but no more pages).
//     Both fold away when NumPending lands.
type observationCollection struct {
	Type           string            `json:"type"` // "ObservationCollection"
	NumberMatched  int               `json:"numberMatched"`
	NumberReturned int               `json:"numberReturned"`
	Truncated      bool              `json:"truncated,omitempty"`
	Items          []json.RawMessage `json:"items"`
	Links          []link            `json:"links"`
}

// envelopeProbe is the minimal slice of message.BaseMessage we need at
// the read seam — just the inner OMS payload. We don't go through
// message.Decoder + payloadregistry because (a) the registry would need
// the OMS payload type registered on the gateway side, and (b) the OMS
// observation JSON is what the client posted in the first place — we
// don't reshape it on read, we hand it back verbatim.
type envelopeProbe struct {
	Payload json.RawMessage `json:"payload"`
}

// handleObservationsGet serves GET /datastreams/{datastreamID}/observations
// (Stage 11; CS API Part 2 §11.3).
//
// Flow:
//  1. Path-validate datastreamID (same rule as POST).
//  2. Negotiate Accept: application/json (collection wrapper) or
//     application/om+json (raw items, no wrapper).
//  3. Parse ?limit (1..MaxListLimit) and ?after (sequence cursor).
//  4. Build an OrderedConsumer filtered on cs-api.observations.{id},
//     starting at OptStartSeq when ?after was set (else DeliverAllPolicy).
//  5. Fetch up to limit messages with FetchNoWait so an empty stream
//     returns immediately instead of holding the request open.
//  6. For each message: unwrap BaseMessage → take .payload verbatim
//     into items[]. Track max sequence for the next-link cursor.
//  7. Truncated when len(items) == limit AND there's anything past the
//     cursor (best-effort signal — JetStream gives us the seq of the
//     last fetched message; the client paginates by re-requesting).
//
// Errors:
//   - 400 invalid id / limit / after
//   - 406 unsupported Accept
//   - 503 JetStream unavailable / consumer creation failed
//   - 500 envelope decode failure (server-side data integrity)
func (c *Component) handleObservationsGet(w http.ResponseWriter, r *http.Request) {
	datastreamID := r.PathValue("datastreamID")
	if err := validateEntityID(datastreamID); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid datastream id: "+err.Error())
		return
	}

	media, ok := NegotiateRequest(r, FamilyObservationCollection)
	if !ok {
		WriteNotAcceptable(w, FamilyObservationCollection)
		return
	}

	limit, err := parseLimit(r.URL.Query().Get("limit"), c.cfg.DefaultListLimit, c.cfg.MaxListLimit)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	// ?after=<sequence> — opaque cursor; we document it as a stream
	// sequence number so operators triaging a paging request can match
	// it against the JetStream stream's seq column. CS API §7 paging is
	// link-token-shaped; this is the v0.1 substrate the link cursor
	// rides on.
	var startSeq uint64
	if raw := r.URL.Query().Get("after"); raw != "" {
		parsed, perr := strconv.ParseUint(raw, 10, 64)
		if perr != nil {
			writeJSONError(w, http.StatusBadRequest, "after must be a positive integer sequence")
			return
		}
		startSeq = parsed
	}

	rdPtr := c.reader.Load()
	if rdPtr == nil {
		// Component not started or Stop() nilled the handle.
		c.writeBackendError(w, errs.WrapTransient(errors.New("observation stream reader not initialized"),
			"cs-api", "handleObservationsGet", "reader unset"))
		return
	}
	rd := *rdPtr

	items, lastSeq, err := c.fetchObservations(r.Context(), rd, datastreamID, limit, startSeq)
	if err != nil {
		c.writeBackendError(w, err)
		return
	}

	// Structured access log line keyed by Identity so reads stay
	// auditable through the same X-Forwarded-* seam the publish path
	// uses (observations.go publish headers). Anonymous-now-no-debt:
	// turning on real auth means the middleware-populated Identity
	// fields fill out; this log line keeps working unchanged.
	id := IdentityFrom(r.Context())
	c.logger.Info("observations read",
		"datastream", datastreamID,
		"limit", limit,
		"after", startSeq,
		"returned", len(items),
		"last_seq", lastSeq,
		"forwarded_user", id.Forwarded["User"],
		"forwarded_email", id.Forwarded["Email"],
	)

	// For application/om+json we return a JSON array of bare observation
	// payloads — CS API §11.3 NF (no-wrapper) mode for OMS-native clients.
	// For application/json we wrap in the ObservationCollection envelope.
	switch media {
	case MediaOMS:
		c.writeObservationsBare(w, r, items)
	default:
		c.writeObservationsWrapped(w, r, datastreamID, items, lastSeq, limit)
	}
}

// fetchObservations delegates to the streamReader (production wraps a
// JetStream OrderedConsumer + FetchNoWait; tests substitute) and unwraps
// each BaseMessage envelope to the inner OMS payload. Returns the
// payloads in stream-sequence order plus the highest sequence seen,
// suitable for a next-link cursor.
func (c *Component) fetchObservations(
	ctx context.Context,
	rd streamReader,
	datastreamID string,
	limit int,
	startSeq uint64,
) ([]json.RawMessage, uint64, error) {
	cctx, cancel := context.WithTimeout(ctx, c.cfg.QueryTimeout)
	defer cancel()
	subject := c.cfg.ObservationsSubjectPrefix + "." + datastreamID
	msgs, err := rd.FetchSubject(cctx, subject, limit, startSeq)
	if err != nil {
		// Classify uniformly — no special timeout exception. FetchNoWait
		// shouldn't normally return ErrTimeout, but if it does we treat
		// it as transient like the publish path does; partial-batch on
		// transient-error is an edge case we'd rather report than mask.
		return nil, 0, classifyJetStreamErr(err, "handleObservationsGet", "fetch")
	}

	items := make([]json.RawMessage, 0, len(msgs))
	var lastSeq uint64
	for _, msg := range msgs {
		if msg.Sequence > lastSeq {
			lastSeq = msg.Sequence
		}
		var env envelopeProbe
		if uerr := json.Unmarshal(msg.Data, &env); uerr != nil || len(env.Payload) == 0 {
			// Skipping a malformed envelope rather than 500-ing the
			// whole request — one bad publish shouldn't poison the
			// page. Logged so operators can grep for "envelope decode"
			// and bisect against the offending stream sequence.
			c.logger.Warn("observation envelope decode failed; skipping",
				"datastream", datastreamID, "seq", msg.Sequence, "err", uerr)
			continue
		}
		items = append(items, env.Payload)
	}
	return items, lastSeq, nil
}

func (c *Component) writeObservationsWrapped(
	w http.ResponseWriter,
	r *http.Request,
	datastreamID string,
	items []json.RawMessage,
	lastSeq uint64,
	limit int,
) {
	links := []link{
		{Href: c.observationsSelfLink(datastreamID, r.URL.RawQuery), Rel: "self", Type: string(MediaJSON)},
	}
	truncated := len(items) == limit && lastSeq > 0
	if truncated {
		links = append(links, link{
			Href: fmt.Sprintf("/datastreams/%s/observations?limit=%d&after=%d",
				datastreamID, limit, lastSeq),
			Rel:  "next",
			Type: string(MediaJSON),
		})
	}

	coll := observationCollection{
		Type:           "ObservationCollection",
		NumberMatched:  len(items),
		NumberReturned: len(items),
		Truncated:      truncated,
		Items:          items,
		Links:          links,
	}

	w.Header().Set("Content-Type", string(MediaJSON))
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	if encErr := json.NewEncoder(w).Encode(coll); encErr != nil {
		c.errs.Add(1)
		c.logger.Error("encode observations response", "err", encErr)
	}
}

// writeObservationsBare emits a JSON array of bare OMS payloads (no CS
// API wrapper). Paging is signalled via response headers since
// application/om+json has no collection-shape link slot.
func (c *Component) writeObservationsBare(w http.ResponseWriter, r *http.Request, items []json.RawMessage) {
	w.Header().Set("Content-Type", string(MediaOMS))
	w.Header().Set("X-CS-Items-Count", strconv.Itoa(len(items)))
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	// Build a top-level JSON array manually so we don't unmarshal each
	// payload back to interface{} just to re-marshal it — preserve the
	// exact bytes the client posted.
	if _, err := w.Write([]byte("[")); err != nil {
		c.errs.Add(1)
		return
	}
	for i, item := range items {
		if i > 0 {
			if _, err := w.Write([]byte(",")); err != nil {
				c.errs.Add(1)
				return
			}
		}
		if _, err := w.Write(item); err != nil {
			c.errs.Add(1)
			return
		}
	}
	if _, err := w.Write([]byte("]")); err != nil {
		c.errs.Add(1)
		return
	}
}

func (c *Component) observationsSelfLink(datastreamID, rawQuery string) string {
	base := "/datastreams/" + datastreamID + "/observations"
	if rawQuery != "" {
		return base + "?" + rawQuery
	}
	return base
}

// classifyJetStreamErr maps JetStream / NATS sentinels to pkg/errs
// classes so writeBackendError can land them on the right HTTP code.
// Mirrors the table in observations.go (publish path) — same
// transients-to-503, unclassified-to-500 policy.
func classifyJetStreamErr(err error, op, hint string) error {
	switch {
	case errors.Is(err, nats.ErrNoResponders),
		errors.Is(err, nats.ErrTimeout),
		errors.Is(err, context.DeadlineExceeded),
		errors.Is(err, nats.ErrConnectionClosed):
		return errs.WrapTransient(err, "cs-api", op, hint)
	default:
		return errs.Wrap(err, "cs-api", op, hint)
	}
}
