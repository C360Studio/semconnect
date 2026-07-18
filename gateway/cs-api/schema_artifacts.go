package csapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/c360studio/semconnect/parser/sensorml"
	csapivocab "github.com/c360studio/semconnect/vocabulary/csapi"
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/pkg/errs"
)

const schemaArtifactContentType = string(MediaJSON)

// createSchemaArtifact stores a canonical SWE schema in ObjectStore, creates
// the first-class SWESchemaDocument graph artifact entity, and returns the
// parent -> artifact relationship triple the caller should add to its resource.
func (c *Component) createSchemaArtifact(
	ctx context.Context,
	parentID string,
	relationshipPredicate string,
	rawSchema json.RawMessage,
	id Identity,
) (message.Triple, error) {
	if parentID == "" {
		return message.Triple{}, errs.WrapInvalid(errors.New("parent entity ID required"), "cs-api", "createSchemaArtifact", "build artifact")
	}
	role, err := schemaArtifactRole(relationshipPredicate)
	if err != nil {
		return message.Triple{}, errs.WrapInvalid(err, "cs-api", "createSchemaArtifact", "build artifact")
	}
	canonical, err := normalizeSWESchema(rawSchema)
	if err != nil {
		return message.Triple{}, errs.WrapInvalid(err, "cs-api", "createSchemaArtifact", "validate SWE schema")
	}
	if len(canonical) == 0 {
		return message.Triple{}, errs.WrapInvalid(errors.New("SWE schema required"), "cs-api", "createSchemaArtifact", "build artifact")
	}

	artifactID := c.mintSchemaArtifactEntityID(parentID, role)
	key := schemaArtifactObjectKey(artifactID)
	triples := []message.Triple{
		{Subject: artifactID, Predicate: sensorml.PredType, Object: csapivocab.SWESchemaDocument},
	}
	entity := &graph.EntityState{
		ID:      artifactID,
		Triples: triples,
		StorageRef: &message.StorageReference{
			StorageInstance: c.cfg.SchemaArtifactsBucket,
			Key:             key,
			ContentType:     schemaArtifactContentType,
			Size:            int64(len(canonical)),
		},
	}
	if err := validateProjectedTriples(artifactID, triples); err != nil {
		return message.Triple{}, errs.WrapInvalid(err, "cs-api", "createSchemaArtifact", "validate final artifact state")
	}

	storePtr := c.schemaArtifacts.Load()
	if storePtr == nil || *storePtr == nil {
		return message.Triple{}, errs.WrapTransient(errors.New("schema artifact object store not initialized"), "cs-api", "createSchemaArtifact", "store schema")
	}
	if _, err := (*storePtr).PutBytes(ctx, key, []byte(canonical)); err != nil {
		return message.Triple{}, classifyJetStreamErr(err, "createSchemaArtifact", "store schema")
	}
	if err := c.createEntityWithTriples(ctx, entity, triples, id, "createSchemaArtifact"); err != nil {
		if !errors.Is(err, errEntityConflict) {
			return message.Triple{}, err
		}
		current, fetchErr := c.fetchEntity(ctx, artifactID)
		if fetchErr != nil {
			return message.Triple{}, fetchErr
		}
		current.StorageRef = entity.StorageRef
		if err := c.replaceEntityTriples(ctx, current, triples, id); err != nil {
			return message.Triple{}, err
		}
	}
	return message.Triple{Subject: parentID, Predicate: relationshipPredicate, Object: artifactID, Datatype: message.EntityReferenceDatatype}, nil
}

func (c *Component) readSchemaArtifact(ctx context.Context, triples []message.Triple, relationshipPredicate string) (json.RawMessage, bool, error) {
	artifactID, ok := firstStringObject(triples, relationshipPredicate)
	if !ok {
		return nil, false, nil
	}
	artifact, err := c.fetchEntity(ctx, artifactID)
	if err != nil {
		return nil, true, err
	}
	if !isSWESchemaArtifact(artifact.Triples) {
		return nil, true, errs.Wrap(
			fmt.Errorf("entity %q is not a SWE schema artifact", artifactID),
			"cs-api", "readSchemaArtifact", "fetch schema artifact")
	}
	if artifact.StorageRef == nil {
		return nil, true, errs.Wrap(
			fmt.Errorf("schema artifact %q has no storage reference", artifactID),
			"cs-api", "readSchemaArtifact", "fetch schema artifact")
	}
	storePtr := c.schemaArtifacts.Load()
	if storePtr == nil || *storePtr == nil {
		return nil, true, errs.WrapTransient(
			errors.New("schema artifact object store not initialized"),
			"cs-api", "readSchemaArtifact", "fetch schema")
	}
	body, err := (*storePtr).GetBytes(ctx, artifact.StorageRef.Key)
	if err != nil {
		return nil, true, classifyJetStreamErr(err, "readSchemaArtifact", "fetch schema")
	}
	return json.RawMessage(body), true, nil
}

func isSWESchemaArtifact(triples []message.Triple) bool {
	typeIRI, ok := firstStringObject(triples, sensorml.PredType)
	return ok && typeIRI == csapivocab.SWESchemaDocument
}

func (c *Component) mintSchemaArtifactEntityID(parentID, role string) string {
	return mintSchemaArtifactID(c.cfg.SchemaArtifactIDPrefix, parentID, role)
}

func schemaArtifactObjectKey(artifactID string) string {
	return artifactID + ".json"
}

func schemaArtifactRole(relationshipPredicate string) (string, error) {
	switch relationshipPredicate {
	case csapivocab.HasResultSchema:
		return "resultSchema", nil
	case csapivocab.HasCommandSchema:
		return "commandSchema", nil
	default:
		return "", fmt.Errorf("unsupported schema relationship predicate %q", relationshipPredicate)
	}
}
