package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	defaultBaseURL         = "http://semconnect:8080"
	defaultNATSURL         = "http://nats:8222"
	systemFixturePath      = "/fixtures/canonical-system.v1.json"
	datastreamFixturePath  = "/fixtures/canonical-datastream.v1.json"
	observationFixturePath = "/fixtures/canonical-observation.v1.json"
	expectedSystemID       = "c360.semconnect.systems.csapi.system.v1"
	expectedSystemUID      = "urn:c360:semconnect:deployment-smoke:system:v1"
	expectedDatastreamID   = "c360.semconnect.systems.csapi.datastream.v1"
	expectedObservationID  = "deployment-smoke-observation-v1"
	probeLimit             = 45 * time.Second
	retryInterval          = 500 * time.Millisecond
)

type systemCollection struct {
	NumberMatched  int         `json:"numberMatched"`
	NumberReturned int         `json:"numberReturned"`
	Items          []systemRef `json:"items"`
}

type systemRef struct {
	ID string `json:"id"`
}

type systemResource struct {
	ID         string `json:"id"`
	UID        string `json:"uid"`
	Properties struct {
		UID string `json:"uid"`
	} `json:"properties"`
}

type result struct {
	SystemID                string `json:"systemId"`
	SystemSHA256            string `json:"systemSha256"`
	SystemNumberMatched     int    `json:"systemNumberMatched"`
	SystemNumberReturned    int    `json:"systemNumberReturned"`
	DatastreamID            string `json:"datastreamId"`
	DatastreamSHA256        string `json:"datastreamSha256"`
	SchemaSHA256            string `json:"schemaSha256"`
	ObservationID           string `json:"observationId"`
	GlobalObservationSHA256 string `json:"globalObservationSha256"`
	ScopedObservationSHA256 string `json:"scopedObservationSha256"`
	GlobalNumberReturned    int    `json:"globalNumberReturned"`
	ScopedNumberReturned    int    `json:"scopedNumberReturned"`
}

type datastreamResource struct {
	ID       string `json:"id"`
	SystemID string `json:"system@id"`
}

type observationCollection struct {
	NumberMatched  int               `json:"numberMatched"`
	NumberReturned int               `json:"numberReturned"`
	Items          []json.RawMessage `json:"items"`
}

type observationResource struct {
	ID           string `json:"id"`
	DatastreamID string `json:"datastream@id"`
}

type jetStreamResponse struct {
	ServerID       *string          `json:"server_id"`
	Accounts       *int             `json:"accounts"`
	Streams        *int             `json:"streams"`
	Consumers      *int             `json:"consumers"`
	Messages       *int             `json:"messages"`
	Bytes          *int             `json:"bytes"`
	AccountDetails *[]accountDetail `json:"account_details"`
}

type accountDetail struct {
	Name string `json:"name"`
	ID   string `json:"id"`
}

type jetStreamProof struct {
	ServerID  string `json:"server_id"`
	Account   string `json:"account"`
	Domain    string `json:"domain"`
	Streams   int    `json:"streams"`
	Consumers int    `json:"consumers"`
	Messages  int    `json:"messages"`
	Bytes     int    `json:"bytes"`
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) != 1 || (args[0] != "preflight" && args[0] != "seed" && args[0] != "verify-only") {
		return errors.New("usage: canonical-smoke preflight|seed|verify-only")
	}
	client := &http.Client{Timeout: 5 * time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), probeLimit)
	defer cancel()
	if args[0] == "preflight" {
		return preflight(ctx, client)
	}
	baseURL := strings.TrimRight(os.Getenv("SEMCONNECT_URL"), "/")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	if err := waitHealthy(ctx, client, baseURL); err != nil {
		return err
	}
	if args[0] == "seed" {
		if err := seed(ctx, client, baseURL); err != nil {
			return err
		}
	}

	proof, err := waitForProof(ctx, client, baseURL)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(proof)
	if err != nil {
		return fmt.Errorf("encode proof: %w", err)
	}
	fmt.Println(string(encoded))
	return nil
}

