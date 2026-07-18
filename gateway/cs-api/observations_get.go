package csapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/c360studio/semconnect/pkg/swecommon"
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
//     items even though there might be later sequences. Caller
//     walks the cursor on next request and finds them. False
//     negative (truncated=false but more pages exist).
//     (b) The genuine tail happened to fill the page — next link
//     points past the tail and the follow-up request returns
//     200 with empty items. Legal but wasteful. False positive
//     (truncated=true but no more pages).
//     Both fold away when NumPending lands.
type observationCollection struct {
	Type           string            `json:"type"` // "ObservationCollection"
	NumberMatched  int               `json:"numberMatched"`
	NumberReturned int               `json:"numberReturned"`
	Truncated      bool              `json:"truncated,omitempty"`
	Items          []json.RawMessage `json:"items"`
	Links          []link            `json:"links"`
}

func (c *Component) handleGlobalObservations(w http.ResponseWriter, r *http.Request) {
	media, ok := NegotiateRequest(r, FamilyDatastreamCollection)
	if !ok || media != MediaJSON {
		WriteNotAcceptable(w, FamilyDatastreamCollection)
		return
	}
	limit, err := parseLimit(r.URL.Query().Get("limit"), c.cfg.DefaultListLimit, c.cfg.MaxListLimit)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	filters := observationCollectionFiltersFromQuery(r.URL.Query())
	startSeq, err := parseObservationAfter(r.URL.Query().Get("after"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	fetchLimit := limit
	if filters.active() {
		fetchLimit = c.cfg.MaxListLimit
	}
	msgs, lastSeq, err := c.fetchObservationMessages(r.Context(), c.cfg.ObservationsSubjectPrefix+".>", fetchLimit, startSeq)
	if err != nil {
		c.writeBackendError(w, err)
		return
	}
	items := c.observationResourcesFromMessages(msgs, "")
	if filters.active() {
		items = filterObservationResources(items, filters, limit)
	}
	links := []link{
		{Href: observationsCollectionSelfLink(r.URL.RawQuery), Rel: "self", Type: string(MediaJSON)},
	}
	truncated := len(items) == limit && lastSeq > 0
	if truncated {
		links = append(links, link{
			Href: fmt.Sprintf("/observations?limit=%d&after=%d", limit, lastSeq),
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
	if err := json.NewEncoder(w).Encode(coll); err != nil {
		c.errs.Add(1)
		c.logger.Error("encode global observations response", "err", err)
	}
}

func (c *Component) handleGlobalObservationsOptions(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Allow", "GET, HEAD, OPTIONS")
	w.WriteHeader(http.StatusNoContent)
}

func (c *Component) handleObservationGet(w http.ResponseWriter, r *http.Request) {
	media, ok := NegotiateRequest(r, FamilyDatastreamCollection)
	if !ok || media != MediaJSON {
		WriteNotAcceptable(w, FamilyDatastreamCollection)
		return
	}
	obsID := r.PathValue("obsID")
	if err := validateOpaqueResourceID(obsID); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid observation id: "+err.Error())
		return
	}
	msgs, _, err := c.fetchObservationMessages(r.Context(), c.cfg.ObservationsSubjectPrefix+".>", c.cfg.MaxListLimit, 0)
	if err != nil {
		c.writeBackendError(w, err)
		return
	}
	for _, msg := range msgs {
		datastreamID := datastreamIDFromObservationSubject(c.cfg.ObservationsSubjectPrefix, msg.Subject)
		resource, ok := observationResourceFromPayload(msg.Data, datastreamID)
		if !ok || observationResourceID(resource) != obsID {
			continue
		}
		w.Header().Set("Content-Type", string(MediaJSON))
		w.WriteHeader(http.StatusOK)
		if r.Method == http.MethodHead {
			return
		}
		_, _ = w.Write(resource)
		return
	}
	writeJSONError(w, http.StatusNotFound, "observation not found")
}

func (c *Component) handleObservationOptions(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Allow", "GET, HEAD, OPTIONS")
	w.WriteHeader(http.StatusNoContent)
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
	filters := observationCollectionFiltersFromQuery(r.URL.Query())

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

	fetchLimit := limit
	if media == MediaJSON && filters.active() {
		fetchLimit = c.cfg.MaxListLimit
	}
	items, lastSeq, err := c.fetchObservations(r.Context(), rd, datastreamID, fetchLimit, startSeq)
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
	case MediaSWEJSON:
		c.writeObservationsSWEJSON(w, r, datastreamID, items)
	case MediaSWECsv:
		c.writeObservationsSWECsv(w, r, datastreamID, items)
	case MediaSWEBinary:
		c.writeObservationsSWEBinary(w, r, datastreamID, items)
	default:
		if filters.active() {
			resources := observationResourcesFromPayloads(items, datastreamID)
			c.writeObservationResourcesWrapped(w, r, datastreamID, filterObservationResources(resources, filters, limit), lastSeq, limit)
			return
		}
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
	subject := c.cfg.ObservationsSubjectPrefix + "." + datastreamID
	msgs, lastSeq, err := c.fetchObservationMessagesWithReader(ctx, rd, subject, limit, startSeq)
	if err != nil {
		return nil, 0, err
	}
	items := make([]json.RawMessage, 0, len(msgs))
	for _, msg := range msgs {
		if len(msg.Data) > 0 {
			items = append(items, msg.Data)
		}
	}
	return items, lastSeq, nil
}

func (c *Component) fetchObservationMessages(
	ctx context.Context,
	subject string,
	limit int,
	startSeq uint64,
) ([]observationMsg, uint64, error) {
	rdPtr := c.reader.Load()
	if rdPtr == nil {
		return nil, 0, errs.WrapTransient(errors.New("observation stream reader not initialized"),
			"cs-api", "fetchObservationMessages", "reader unset")
	}
	return c.fetchObservationMessagesWithReader(ctx, *rdPtr, subject, limit, startSeq)
}

func (c *Component) fetchObservationMessagesWithReader(
	ctx context.Context,
	rd streamReader,
	subject string,
	limit int,
	startSeq uint64,
) ([]observationMsg, uint64, error) {
	cctx, cancel := context.WithTimeout(ctx, c.cfg.QueryTimeout)
	defer cancel()
	msgs, err := rd.FetchSubject(cctx, subject, limit, startSeq)
	if err != nil {
		// Classify uniformly — no special timeout exception. FetchNoWait
		// shouldn't normally return ErrTimeout, but if it does we treat
		// it as transient like the publish path does; partial-batch on
		// transient-error is an edge case we'd rather report than mask.
		return nil, 0, classifyJetStreamErr(err, "handleObservationsGet", "fetch")
	}

	out := make([]observationMsg, 0, len(msgs))
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
				"subject", subject, "seq", msg.Sequence, "err", uerr)
			continue
		}
		if msg.Subject == "" {
			msg.Subject = subject
		}
		msg.Data = env.Payload
		out = append(out, msg)
	}
	return out, lastSeq, nil
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
		Items:          observationResourcesFromPayloads(items, datastreamID),
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

func (c *Component) writeObservationResourcesWrapped(
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

func (c *Component) writeObservationsSWEJSON(
	w http.ResponseWriter,
	r *http.Request,
	datastreamID string,
	items []json.RawMessage,
) {
	schema, rows, schemaBacked := c.sweRowsForDatastream(r, datastreamID, items)
	var body []byte
	if r.Method != http.MethodHead {
		var err error
		body, err = swecommon.MarshalJSONRows(schema, rows)
		if err != nil {
			c.errs.Add(1)
			c.logger.Error("encode observations SWE JSON response", "err", err)
			writeJSONError(w, http.StatusInternalServerError, "encode observations SWE JSON response")
			return
		}
	}
	w.Header().Set("Content-Type", string(MediaSWEJSON))
	if !schemaBacked {
		w.Header().Set("X-CS-SWE-Subset", "observation-values")
	}
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	if _, err := w.Write(body); err != nil {
		c.errs.Add(1)
		c.logger.Error("write observations SWE JSON response", "err", err)
	}
}

func (c *Component) writeObservationsSWECsv(w http.ResponseWriter, r *http.Request, datastreamID string, items []json.RawMessage) {
	schema, rows, schemaBacked := c.sweRowsForDatastream(r, datastreamID, items)
	var body []byte
	if r.Method != http.MethodHead {
		enc := swecommon.DefaultTextEncoding()
		enc.EmitHeader = !schemaBacked
		var err error
		body, err = swecommon.MarshalTextRows(schema, rows, enc)
		if err != nil {
			c.errs.Add(1)
			c.logger.Error("encode observations SWE CSV response", "err", err)
			writeJSONError(w, http.StatusInternalServerError, "encode observations SWE CSV response")
			return
		}
	}
	w.Header().Set("Content-Type", string(MediaSWECsv))
	if !schemaBacked {
		w.Header().Set("X-CS-SWE-Subset", "observation-values")
	}
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	if _, err := w.Write(body); err != nil {
		c.errs.Add(1)
		c.logger.Error("write observations SWE CSV response", "err", err)
	}
}

func (c *Component) writeObservationsSWEBinary(w http.ResponseWriter, r *http.Request, datastreamID string, items []json.RawMessage) {
	schema, rows, schemaBacked := c.sweRowsForDatastream(r, datastreamID, items)
	var body []byte
	if r.Method != http.MethodHead {
		var err error
		body, err = swecommon.MarshalBinaryRows(schema, rows, swecommon.DefaultBinaryEncoding())
		if err != nil {
			c.errs.Add(1)
			c.logger.Error("encode observations SWE binary response", "err", err)
			writeJSONError(w, http.StatusInternalServerError, "encode observations SWE binary response")
			return
		}
	}
	w.Header().Set("Content-Type", string(MediaSWEBinary))
	if !schemaBacked {
		w.Header().Set("X-CS-SWE-Subset", "observation-values")
	}
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	if _, err := w.Write(body); err != nil {
		c.errs.Add(1)
		c.logger.Error("write observations SWE binary response", "err", err)
	}
}

func (c *Component) sweRowsForDatastream(
	r *http.Request,
	datastreamID string,
	items []json.RawMessage,
) (*swecommon.DataRecord, []swecommon.Values, bool) {
	if schema, ok := c.fetchDatastreamObservationSchema(r, datastreamID); ok {
		return schema, sweRowsFromOMSWithSchema(items, schema), true
	}
	schema, rows := sweRowsFromOMS(items)
	return schema, rows, false
}

func sweRowsFromOMS(items []json.RawMessage) (*swecommon.DataRecord, []swecommon.Values) {
	raw := make([]map[string]any, 0, len(items))
	var resultKind swecommon.ComponentKind
	for _, item := range items {
		var obj map[string]any
		if err := json.Unmarshal(item, &obj); err != nil {
			continue
		}
		raw = append(raw, obj)
		resultKind = widenSWEKind(resultKind, sweKindForValue(obj["result"]))
	}
	if resultKind == "" {
		resultKind = swecommon.KindQuantity
	}

	schema := sweObservationSchema(resultKind)
	rows := make([]swecommon.Values, 0, len(raw))
	for _, obj := range raw {
		var timeValue any
		if t := firstStringValue(obj, "phenomenonTime", "resultTime", "time"); t != "" {
			timeValue = t
		}
		rows = append(rows, swecommon.Values{
			"time":   timeValue,
			"result": sweValueForKind(obj["result"], resultKind),
		})
	}
	return schema, rows
}

func sweRowsFromOMSWithSchema(items []json.RawMessage, schema *swecommon.DataRecord) []swecommon.Values {
	rows := make([]swecommon.Values, 0, len(items))
	for _, item := range items {
		var obj map[string]any
		if err := json.Unmarshal(item, &obj); err != nil {
			continue
		}
		row := make(swecommon.Values, len(schema.Fields))
		result := obj["result"]
		resultObj, _ := result.(map[string]any)
		for _, field := range schema.Fields {
			row[field.Name] = sweSchemaFieldValue(obj, result, resultObj, field, len(schema.Fields))
		}
		rows = append(rows, row)
	}
	return rows
}

func sweSchemaFieldValue(
	obs map[string]any,
	result any,
	resultObj map[string]any,
	field swecommon.Field,
	fieldCount int,
) any {
	var v any
	switch field.Name {
	case "time":
		if t := firstStringValue(obs, "phenomenonTime", "resultTime", "time"); t != "" {
			v = t
		}
	case "result":
		v = result
	default:
		if resultObj != nil {
			v = resultObj[field.Name]
		} else if fieldCount == 1 {
			v = result
		}
	}
	if field.Component.Kind() == swecommon.KindText {
		return sweValueForKind(v, swecommon.KindText)
	}
	return v
}

func sweObservationSchema(resultKind swecommon.ComponentKind) *swecommon.DataRecord {
	return &swecommon.DataRecord{
		Fields: []swecommon.Field{
			{
				Name: "time",
				Component: swecommon.Time{CommonFields: swecommon.CommonFields{
					LabelValue:      "Observation time",
					DefinitionValue: "http://www.opengis.net/def/property/OGC/0/SamplingTime",
				}},
			},
			{
				Name:      "result",
				Component: sweComponentForKind(resultKind),
			},
		},
	}
}

func sweComponentForKind(kind swecommon.ComponentKind) swecommon.DataComponent {
	switch kind {
	case swecommon.KindBoolean:
		return swecommon.Boolean{CommonFields: swecommon.CommonFields{LabelValue: "Result"}}
	case swecommon.KindText:
		return swecommon.Text{CommonFields: swecommon.CommonFields{LabelValue: "Result"}}
	default:
		return swecommon.Quantity{CommonFields: swecommon.CommonFields{LabelValue: "Result"}}
	}
}

func sweKindForValue(v any) swecommon.ComponentKind {
	switch v.(type) {
	case nil:
		return ""
	case bool:
		return swecommon.KindBoolean
	case float64:
		return swecommon.KindQuantity
	case string:
		return swecommon.KindText
	default:
		return swecommon.KindText
	}
}

func widenSWEKind(current, next swecommon.ComponentKind) swecommon.ComponentKind {
	if next == "" {
		return current
	}
	if current == "" {
		return next
	}
	if current == next {
		return current
	}
	return swecommon.KindText
}

func sweValueForKind(v any, kind swecommon.ComponentKind) any {
	if v == nil {
		return nil
	}
	switch kind {
	case swecommon.KindText:
		return sweTextValue(v)
	default:
		return v
	}
}

func sweTextValue(v any) string {
	switch t := v.(type) {
	case string:
		return t
	default:
		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(t); err != nil {
			return ""
		}
		return string(bytes.TrimSpace(buf.Bytes()))
	}
}

func firstStringValue(obj map[string]any, keys ...string) string {
	for _, key := range keys {
		if v, ok := obj[key].(string); ok {
			return v
		}
	}
	return ""
}

func (c *Component) observationsSelfLink(datastreamID, rawQuery string) string {
	base := "/datastreams/" + datastreamID + "/observations"
	if rawQuery != "" {
		return base + "?" + rawQuery
	}
	return base
}

func observationsCollectionSelfLink(rawQuery string) string {
	if rawQuery == "" {
		return "/observations"
	}
	return "/observations?" + rawQuery
}

func parseObservationAfter(raw string) (uint64, error) {
	if raw == "" {
		return 0, nil
	}
	parsed, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0, errors.New("after must be a positive integer sequence")
	}
	return parsed, nil
}

func (c *Component) observationResourcesFromMessages(msgs []observationMsg, fallbackDatastreamID string) []json.RawMessage {
	items := make([]json.RawMessage, 0, len(msgs))
	for _, msg := range msgs {
		datastreamID := fallbackDatastreamID
		if datastreamID == "" {
			datastreamID = datastreamIDFromObservationSubject(c.cfg.ObservationsSubjectPrefix, msg.Subject)
		}
		resource, ok := observationResourceFromPayload(msg.Data, datastreamID)
		if ok {
			items = append(items, resource)
		}
	}
	return items
}

func observationResourcesFromPayloads(payloads []json.RawMessage, datastreamID string) []json.RawMessage {
	items := make([]json.RawMessage, 0, len(payloads))
	for _, payload := range payloads {
		resource, ok := observationResourceFromPayload(payload, datastreamID)
		if ok {
			items = append(items, resource)
		}
	}
	return items
}

func observationResourceFromPayload(payload json.RawMessage, datastreamID string) (json.RawMessage, bool) {
	if len(payload) == 0 {
		return nil, false
	}
	var obj map[string]any
	if err := json.Unmarshal(payload, &obj); err != nil {
		return nil, false
	}
	if datastreamID != "" {
		obj["datastream@id"] = datastreamID
	}
	if _, ok := obj["phenomenonTime"]; !ok {
		if resultTime, ok := obj["resultTime"]; ok {
			obj["phenomenonTime"] = resultTime
		}
	}
	out, err := json.Marshal(obj)
	if err != nil {
		return nil, false
	}
	return out, true
}

type observationCollectionFilters struct {
	phenomenonTime string
	resultTime     string
}

func observationCollectionFiltersFromQuery(query url.Values) observationCollectionFilters {
	return observationCollectionFilters{
		phenomenonTime: queryString(query, "phenomenonTime"),
		resultTime:     queryString(query, "resultTime"),
	}
}

func (f observationCollectionFilters) active() bool {
	return f.phenomenonTime != "" || f.resultTime != ""
}

func filterObservationResources(items []json.RawMessage, filters observationCollectionFilters, limit int) []json.RawMessage {
	if !filters.active() {
		if len(items) > limit {
			return items[:limit]
		}
		return items
	}
	filtered := make([]json.RawMessage, 0, len(items))
	for _, item := range items {
		if observationResourceMatchesFilters(item, filters) {
			filtered = append(filtered, item)
			if len(filtered) == limit {
				break
			}
		}
	}
	return filtered
}

func observationResourceMatchesFilters(item json.RawMessage, filters observationCollectionFilters) bool {
	if filters.phenomenonTime != "" && !resourceTimeIntersects(jsonResourceString(item, "phenomenonTime"), filters.phenomenonTime) {
		return false
	}
	if filters.resultTime != "" && !resourceTimeIntersects(jsonResourceString(item, "resultTime"), filters.resultTime) {
		return false
	}
	return true
}

func observationResourceID(resource json.RawMessage) string {
	var obj struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(resource, &obj); err != nil {
		return ""
	}
	return obj.ID
}

func datastreamIDFromObservationSubject(prefix, subject string) string {
	want := prefix + "."
	if !strings.HasPrefix(subject, want) {
		return ""
	}
	return strings.TrimPrefix(subject, want)
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
