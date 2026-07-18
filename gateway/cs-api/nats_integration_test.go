package csapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/c360studio/semconnect/message/oms"
	"github.com/c360studio/semconnect/parser/sensorml"
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/payloadbuiltins"
	"github.com/c360studio/semstreams/pkg/errs"
	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

func TestRealNATSEntityMutationCarriesCanonicalFinalState(t *testing.T) {
	server := startEmbeddedNATSServer(t, false)

	responder, err := nats.Connect(server.ClientURL())
	if err != nil {
		t.Fatalf("connect responder: %v", err)
	}
	t.Cleanup(responder.Close)

	observed := make(chan error, 1)
	emptyResponse := encodeBatchOK(t, 0)
	successResponse := encodeBatchOK(t, 2)
	_, err = responder.Subscribe(SubjectEntityCreateWithTriples, func(msg *nats.Msg) {
		var request graph.CreateEntityWithTriplesRequest
		if decodeErr := json.Unmarshal(msg.Data, &request); decodeErr != nil {
			observed <- fmt.Errorf("decode mutation request: %w", decodeErr)
			_ = msg.Respond(emptyResponse)
			return
		}
		if request.Entity == nil {
			observed <- fmt.Errorf("mutation request omitted entity")
		} else if validationErr := graph.ValidateEntityStateContract(request.Entity); validationErr != nil {
			observed <- fmt.Errorf("validate received entity: %w", validationErr)
		} else if msg.Header.Get("X-CS-Forwarded-User") != "alice" {
			observed <- fmt.Errorf("forwarded-user header: got %q", msg.Header.Get("X-CS-Forwarded-User"))
		} else {
			observed <- nil
		}
		_ = msg.Respond(successResponse)
	})
	if err != nil {
		t.Fatalf("subscribe mutation responder: %v", err)
	}

	entityID := "acme.ops.robotics.gcs.system.child"
	parentID := "acme.ops.robotics.gcs.system.parent"
	stateResponse, err := json.Marshal(graph.EntityState{
		ID: entityID,
		Triples: []message.Triple{{
			Subject: entityID, Predicate: sensorml.PredIsHostedBy,
			Object: parentID, Datatype: message.EntityReferenceDatatype,
		}},
	})
	if err != nil {
		t.Fatalf("marshal query response: %v", err)
	}
	_, err = responder.Subscribe(subjectEntityQuery, func(msg *nats.Msg) {
		var request struct {
			ID string `json:"id"`
		}
		if decodeErr := json.Unmarshal(msg.Data, &request); decodeErr != nil {
			_ = natsclient.RespondError(msg, errs.Classified(errs.ErrorInvalid, decodeErr))
			return
		}
		if request.ID != entityID {
			_ = natsclient.RespondError(msg, errs.ClassifiedCode(
				errs.ErrorInvalid,
				graph.ErrorCodeEntityNotFound,
				fmt.Errorf("not found: %s", request.ID),
			))
			return
		}
		_ = msg.Respond(stateResponse)
	})
	if err != nil {
		t.Fatalf("subscribe query responder: %v", err)
	}

	if err := responder.Flush(); err != nil {
		t.Fatalf("flush responder subscriptions: %v", err)
	}

	client := connectSemStreamsClient(t, server.ClientURL())

	config := DefaultConfig()
	config.QueryTimeout = 2 * time.Second
	component, err := New(config, client, nil)
	if err != nil {
		t.Fatalf("create component: %v", err)
	}
	triples := []message.Triple{
		{Subject: entityID, Predicate: sensorml.PredType, Object: "http://www.w3.org/ns/ssn/System"},
		{
			Subject: entityID, Predicate: sensorml.PredIsHostedBy,
			Object: parentID, Datatype: message.EntityReferenceDatatype,
		},
	}
	if err := component.ingestProjectedTriples(
		context.Background(), entityID, triples, systemProjectionMessageType,
		Identity{
			Subject: "subject-alice",
			Forwarded: map[string]string{
				"User":  "alice",
				"Email": "alice@example.test",
			},
		},
	); err != nil {
		t.Fatalf("real-NATS mutation: %v", err)
	}
	if err := <-observed; err != nil {
		t.Fatal(err)
	}

	state, err := component.fetchEntity(context.Background(), entityID)
	if err != nil {
		t.Fatalf("real-NATS query: %v", err)
	}
	if state.ID != entityID || len(state.Triples) != 1 || state.Triples[0].Datatype != message.EntityReferenceDatatype {
		t.Fatalf("queried entity lost canonical relationship state: %+v", state)
	}
	_, err = component.fetchEntity(context.Background(), "acme.ops.robotics.gcs.system.missing")
	if !errors.Is(err, errEntityNotFound) {
		t.Fatalf("classified not-found query: got %v want errEntityNotFound", err)
	}
}