func preflight(ctx context.Context, client *http.Client) error {
	natsURL := strings.TrimRight(os.Getenv("NATS_MONITOR_URL"), "/")
	if natsURL == "" {
		natsURL = defaultNATSURL
	}
	resp, err := do(ctx, client, http.MethodGet, natsURL+"/jsz?accounts=true", nil, "")
	if err != nil {
		return fmt.Errorf("read JetStream state: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("JetStream status %d", resp.StatusCode)
	}
	var state jetStreamResponse
	if err := json.NewDecoder(resp.Body).Decode(&state); err != nil {
		return fmt.Errorf("decode JetStream state: %w", err)
	}
	proof, err := validateGreenfieldState(state)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(proof)
	if err != nil {
		return fmt.Errorf("encode JetStream proof: %w", err)
	}
	fmt.Println(string(encoded))
	return nil
}

func validateGreenfieldState(state jetStreamResponse) (jetStreamProof, error) {
	if state.ServerID == nil || strings.TrimSpace(*state.ServerID) == "" {
		return jetStreamProof{}, errors.New("JetStream server_id is absent or empty")
	}
	if state.Accounts == nil {
		return jetStreamProof{}, errors.New("JetStream accounts count is absent")
	}
	if *state.Accounts != 1 {
		return jetStreamProof{}, fmt.Errorf("JetStream account count %d, want 1", *state.Accounts)
	}
	if state.AccountDetails == nil {
		return jetStreamProof{}, errors.New("JetStream account_details is absent")
	}
	if len(*state.AccountDetails) != 1 || (*state.AccountDetails)[0].Name != "$G" ||
		(*state.AccountDetails)[0].ID != "$G" {
		return jetStreamProof{}, fmt.Errorf("JetStream account identity is not exactly $G: %+v", *state.AccountDetails)
	}
	for name, value := range map[string]*int{
		"streams": state.Streams, "consumers": state.Consumers, "messages": state.Messages, "bytes": state.Bytes,
	} {
		if value == nil {
			return jetStreamProof{}, fmt.Errorf("JetStream %s count is absent", name)
		}
	}
	if *state.Streams != 0 || *state.Consumers != 0 || *state.Messages != 0 || *state.Bytes != 0 {
		return jetStreamProof{}, fmt.Errorf(
			"NATS is not clean: streams=%d consumers=%d messages=%d bytes=%d",
			*state.Streams,
			*state.Consumers,
			*state.Messages,
			*state.Bytes,
		)
	}
	return jetStreamProof{
		ServerID:  *state.ServerID,
		Account:   "$G",
		Domain:    "default (unset)",
		Streams:   *state.Streams,
		Consumers: *state.Consumers,
		Messages:  *state.Messages,
		Bytes:     *state.Bytes,
	}, nil
}

func waitHealthy(ctx context.Context, client *http.Client, baseURL string) error {
	return retry(ctx, func() error {
		resp, err := do(ctx, client, http.MethodGet, baseURL+"/health", nil, "")
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("health status %d", resp.StatusCode)
		}
		return nil
	})
}

func seed(ctx context.Context, client *http.Client, baseURL string) error {
	systemBody, err := os.ReadFile(systemFixturePath)
	if err != nil {
		return fmt.Errorf("read canonical System fixture: %w", err)
	}
	datastreamBody, err := os.ReadFile(datastreamFixturePath)
	if err != nil {
		return fmt.Errorf("read canonical Datastream fixture: %w", err)
	}
	observationBody, err := os.ReadFile(observationFixturePath)
	if err != nil {
		return fmt.Errorf("read canonical Observation fixture: %w", err)
	}
	return seedFixtures(ctx, client, baseURL, systemBody, datastreamBody, observationBody)
}

