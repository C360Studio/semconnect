package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestSeedFixturesCreatesPersistenceChainInOrder(t *testing.T) {
	t.Parallel()
	type request struct {
		method      string
		path        string
		contentType string
		body        string
	}
	var got []request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request: %v", err)
		}
		got = append(got, request{r.Method, r.URL.Path, r.Header.Get("Content-Type"), string(body)})
		switch r.URL.Path {
		case "/systems":
			w.Header().Set("Location", "/systems/"+expectedSystemID)
		case "/datastreams":
			w.Header().Set("Location", "/datastreams/"+expectedDatastreamID)
		case "/datastreams/" + expectedDatastreamID + "/observations":
			w.Header().Set("Location", r.URL.Path+"/"+expectedObservationID)
		default:
			http.Error(w, "unexpected path", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := seedFixtures(ctx, server.Client(), server.URL, []byte("system"), []byte("datastream"), []byte("observation")); err != nil {
		t.Fatalf("seedFixtures() error = %v", err)
	}
	want := []request{
		{http.MethodPost, "/systems", "application/geo+json", "system"},
		{http.MethodPost, "/datastreams", "application/json", "datastream"},
		{http.MethodPost, "/datastreams/" + expectedDatastreamID + "/observations", "application/om+json", "observation"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("requests = %#v, want %#v", got, want)
	}
}

func TestWaitForProofCoversSystemSchemaAndObservationReads(t *testing.T) {
	t.Parallel()
	observation := `{"id":"` + expectedObservationID + `","datastream@id":"` + expectedDatastreamID + `","result":21.5}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.RequestURI() {
		case "/systems":
			_, _ = io.WriteString(w, `{"numberMatched":1,"numberReturned":1,"items":[{"id":"`+expectedSystemID+`"}]}`)
		case "/systems/" + expectedSystemID:
			_, _ = io.WriteString(w, `{"id":"`+expectedSystemID+`","uid":"`+expectedSystemUID+`","properties":{"uid":"`+expectedSystemUID+`"}}`)
		case "/datastreams/" + expectedDatastreamID:
			_, _ = io.WriteString(w, `{"id":"`+expectedDatastreamID+`","system@id":"`+expectedSystemID+`"}`)
		case "/datastreams/" + expectedDatastreamID + "/schema":
			_, _ = io.WriteString(w, `{"obsFormat":"application/json","resultSchema":{"type":"DataRecord","fields":[{"name":"temperature","type":"Quantity"}]}}`)
		case "/observations?limit=10", "/datastreams/" + expectedDatastreamID + "/observations?limit=10":
			_, _ = io.WriteString(w, `{"numberMatched":1,"numberReturned":1,"items":[`+observation+`]}`)
		default:
			http.Error(w, "unexpected URI: "+r.URL.RequestURI(), http.StatusNotFound)
		}
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	proof, err := waitForProof(ctx, server.Client(), server.URL)
	if err != nil {
		t.Fatalf("waitForProof() error = %v", err)
	}
	if proof.SystemID != expectedSystemID || proof.DatastreamID != expectedDatastreamID || proof.ObservationID != expectedObservationID {
		t.Fatalf("proof identities = %#v", proof)
	}
	if proof.SystemSHA256 == "" || proof.DatastreamSHA256 == "" || proof.SchemaSHA256 == "" {
		t.Fatalf("proof lacks entity/schema digests: %#v", proof)
	}
	if proof.GlobalObservationSHA256 == "" || proof.GlobalObservationSHA256 != proof.ScopedObservationSHA256 {
		t.Fatalf("global/scoped observation proof differs: %#v", proof)
	}
	if proof.SystemNumberMatched != 1 || proof.SystemNumberReturned != 1 ||
		proof.GlobalNumberReturned != 1 || proof.ScopedNumberReturned != 1 {
		t.Fatalf("proof counts = %#v", proof)
	}
}

func TestRequireCanonicalObservationRejectsSystemOnlyProof(t *testing.T) {
	t.Parallel()
	_, err := requireCanonicalObservation(observationCollection{
		NumberMatched:  0,
		NumberReturned: 0,
	})
	if err == nil || !strings.Contains(err.Error(), "counts not ready") {
		t.Fatalf("requireCanonicalObservation() error = %v", err)
	}
}

func TestValidateGreenfieldState(t *testing.T) {
	t.Parallel()
	zero := 0
	one := 1
	two := 2
	ten := 10
	serverID := "NTEST"
	globalAccount := []accountDetail{{Name: "$G", ID: "$G"}}
	for _, tc := range []struct {
		name    string
		state   jetStreamResponse
		wantErr string
	}{
		{
			name: "clean",
			state: jetStreamResponse{
				ServerID: &serverID, Accounts: &one, Streams: &zero, Consumers: &zero,
				Messages: &zero, Bytes: &zero, AccountDetails: &globalAccount,
			},
		},
		{
			name:    "ten streams is not mistaken for zero",
			state:   validState(&serverID, &one, &ten, &zero, &globalAccount),
			wantErr: "streams=10",
		},
		{
			name:    "messages without streams fail closed",
			state:   validState(&serverID, &one, nil, &zero, &globalAccount),
			wantErr: "streams count is absent",
		},
		{
			name:    "unexpected account count",
			state:   validState(&serverID, &two, &zero, &zero, &globalAccount),
			wantErr: "account count 2",
		},
		{name: "missing server id", state: validState(nil, &one, &zero, &zero, &globalAccount), wantErr: "server_id"},
		{name: "missing account details", state: validState(&serverID, &one, &zero, &zero, nil), wantErr: "account_details"},
		{
			name: "wrong account identity",
			state: validState(
				&serverID, &one, &zero, &zero, &[]accountDetail{{Name: "APP", ID: "APP"}},
			),
			wantErr: "not exactly $G",
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := validateGreenfieldState(tc.state)
			if tc.wantErr == "" && err != nil {
				t.Fatalf("validateGreenfieldState() error = %v", err)
			}
			if tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
				t.Fatalf("validateGreenfieldState() error = %v, want substring %q", err, tc.wantErr)
			}
		})
	}
}

func validState(
	serverID *string,
	accounts *int,
	streams *int,
	zero *int,
	accountDetails *[]accountDetail,
) jetStreamResponse {
	return jetStreamResponse{
		ServerID: serverID, Accounts: accounts, Streams: streams, Consumers: zero,
		Messages: zero, Bytes: zero, AccountDetails: accountDetails,
	}
}

func TestValidateGreenfieldStateRequiresEveryCount(t *testing.T) {
	t.Parallel()
	zero := 0
	one := 1
	serverID := "NTEST"
	globalAccount := []accountDetail{{Name: "$G", ID: "$G"}}
	for _, field := range []string{"accounts", "streams", "consumers", "messages", "bytes"} {
		field := field
		t.Run(field, func(t *testing.T) {
			t.Parallel()
			state := jetStreamResponse{
				ServerID: &serverID, Accounts: &one, Streams: &zero, Consumers: &zero,
				Messages: &zero, Bytes: &zero, AccountDetails: &globalAccount,
			}
			switch field {
			case "accounts":
				state.Accounts = nil
			case "streams":
				state.Streams = nil
			case "consumers":
				state.Consumers = nil
			case "messages":
				state.Messages = nil
			case "bytes":
				state.Bytes = nil
			}
			_, err := validateGreenfieldState(state)
			if err == nil || !strings.Contains(err.Error(), field) {
				t.Fatalf("validateGreenfieldState() error = %v, want missing %s", err, field)
			}
		})
	}
}