func TestRealNATSJetStreamAndObjectStoreLifecycle(t *testing.T) {
	server := startEmbeddedNATSServer(t, true)
	client := connectSemStreamsClient(t, server.ClientURL())

	component, err := New(DefaultConfig(), client, nil)
	if err != nil {
		t.Fatalf("create component: %v", err)
	}
	if err := component.Initialize(); err != nil {
		t.Fatalf("initialize component: %v", err)
	}
	startCtx, cancelStart := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelStart()
	if err := component.Start(startCtx); err != nil {
		t.Fatalf("start component with real JetStream: %v", err)
	}
	t.Cleanup(func() {
		_ = component.Stop(5 * time.Second)
	})

	datastreamID := "acme.ops.robotics.gcs.datastream.integration"
	subject := component.cfg.ObservationsSubjectPrefix + "." + datastreamID
	identity := Identity{Forwarded: map[string]string{"User": "alice"}}
	observation := validObservation()
	if err := component.publishObservation(context.Background(), subject, &observation, identity); err != nil {
		t.Fatalf("publish real JetStream observation: %v", err)
	}

	readerPtr := component.reader.Load()
	if readerPtr == nil {
		t.Fatal("real JetStream reader was not initialized")
	}
	messages, err := (*readerPtr).FetchSubject(context.Background(), subject, 1, 0)
	if err != nil {
		t.Fatalf("read real JetStream observation: %v", err)
	}
	if len(messages) != 1 || messages[0].Subject != subject {
		t.Fatalf("observation subject drift: got %+v want %q", messages, subject)
	}
	registry := payloadbuiltins.NewTestRegistry(t)
	if err := oms.RegisterPayloads(registry); err != nil {
		t.Fatalf("register OMS payloads: %v", err)
	}
	decoded, err := message.NewDecoder(registry).Decode(messages[0].Data)
	if err != nil {
		t.Fatalf("decode real JetStream envelope: %v", err)
	}
	if decoded.Type() != oms.SchemaType() {
		t.Fatalf("observation envelope type: got %s want %s", decoded.Type(), oms.SchemaType())
	}

	storePtr := component.schemaArtifacts.Load()
	if storePtr == nil {
		t.Fatal("real ObjectStore was not initialized")
	}
	const objectKey = "integration-schema.json"
	wantSchema := []byte(`{"type":"DataRecord","fields":[]}`)
	if _, err := (*storePtr).PutBytes(context.Background(), objectKey, wantSchema); err != nil {
		t.Fatalf("put real ObjectStore schema: %v", err)
	}
	gotSchema, err := (*storePtr).GetBytes(context.Background(), objectKey)
	if err != nil {
		t.Fatalf("get real ObjectStore schema: %v", err)
	}
	if string(gotSchema) != string(wantSchema) {
		t.Fatalf("ObjectStore schema: got %s want %s", gotSchema, wantSchema)
	}
}

func startEmbeddedNATSServer(t *testing.T, jetStream bool) *natsserver.Server {
	t.Helper()
	options := &natsserver.Options{
		Host:   "127.0.0.1",
		Port:   -1,
		NoLog:  true,
		NoSigs: true,
	}
	if jetStream {
		options.JetStream = true
		options.StoreDir = t.TempDir()
	}
	server, err := natsserver.NewServer(options)
	if err != nil {
		t.Fatalf("create NATS server: %v", err)
	}
	go server.Start()
	if !server.ReadyForConnections(5 * time.Second) {
		server.Shutdown()
		t.Fatal("NATS server did not become ready")
	}
	t.Cleanup(func() {
		server.Shutdown()
		server.WaitForShutdown()
	})
	return server
}

func connectSemStreamsClient(t *testing.T, url string) *natsclient.Client {
	t.Helper()
	client, err := natsclient.NewClient(url)
	if err != nil {
		t.Fatalf("create SemStreams NATS client: %v", err)
	}
	connectCtx, cancelConnect := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelConnect()
	if err := client.Connect(connectCtx); err != nil {
		t.Fatalf("connect SemStreams NATS client: %v", err)
	}
	t.Cleanup(func() {
		closeCtx, cancelClose := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancelClose()
		_ = client.Close(closeCtx)
	})
	return client
}