func seedFixtures(ctx context.Context, client *http.Client, baseURL string, systemBody, datastreamBody, observationBody []byte) error {
	resp, err := do(ctx, client, http.MethodPost, baseURL+"/systems", systemBody, "application/geo+json")
	if err != nil {
		return fmt.Errorf("seed canonical system: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		responseBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("seed canonical system: status %d: %s", resp.StatusCode, strings.TrimSpace(string(responseBody)))
	}
	if got := resp.Header.Get("Location"); got != "/systems/"+expectedSystemID {
		return fmt.Errorf("seed System Location %q, want /systems/%s", got, expectedSystemID)
	}

	resp, err = do(ctx, client, http.MethodPost, baseURL+"/datastreams", datastreamBody, "application/json")
	if err != nil {
		return fmt.Errorf("seed canonical Datastream: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		responseBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("seed canonical Datastream: status %d: %s", resp.StatusCode, strings.TrimSpace(string(responseBody)))
	}
	if got := resp.Header.Get("Location"); got != "/datastreams/"+expectedDatastreamID {
		return fmt.Errorf("seed Datastream Location %q, want /datastreams/%s", got, expectedDatastreamID)
	}

	observationPath := "/datastreams/" + expectedDatastreamID + "/observations"
	resp, err = do(ctx, client, http.MethodPost, baseURL+observationPath, observationBody, "application/om+json")
	if err != nil {
		return fmt.Errorf("seed canonical Observation: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		responseBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("seed canonical Observation: status %d: %s", resp.StatusCode, strings.TrimSpace(string(responseBody)))
	}
	wantLocation := observationPath + "/" + expectedObservationID
	if got := resp.Header.Get("Location"); got != wantLocation {
		return fmt.Errorf("seed Observation Location %q, want %s", got, wantLocation)
	}
	return nil
}

func waitForProof(ctx context.Context, client *http.Client, baseURL string) (result, error) {
	var proof result
	err := retry(ctx, func() error {
		collection, err := getCollection(ctx, client, baseURL)
		if err != nil {
			return err
		}
		if collection.NumberMatched != 1 || collection.NumberReturned != 1 || len(collection.Items) != 1 {
			return fmt.Errorf("system counts not ready: matched=%d returned=%d items=%d",
				collection.NumberMatched, collection.NumberReturned, len(collection.Items))
		}
		if collection.Items[0].ID != expectedSystemID {
			return fmt.Errorf("system id %q, want %q", collection.Items[0].ID, expectedSystemID)
		}

		itemBody, item, err := getItem(ctx, client, baseURL)
		if err != nil {
			return err
		}
		if item.ID != expectedSystemID || item.UID != expectedSystemUID || item.Properties.UID != expectedSystemUID {
			return fmt.Errorf("canonical item mismatch: id=%q uid=%q properties.uid=%q",
				item.ID, item.UID, item.Properties.UID)
		}
		datastreamBody, err := getCanonicalJSON(ctx, client, baseURL+"/datastreams/"+expectedDatastreamID)
		if err != nil {
			return err
		}
		var datastream datastreamResource
		if err := json.Unmarshal(datastreamBody, &datastream); err != nil {
			return fmt.Errorf("decode Datastream: %w", err)
		}
		if datastream.ID != expectedDatastreamID || datastream.SystemID != expectedSystemID {
			return fmt.Errorf("canonical Datastream mismatch: id=%q system@id=%q", datastream.ID, datastream.SystemID)
		}
		schemaBody, err := getCanonicalJSON(ctx, client, baseURL+"/datastreams/"+expectedDatastreamID+"/schema")
		if err != nil {
			return err
		}
		global, err := getObservationCollection(ctx, client, baseURL+"/observations?limit=10")
		if err != nil {
			return err
		}
		scoped, err := getObservationCollection(ctx, client,
			baseURL+"/datastreams/"+expectedDatastreamID+"/observations?limit=10")
		if err != nil {
			return err
		}
		globalItem, err := requireCanonicalObservation(global)
		if err != nil {
			return fmt.Errorf("global observations: %w", err)
		}
		scopedItem, err := requireCanonicalObservation(scoped)
		if err != nil {
			return fmt.Errorf("scoped observations: %w", err)
		}
		proof = result{
			SystemID: expectedSystemID, SystemSHA256: digestBytes(itemBody),
			SystemNumberMatched: collection.NumberMatched, SystemNumberReturned: collection.NumberReturned,
			DatastreamID: expectedDatastreamID, DatastreamSHA256: digestBytes(datastreamBody),
			SchemaSHA256: digestBytes(schemaBody), ObservationID: expectedObservationID,
			GlobalObservationSHA256: digestBytes(globalItem), ScopedObservationSHA256: digestBytes(scopedItem),
			GlobalNumberReturned: global.NumberReturned, ScopedNumberReturned: scoped.NumberReturned,
		}
		return nil
	})
	return proof, err
}

func getCollection(ctx context.Context, client *http.Client, baseURL string) (systemCollection, error) {
	var collection systemCollection
	resp, err := do(ctx, client, http.MethodGet, baseURL+"/systems", nil, "")
	if err != nil {
		return collection, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return collection, fmt.Errorf("systems status %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(&collection); err != nil {
		return collection, fmt.Errorf("decode systems: %w", err)
	}
	return collection, nil
}

func getItem(ctx context.Context, client *http.Client, baseURL string) ([]byte, systemResource, error) {
	var item systemResource
	resp, err := do(ctx, client, http.MethodGet, baseURL+"/systems/"+expectedSystemID, nil, "")
	if err != nil {
		return nil, item, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, item, fmt.Errorf("system item status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, item, fmt.Errorf("read system item: %w", err)
	}
	var normalized any
	if err := json.Unmarshal(body, &normalized); err != nil {
		return nil, item, fmt.Errorf("decode system item: %w", err)
	}
	canonical, err := json.Marshal(normalized)
	if err != nil {
		return nil, item, fmt.Errorf("normalize system item: %w", err)
	}
	if err := json.Unmarshal(canonical, &item); err != nil {
		return nil, item, fmt.Errorf("decode normalized system item: %w", err)
	}
	return canonical, item, nil
}

func getCanonicalJSON(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	resp, err := do(ctx, client, http.MethodGet, url, nil, "")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("GET %s status %d: %s", url, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var normalized any
	if err := json.NewDecoder(resp.Body).Decode(&normalized); err != nil {
		return nil, fmt.Errorf("decode GET %s: %w", url, err)
	}
	canonical, err := json.Marshal(normalized)
	if err != nil {
		return nil, fmt.Errorf("normalize GET %s: %w", url, err)
	}
	return canonical, nil
}

func getObservationCollection(ctx context.Context, client *http.Client, url string) (observationCollection, error) {
	body, err := getCanonicalJSON(ctx, client, url)
	if err != nil {
		return observationCollection{}, err
	}
	var collection observationCollection
	if err := json.Unmarshal(body, &collection); err != nil {
		return collection, fmt.Errorf("decode ObservationCollection: %w", err)
	}
	return collection, nil
}

func requireCanonicalObservation(collection observationCollection) ([]byte, error) {
	if collection.NumberMatched != 1 || collection.NumberReturned != 1 || len(collection.Items) != 1 {
		return nil, fmt.Errorf("counts not ready: matched=%d returned=%d items=%d",
			collection.NumberMatched, collection.NumberReturned, len(collection.Items))
	}
	var observation observationResource
	if err := json.Unmarshal(collection.Items[0], &observation); err != nil {
		return nil, fmt.Errorf("decode canonical Observation: %w", err)
	}
	if observation.ID != expectedObservationID || observation.DatastreamID != expectedDatastreamID {
		return nil, fmt.Errorf("canonical Observation mismatch: id=%q datastream@id=%q",
			observation.ID, observation.DatastreamID)
	}
	var normalized any
	if err := json.Unmarshal(collection.Items[0], &normalized); err != nil {
		return nil, err
	}
	return json.Marshal(normalized)
}

func digestBytes(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func do(
	ctx context.Context,
	client *http.Client,
	method string,
	url string,
	body []byte,
	contentType string,
) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	return client.Do(req)
}

func retry(ctx context.Context, operation func() error) error {
	var lastErr error
	ticker := time.NewTicker(retryInterval)
	defer ticker.Stop()
	for {
		if err := operation(); err == nil {
			return nil
		} else {
			lastErr = err
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("probe deadline: %w: last error: %v", ctx.Err(), lastErr)
		case <-ticker.C:
		}
	}
}
