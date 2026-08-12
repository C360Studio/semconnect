package csapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/c360studio/semconnect/parser/sensorml"
	csapivocab "github.com/c360studio/semconnect/vocabulary/csapi"
	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/pkg/errs"
	"github.com/c360studio/semstreams/pkg/projection"
	semtypes "github.com/c360studio/semstreams/pkg/types"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
)

const schemaArtifactStorageInstance = "objectstore"

type commitUnknownError struct {
	correlation string
	err         error
}

func (e *commitUnknownError) Error() string {
	return fmt.Sprintf("graph mutation commit outcome unknown (correlation %s): %v", e.correlation, e.err)
}

func (e *commitUnknownError) Unwrap() error { return e.err }

type mutationAttempt struct {
	requestID string
	traceID   string
	identity  Identity
	operation string
	entityID  string
	revision  uint64
}

func newMutationAttempt(ctx context.Context, identity Identity, operation, entityID string, revision uint64) mutationAttempt {
	traceID := ""
	if trace, ok := natsclient.TraceContextFromContext(ctx); ok && trace != nil {
		traceID = trace.TraceID
	}
	if traceID == "" {
		traceID = natsclient.NewTraceContext().TraceID
	}
	return mutationAttempt{
		requestID: uuid.NewString(), traceID: traceID, identity: identity,
		operation: operation, entityID: entityID, revision: revision,
	}
}

func (c *Component) auditMutation(attempt mutationAttempt, commit string, revision uint64, err error) {
	classifiedError := ""
	if err != nil {
		classifiedError = err.Error()
	}
	attrs := []any{
		"request_id", attempt.requestID,
		"trace_id", attempt.traceID,
		"operation", attempt.operation,
		"entity_id", attempt.entityID,
		"commit_state", commit,
		"kv_revision", revision,
		"identity_subject", attempt.identity.Subject,
		"identity_verified", attempt.identity.Verified,
		"forwarded_user", attempt.identity.Forwarded["User"],
		"forwarded_email", attempt.identity.Forwarded["Email"],
		"classified_error", classifiedError,
	}
	if err != nil {
		c.logger.Log(context.Background(), slog.LevelWarn, "graph mutation audit", attrs...)
		return
	}
	c.logger.Log(context.Background(), slog.LevelInfo, "graph mutation audit", attrs...)
}

func (c *Component) requestTypedMutation(
	ctx context.Context,
	subject string,
	payload any,
	attempt mutationAttempt,
) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		wrapped := errs.WrapTransient(err, "cs-api", attempt.operation, "graph mutation canceled before request")
		c.auditMutation(attempt, "not-committed", attempt.revision, wrapped)
		return nil, wrapped
	}
	body, err := json.Marshal(payload)
	if err != nil {
		wrapped := errs.Wrap(err, "cs-api", attempt.operation, "marshal typed graph mutation")
		c.auditMutation(attempt, "not-committed", attempt.revision, wrapped)
		return nil, wrapped
	}
	reply, err := c.nats.RequestWithHeaders(ctx, subject, body, nil, c.cfg.QueryTimeout)
	if err != nil {
		if errors.Is(err, nats.ErrNoResponders) {
			wrapped := errs.WrapTransient(err, "cs-api", attempt.operation, "graph mutation unavailable")
			c.auditMutation(attempt, "not-committed", attempt.revision, wrapped)
			return nil, wrapped
		}
		unknown := &commitUnknownError{correlation: attempt.requestID, err: err}
		wrapped := errs.WrapTransient(unknown, "cs-api", attempt.operation, "graph mutation commit outcome unknown")
		c.auditMutation(attempt, "unknown", attempt.revision, wrapped)
		return nil, wrapped
	}
	data, err := natsclient.ClassifyReply(reply)
	if err != nil {
		classified := mutationFailure(attempt.operation, err)
		c.auditMutation(attempt, "not-committed", attempt.revision, classified)
		return nil, classified
	}
	return data, nil
}

func (c *Component) graphMutationSubject(operation string) (string, error) {
	switch operation {
	case graphMutationCreateOp, graphMutationReconcileOp, graphMutationDeleteOp:
	default:
		return "", fmt.Errorf("unsupported typed graph mutation operation %q", operation)
	}
	return component.ResolveSubject(c.outputPortDefinitions(), graphMutationPortName, operation)
}

