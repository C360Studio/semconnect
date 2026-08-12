package csapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/c360studio/semconnect/parser/sensorml"
	csapivocab "github.com/c360studio/semconnect/vocabulary/csapi"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/pkg/errs"
	"github.com/nats-io/nats.go/jetstream"
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
	if _, err := schemaArtifactRole(relationshipPredicate); err != nil {
		return message.Triple{}, errs.WrapInvalid(err, "cs-api", "createSchemaArtifact", "build artifact")
	}
	canonical, err := normalizeSWESchema(rawSchema)
	if err != nil {
		return message.Triple{}, errs.WrapInvalid(err, "cs-api", "createSchemaArtifact", "validate SWE schema")
	}
	if len(canonical) == 0 {
		return message.Triple{}, errs.WrapInvalid(errors.New("SWE schema required"), "cs-api", "createSchemaArtifact", "build artifact")
	}

	digest := sha256.Sum256(canonical)
	artifactID := c.contentAddressedSchemaArtifactID(digest)
	key := schemaArtifactObjectKey(artifactID)
	triples := []message.Triple{
		{Subject: artifactID, Predicate: sensorml.PredType, Object: csapivocab.SWESchemaDocument},
	}
	storageRef := &message.StorageReference{
		StorageInstance: schemaArtifactStorageInstance,
		Key:             key,
		ContentType:     schemaArtifactContentType,
		Size:            int64(len(canonical)),
	}
	if err := validateProjectedTriples(artifactID, triples); err != nil {
		return message.Triple{}, errs.WrapInvalid(err, "cs-api", "createSchemaArtifact", "validate final artifact state")
	}

	storePtr := c.schemaArtifacts.Load()
	if storePtr == nil || *storePtr == nil {
		return message.Triple{}, errs.WrapTransient(errors.New("schema artifact object store not initialized"), "cs-api", "createSchemaArtifact", "store schema")
	}
	store := *storePtr
	existing, getErr := store.GetBytes(ctx, key)
	switch {
	case getErr == nil:
		if err := verifySchemaArtifactBytes(artifactID, existing, canonical, digest); err != nil {
			return message.Triple{}, errs.Wrap(err, "cs-api", "createSchemaArtifact", "verify content-addressed object")
		}
	case errors.Is(getErr, jetstream.ErrObjectNotFound):
		if _, putErr := store.PutBytes(ctx, key, []byte(canonical)); putErr != nil {
			return message.Triple{}, classifyJetStreamErr(putErr, "createSchemaArtifact", "store schema")
		}
		stored, verifyErr := store.GetBytes(ctx, key)
		if verifyErr != nil {
			return message.Triple{}, classifyJetStreamErr(verifyErr, "createSchemaArtifact", "verify stored schema")
		}
		if err := verifySchemaArtifactBytes(artifactID, stored, canonical, digest); err != nil {
			return message.Triple{}, errs.Wrap(err, "cs-api", "createSchemaArtifact", "verify stored schema")
		}
	default:
		return message.Triple{}, classifyJetStreamErr(getErr, "createSchemaArtifact", "inspect content-addressed object")
	}

	// Object durability precedes graph visibility. A later graph failure may
	// leave a digest-addressed orphan for deferred GC, but no graph artifact is
	// ever created before its canonical bytes are verified present.
	createErr := c.createProjectedEntity(ctx, schemaArtifactProjectionContractName, artifactID, triples,
		schemaArtifactProjectionMessageType, storageRef, id, "createSchemaArtifact")
	if createErr != nil {
		if !errors.Is(createErr, errEntityConflict) {
			return message.Triple{}, createErr
		}
		current, fetchErr := c.fetchEntityExact(ctx, artifactID)
		if fetchErr != nil {
			return message.Triple{}, fetchErr
		}
		if current.Entity == nil || !isSWESchemaArtifact(current.Entity.Triples) || !storageReferencesEqual(current.Entity.StorageRef, storageRef) {
			return message.Triple{}, errs.Wrap(
				fmt.Errorf("immutable schema artifact %q conflicts with digest identity or storage reference", artifactID),
				"cs-api", "createSchemaArtifact", "verify immutable artifact")
		}
		existing, getErr := store.GetBytes(ctx, key)
		if getErr != nil {
			return message.Triple{}, classifyJetStreamErr(getErr, "createSchemaArtifact", "verify immutable artifact bytes")
		}
		if err := verifySchemaArtifactBytes(artifactID, existing, canonical, digest); err != nil {
			return message.Triple{}, errs.Wrap(err, "cs-api", "createSchemaArtifact", "verify immutable artifact bytes")
		}
		return message.Triple{Subject: parentID, Predicate: relationshipPredicate, Object: artifactID, Datatype: message.EntityReferenceDatatype}, nil
	}
	return message.Triple{Subject: parentID, Predicate: relationshipPredicate, Object: artifactID, Datatype: message.EntityReferenceDatatype}, nil
}

func verifySchemaArtifactBytes(artifactID string, actual, canonical []byte, digest [sha256.Size]byte) error {
	if sha256.Sum256(actual) != digest || !bytes.Equal(actual, canonical) {
		return fmt.Errorf("immutable schema artifact %q object bytes do not match canonical digest identity", artifactID)
	}
	return nil
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

func (c *Component) contentAddressedSchemaArtifactID(digest [sha256.Size]byte) string {
	return fmt.Sprintf("%s.sha256-%x", c.cfg.SchemaArtifactIDPrefix, digest)
}

func schemaArtifactObjectKey(artifactID string) string {
	token := artifactID
	if separator := strings.LastIndexByte(artifactID, '.'); separator >= 0 {
		token = artifactID[separator+1:]
	}
	return token + ".json"
}

func storageReferencesEqual(left, right *message.StorageReference) bool {
	if left == nil || right == nil {
		return left == right
	}
	return left.StorageInstance == right.StorageInstance && left.Key == right.Key &&
		left.ContentType == right.ContentType && left.Size == right.Size
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
