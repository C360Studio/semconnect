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
	defaultBaseURL = "http://semconnect:8080"
	defaultNATSURL = "http://nats:8222"
	fixturePath    = "/fixtures/canonical-system.v1.json"
	expectedID     = "c360.semconnect.systems.csapi.system.v1"
	expectedUID    = "urn:c360:semconnect:deployment-smoke:system:v1"
	probeLimit     = 45 * time.Second
	retryInterval  = 500 * time.Millisecond
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
	EntityID       string `json:"entityId"`
	ItemSHA256     string `json:"itemSha256"`
	NumberMatched  int    `json:"numberMatched"`
	NumberReturned int    `json:"numberReturned"`
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
	body, err := os.ReadFile(fixturePath)
	if err != nil {
		return fmt.Errorf("read canonical fixture: %w", err)
	}
	resp, err := do(ctx, client, http.MethodPost, baseURL+"/systems", body, "application/geo+json")
	if err != nil {
		return fmt.Errorf("seed canonical system: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		responseBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("seed canonical system: status %d: %s", resp.StatusCode, strings.TrimSpace(string(responseBody)))
	}
	if got := resp.Header.Get("Location"); got != "/systems/"+expectedID {
		return fmt.Errorf("seed Location %q, want /systems/%s", got, expectedID)
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
		if collection.Items[0].ID != expectedID {
			return fmt.Errorf("system id %q, want %q", collection.Items[0].ID, expectedID)
		}

		itemBody, item, err := getItem(ctx, client, baseURL)
		if err != nil {
			return err
		}
		if item.ID != expectedID || item.UID != expectedUID || item.Properties.UID != expectedUID {
			return fmt.Errorf("canonical item mismatch: id=%q uid=%q properties.uid=%q",
				item.ID, item.UID, item.Properties.UID)
		}
		sum := sha256.Sum256(itemBody)
		proof = result{
			EntityID:       expectedID,
			ItemSHA256:     hex.EncodeToString(sum[:]),
			NumberMatched:  collection.NumberMatched,
			NumberReturned: collection.NumberReturned,
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
	resp, err := do(ctx, client, http.MethodGet, baseURL+"/systems/"+expectedID, nil, "")
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