func (c *Component) createProjectedEntity(
	ctx context.Context,
	contractName, entityID string,
	triples []message.Triple,
	mt message.Type,
	storageRef *message.StorageReference,
	identity Identity,
	op string,
) error {
	if !mt.IsValid() {
		return errs.WrapInvalid(errors.New("valid resource-specific message type required"), "cs-api", op, "validate projection")
	}
	if err := validateProjectedTriples(entityID, triples); err != nil {
		return errs.WrapInvalid(err, "cs-api", op, "validate mutation triples")
	}
	contract, ok := projectionContractByName(c.cfg, contractName)
	if !ok {
		return errs.WrapInvalid(fmt.Errorf("unknown projection contract %q", contractName), "cs-api", op, "validate projection")
	}
	if contract.MessageType != mt.Key() {
		return errs.WrapInvalid(fmt.Errorf("message type %q does not match contract %q", mt.Key(), contractName), "cs-api", op, "validate projection")
	}
	if err := contract.Validate(); err != nil {
		return errs.WrapInvalid(err, "cs-api", op, "validate projection contract")
	}
	if err := validateProjectionFacts(contract, entityID, triples); err != nil {
		return errs.WrapInvalid(err, "cs-api", op, "validate projection facts")
	}
	entity := &graph.EntityState{ID: entityID, MessageType: mt, StorageRef: storageRef}
	attempt := newMutationAttempt(ctx, identity, op, entityID, 0)
	subject, err := c.graphMutationSubject(graphMutationCreateOp)
	if err != nil {
		return errs.Wrap(err, "cs-api", op, "resolve typed graph mutation subject")
	}
	data, err := c.requestTypedMutation(ctx, subject, graph.CreateEntityRequest{
		Entity: entity, Triples: triples, IndexingProfile: contract.IndexingProfile,
		TraceID: attempt.traceID, RequestID: attempt.requestID,
	}, attempt)
	if err != nil {
		return err
	}
	var response graph.CreateEntityResponse
	if err := json.Unmarshal(data, &response); err != nil || response.Outcome != graph.MutationApplied ||
		response.Entity == nil || response.Entity.ID != entityID || response.KVRevision == 0 {
		if err == nil {
			err = errors.New("invalid typed create response")
		}
		unknown := &commitUnknownError{correlation: attempt.requestID, err: err}
		wrapped := errs.WrapTransient(unknown, "cs-api", op, "graph mutation commit outcome unknown")
		c.auditMutation(attempt, "unknown", 0, wrapped)
		return wrapped
	}
	c.auditMutation(attempt, "verified", response.KVRevision, nil)
	return nil
}

func (c *Component) reconcileEntity(
	ctx context.Context,
	exact graph.ExactEntity,
	desired []message.Triple,
	identity Identity,
	op string,
) error {
	if exact.Entity == nil || exact.KVRevision == 0 {
		return errs.WrapInvalid(errors.New("exact entity and nonzero KV revision required"), "cs-api", op, "authorize reconcile")
	}
	if err := validateProjectedTriples(exact.Entity.ID, desired); err != nil {
		return errs.WrapInvalid(err, "cs-api", op, "validate desired representation")
	}
	attempt := newMutationAttempt(ctx, identity, op, exact.Entity.ID, exact.KVRevision)
	contractName, _ := c.projectionForTriples(exact.Entity.ID, exact.Entity.Triples)
	contract, ok := projectionContractByName(c.cfg, contractName)
	if !ok || len(contract.Groups) != 1 || contract.Groups[0].Name != "representation" || contract.Groups[0].Mode != projection.ModeReconcile {
		return errs.WrapInvalid(fmt.Errorf("entity %q has no representation reconcile group", exact.Entity.ID), "cs-api", op, "authorize reconcile")
	}
	if err := validateProjectionFacts(contract, exact.Entity.ID, desired); err != nil {
		return errs.WrapInvalid(err, "cs-api", op, "validate desired representation contract")
	}
	desiredRepresentation := triplesForPredicates(desired, contract.Groups[0].Predicates)
	subject, err := c.graphMutationSubject(graphMutationReconcileOp)
	if err != nil {
		return errs.Wrap(err, "cs-api", op, "resolve typed graph mutation subject")
	}
	data, err := c.requestTypedMutation(ctx, subject, graph.ReconcilePredicatesRequest{
		EntityID: exact.Entity.ID, ExpectedRevision: exact.KVRevision,
		Predicates: append([]string(nil), contract.Groups[0].Predicates...), Desired: desiredRepresentation,
		TraceID: attempt.traceID, RequestID: attempt.requestID,
	}, attempt)
	if err != nil {
		return err
	}
	var response graph.ReconcilePredicatesResponse
	if err := json.Unmarshal(data, &response); err != nil ||
		(response.Outcome != graph.MutationApplied && response.Outcome != graph.MutationUnchanged) ||
		response.Entity == nil || response.Entity.ID != exact.Entity.ID || response.KVRevision == 0 {
		if err == nil {
			err = errors.New("invalid typed reconcile response")
		}
		unknown := &commitUnknownError{correlation: attempt.requestID, err: err}
		wrapped := errs.WrapTransient(unknown, "cs-api", op, "graph mutation commit outcome unknown")
		c.auditMutation(attempt, "unknown", exact.KVRevision, wrapped)
		return wrapped
	}
	c.auditMutation(attempt, "verified", response.KVRevision, nil)
	return nil
}

