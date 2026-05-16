package csapi

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
)

// MediaType is a CS API response encoding the server can produce.
type MediaType string

const (
	MediaJSON      MediaType = "application/json"
	MediaJSONLD    MediaType = "application/ld+json"
	MediaGeoJSON   MediaType = "application/geo+json"
	MediaSensorML  MediaType = "application/sensorml+json"
	MediaOMS       MediaType = "application/om+json"
)

// ResourceFamily groups endpoints that share the same negotiable encoding set.
type ResourceFamily int

const (
	FamilySystem ResourceFamily = iota
	FamilyObservation
	FamilySpatial
	FamilyService // /, /api, /conformance
)

// supported returns the negotiable encodings for fam, in preference order.
// The first entry is the default when the client did not constrain Accept.
//
// Stage 2 only ships the JSON encoder. ADR-S001 §1 claims SensorML / GeoJSON /
// OMS / JSON-LD conformance classes at the v0.1 *release* — those rows expand
// as their encoders land per stage (Stage 3 OMS for observations, Stage 4
// SensorML for /systems/{id}, Stage 5 GeoJSON for /areas, JSON-LD any time
// after vocabulary/export is wired). Until then, Negotiate honestly 406s on
// types we cannot serialise rather than promising bytes we can't produce.
func (fam ResourceFamily) supported() []MediaType {
	switch fam {
	case FamilySystem:
		return []MediaType{MediaJSON}
	case FamilyObservation:
		return []MediaType{MediaJSON}
	case FamilySpatial:
		return []MediaType{MediaJSON}
	case FamilyService:
		return []MediaType{MediaJSON}
	default:
		return []MediaType{MediaJSON}
	}
}

// Negotiate picks the response MediaType for an Accept header against fam's
// supported set. Returns (chosen, true) on success, ("", false) on 406 — the
// caller writes the 406 response (with the supported set in its body) so this
// function stays pure.
//
// Rules:
//   - Empty / missing Accept → fam's default (first supported entry).
//   - Wildcards (`*/*`, `application/*`) resolve to the highest-q supported.
//   - Exact matches win over wildcards at the same q.
//   - Ties at equal q break in fam's supported-list order.
func Negotiate(accept string, fam ResourceFamily) (MediaType, bool) {
	supported := fam.supported()
	accept = strings.TrimSpace(accept)
	if accept == "" {
		return supported[0], true
	}

	type offer struct {
		media MediaType
		q     float64
		idx   int // index in supported — tiebreaker
	}
	var offers []offer

	parts := parseAccept(accept)
	for i, m := range supported {
		bestQ := -1.0
		for _, p := range parts {
			if !p.matches(string(m)) {
				continue
			}
			if p.q > bestQ {
				bestQ = p.q
			}
		}
		if bestQ > 0 {
			offers = append(offers, offer{m, bestQ, i})
		}
	}
	if len(offers) == 0 {
		return "", false
	}
	sort.SliceStable(offers, func(i, j int) bool {
		if offers[i].q != offers[j].q {
			return offers[i].q > offers[j].q
		}
		return offers[i].idx < offers[j].idx
	})
	return offers[0].media, true
}

// SupportedMedia returns the supported set for fam — for use in 406 bodies.
func SupportedMedia(fam ResourceFamily) []string {
	supported := fam.supported()
	out := make([]string, len(supported))
	for i, m := range supported {
		out[i] = string(m)
	}
	return out
}

// WriteNotAcceptable writes a 406 with the supported encoding list.
func WriteNotAcceptable(w http.ResponseWriter, fam ResourceFamily) {
	w.Header().Set("Content-Type", string(MediaJSON))
	w.WriteHeader(http.StatusNotAcceptable)
	supported := SupportedMedia(fam)
	body := `{"error":"not acceptable","supported":["` +
		strings.Join(supported, `","`) + `"]}`
	_, _ = w.Write([]byte(body))
}

type acceptPart struct {
	typ, sub string
	q        float64
}

func (p acceptPart) matches(media string) bool {
	slash := strings.IndexByte(media, '/')
	if slash < 0 {
		return false
	}
	mt, ms := media[:slash], media[slash+1:]
	if p.typ != "*" && p.typ != mt {
		return false
	}
	if p.sub != "*" && p.sub != ms {
		return false
	}
	return true
}

func parseAccept(h string) []acceptPart {
	var out []acceptPart
	for _, raw := range strings.Split(h, ",") {
		seg := strings.TrimSpace(raw)
		if seg == "" {
			continue
		}
		fields := strings.Split(seg, ";")
		mediaSeg := strings.TrimSpace(fields[0])
		slash := strings.IndexByte(mediaSeg, '/')
		if slash < 0 {
			continue
		}
		p := acceptPart{
			typ: strings.ToLower(mediaSeg[:slash]),
			sub: strings.ToLower(mediaSeg[slash+1:]),
			q:   1.0,
		}
		for _, param := range fields[1:] {
			param = strings.TrimSpace(param)
			if !strings.HasPrefix(param, "q=") {
				continue
			}
			if q, err := strconv.ParseFloat(strings.TrimPrefix(param, "q="), 64); err == nil {
				p.q = q
			}
		}
		out = append(out, p)
	}
	return out
}