func validateProjectionFacts(contract projection.Contract, entityID string, triples []message.Triple) error {
	matched, err := semtypes.MatchEntityIDPattern(contract.EntityPattern, entityID)
	if err != nil {
		return fmt.Errorf("match entity pattern: %w", err)
	}
	if !matched {
		return fmt.Errorf("entity %q does not match contract pattern %q", entityID, contract.EntityPattern)
	}
	allowed := make(map[string]struct{}, len(contract.BirthPredicates))
	for _, predicate := range contract.BirthPredicates {
		allowed[predicate] = struct{}{}
	}
	for _, group := range contract.Groups {
		for _, predicate := range group.Predicates {
			allowed[predicate] = struct{}{}
		}
	}
	for index, triple := range triples {
		if triple.Subject != entityID {
			return fmt.Errorf("triple[%d] subject %q does not match entity %q", index, triple.Subject, entityID)
		}
		if _, ok := allowed[triple.Predicate]; !ok {
			return fmt.Errorf("predicate %q is not declared by contract %q", triple.Predicate, contract.Name)
		}
	}
	return nil
}

func (c *Component) deleteExactEntity(ctx context.Context, exact graph.ExactEntity, identity Identity) error {
	if exact.Entity == nil || exact.KVRevision == 0 {
		return errs.WrapInvalid(errors.New("exact entity and nonzero KV revision required"), "cs-api", "deleteEntity", "authorize delete")
	}
	attempt := newMutationAttempt(ctx, identity, "deleteEntity", exact.Entity.ID, exact.KVRevision)
	subject, err := c.graphMutationSubject(graphMutationDeleteOp)
	if err != nil {
		return errs.Wrap(err, "cs-api", "deleteEntity", "resolve typed graph mutation subject")
	}
	data, err := c.requestTypedMutation(ctx, subject, graph.DeleteEntityRequest{
		EntityID: exact.Entity.ID, ExpectedRevision: exact.KVRevision,
		TraceID: attempt.traceID, RequestID: attempt.requestID,
	}, attempt)
	if err != nil {
		return err
	}
	var response graph.DeleteEntityResponse
	if err := json.Unmarshal(data, &response); err != nil || response.Outcome != graph.MutationApplied ||
		response.EntityID != exact.Entity.ID || response.ExpectedRevision != exact.KVRevision {
		if err == nil {
			err = errors.New("invalid typed delete response")
		}
		unknown := &commitUnknownError{correlation: attempt.requestID, err: err}
		wrapped := errs.WrapTransient(unknown, "cs-api", "deleteEntity", "graph mutation commit outcome unknown")
		c.auditMutation(attempt, "unknown", exact.KVRevision, wrapped)
		return wrapped
	}
	c.auditMutation(attempt, "verified", exact.KVRevision, nil)
	return nil
}

func triplesForPredicates(triples []message.Triple, predicates []string) []message.Triple {
	allowed := make(map[string]struct{}, len(predicates))
	for _, predicate := range predicates {
		allowed[predicate] = struct{}{}
	}
	out := make([]message.Triple, 0, len(triples))
	for _, triple := range triples {
		if _, ok := allowed[triple.Predicate]; ok {
			out = append(out, triple)
		}
	}
	return out
}

func projectionContractByName(cfg Config, name string) (projection.Contract, bool) {
	for _, contract := range projectionContracts(cfg) {
		if contract.Name == name {
			return contract, true
		}
	}
	return projection.Contract{}, false
}

func rootSubjectTriples(entityID string, triples []message.Triple) []message.Triple {
	out := make([]message.Triple, 0, len(triples))
	for _, triple := range triples {
		if triple.Subject == entityID {
			out = append(out, triple)
		}
	}
	return out
}

func (c *Component) projectionForTriples(entityID string, triples []message.Triple) (string, message.Type) {
	typeIRI, _ := firstStringObject(triples, sensorml.PredType)
	switch typeIRI {
	case csapivocab.Datastream:
		return datastreamProjectionContractName, datastreamProjectionMessageType
	case csapivocab.ControlStream:
		return controlStreamProjectionContractName, controlStreamProjectionMessageType
	case csapivocab.Command:
		return commandProjectionContractName, commandProjectionMessageType
	case csapivocab.SystemEvent:
		return systemEventProjectionContractName, systemEventProjectionMessageType
	case csapivocab.Feasibility:
		return feasibilityProjectionContractName, feasibilityProjectionMessageType
	case csapivocab.SWESchemaDocument:
		return schemaArtifactProjectionContractName, schemaArtifactProjectionMessageType
	}
	for _, candidate := range []struct {
		prefix, contract string
		mt               message.Type
	}{
		{c.cfg.ProcedureIDPrefix, procedureProjectionContractName, procedureProjectionMessageType},
		{c.cfg.DeploymentIDPrefix, deploymentProjectionContractName, deploymentProjectionMessageType},
		{c.cfg.SamplingFeatureIDPrefix, samplingFeatureProjectionContractName, samplingFeatureProjectionMessageType},
		{c.cfg.PropertyIDPrefix, propertyProjectionContractName, propertyProjectionMessageType},
		{c.cfg.SystemIDPrefix, systemProjectionContractName, systemProjectionMessageType},
	} {
		if len(entityID) > len(candidate.prefix) && entityID[:len(candidate.prefix)] == candidate.prefix {
			return candidate.contract, candidate.mt
		}
	}
	return systemProjectionContractName, systemProjectionMessageType
}
