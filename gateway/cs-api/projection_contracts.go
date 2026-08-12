package csapi

import (
	"context"
	"fmt"

	"github.com/c360studio/semconnect/parser/sensorml"
	csapivocab "github.com/c360studio/semconnect/vocabulary/csapi"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/pkg/projection"
	"github.com/c360studio/semstreams/vocabulary"
)

const (
	systemProjectionContractName          = "cs-api.system"
	datastreamProjectionContractName      = "cs-api.datastream"
	procedureProjectionContractName       = "cs-api.procedure"
	deploymentProjectionContractName      = "cs-api.deployment"
	samplingFeatureProjectionContractName = "cs-api.sampling-feature"
	propertyProjectionContractName        = "cs-api.property"
	controlStreamProjectionContractName   = "cs-api.control-stream"
	commandProjectionContractName         = "cs-api.command"
	systemEventProjectionContractName     = "cs-api.system-event"
	feasibilityProjectionContractName     = "cs-api.feasibility"
	schemaArtifactProjectionContractName  = "cs-api.schema-artifact"
)

var (
	systemProjectionMessageType          = message.Type{Domain: "c360", Category: "csapi-system", Version: "v1"}
	datastreamProjectionMessageType      = message.Type{Domain: "c360", Category: "csapi-datastream", Version: "v1"}
	procedureProjectionMessageType       = message.Type{Domain: "c360", Category: "csapi-procedure", Version: "v1"}
	deploymentProjectionMessageType      = message.Type{Domain: "c360", Category: "csapi-deployment", Version: "v1"}
	samplingFeatureProjectionMessageType = message.Type{Domain: "c360", Category: "csapi-sampling-feature", Version: "v1"}
	propertyProjectionMessageType        = message.Type{Domain: "c360", Category: "csapi-property", Version: "v1"}
	controlStreamProjectionMessageType   = message.Type{Domain: "c360", Category: "csapi-control-stream", Version: "v1"}
	commandProjectionMessageType         = message.Type{Domain: "c360", Category: "csapi-command", Version: "v1"}
	systemEventProjectionMessageType     = message.Type{Domain: "c360", Category: "csapi-system-event", Version: "v1"}
	feasibilityProjectionMessageType     = message.Type{Domain: "c360", Category: "csapi-feasibility", Version: "v1"}
	schemaArtifactProjectionMessageType  = message.Type{Domain: "c360", Category: "csapi-schema-artifact", Version: "v1"}
)

func representationContract(name string, mt message.Type, pattern string, predicates ...string) projection.Contract {
	return projection.Contract{
		Name:            name,
		MessageType:     mt.Key(),
		EntityPattern:   pattern,
		BirthPredicates: []string{sensorml.PredType, vocabulary.EntityIndexingProfile},
		Groups: []projection.PredicateGroup{{
			Name:       "representation",
			Mode:       projection.ModeReconcile,
			Predicates: predicates,
		}},
		IndexingProfile: "content",
	}
}

func birthOnlyContract(name string, mt message.Type, pattern string, predicates ...string) projection.Contract {
	return projection.Contract{
		Name:            name,
		MessageType:     mt.Key(),
		EntityPattern:   pattern,
		BirthPredicates: append([]string{sensorml.PredType, vocabulary.EntityIndexingProfile}, predicates...),
		IndexingProfile: "content",
	}
}

func projectionContracts(cfg Config) []projection.Contract {
	registerProjectionVocabulary()
	common := []string{
		sensorml.PredUniqueID,
		sensorml.PredLabel,
		sensorml.PredDescription,
		sensorml.PredDefinition,
		sensorml.PredIdentifierValue,
		sensorml.PredCapabilityValue,
		sensorml.PredCharacteristicValue,
	}
	system := append(append([]string{}, common...),
		sensorml.PredPosition,
		sensorml.PredHosts,
		sensorml.PredIsHostedBy,
		sensorml.PredAttachedTo,
		vocabulary.GeoLocationLongitude,
		vocabulary.GeoLocationLatitude,
		vocabulary.GeoLocationAltitude,
	)
	datastream := append(append([]string{}, common...), PredDatastreamSystem, csapivocab.ObservedProperty,
		PredDatastreamSchema, predDatastreamPhenomenonTime, predDatastreamResultTime)
	procedure := append(append([]string{}, common...),
		sensorml.PredUsedProcedure,
		sensorml.PredAttachedTo,
		sensorml.PredHasSubSystem,
	)
	deployment := append(append([]string{}, common...), sensorml.PredPosition,
		vocabulary.GeoLocationLongitude, vocabulary.GeoLocationLatitude, vocabulary.GeoLocationAltitude,
		predDeploymentDeployedSystems, predDeploymentParent)
	samplingFeature := append(append([]string{}, common...), sensorml.PredPosition,
		vocabulary.GeoLocationLongitude, vocabulary.GeoLocationLatitude, vocabulary.GeoLocationAltitude,
		predSamplingFeatureHostedProcedure)
	controlStream := append(append([]string{}, common...), PredControlStreamSystem,
		predControlStreamInputName, predControlStreamAsync, predControlStreamCommandFormat,
		predControlStreamSchema, predControlStreamControlledProperties,
		predControlStreamIssueTime, predControlStreamExecutionTime)
	command := append(append([]string{}, common...), PredCommandControlStream, predCommandStatus,
		predCommandIssueTime, predCommandExecutionTime, predCommandSender, predCommandParams)
	systemEvent := append(append([]string{}, common...), PredSystemEventSystem, predSystemEventTime,
		predSystemEventType, predSystemEventMessage, predSystemEventSeverity, predSystemEventSource,
		predSystemEventPayload, predSystemEventKeywords)
	feasibility := append(append([]string{}, common...), PredFeasibilityControlStream,
		predFeasibilityStatus, predFeasibilityParams, predFeasibilityResult)
	return []projection.Contract{
		representationContract(systemProjectionContractName, systemProjectionMessageType, cfg.SystemIDPrefix+".*", system...),
		// Six wildcards intentionally preserve client-supplied federated six-part IDs.
		representationContract(datastreamProjectionContractName, datastreamProjectionMessageType, "*.*.*.*.*.*", datastream...),
		birthOnlyContract(procedureProjectionContractName, procedureProjectionMessageType, cfg.ProcedureIDPrefix+".*", procedure...),
		birthOnlyContract(deploymentProjectionContractName, deploymentProjectionMessageType, cfg.DeploymentIDPrefix+".*", deployment...),
		birthOnlyContract(samplingFeatureProjectionContractName, samplingFeatureProjectionMessageType, cfg.SamplingFeatureIDPrefix+".*", samplingFeature...),
		birthOnlyContract(propertyProjectionContractName, propertyProjectionMessageType, cfg.PropertyIDPrefix+".*", append(common, predPropertyDefinition, predPropertyBaseProperty)...),
		birthOnlyContract(controlStreamProjectionContractName, controlStreamProjectionMessageType, cfg.ControlStreamIDPrefix+".*", controlStream...),
		birthOnlyContract(commandProjectionContractName, commandProjectionMessageType, cfg.CommandIDPrefix+".*", command...),
		birthOnlyContract(systemEventProjectionContractName, systemEventProjectionMessageType, cfg.SystemEventIDPrefix+".*", systemEvent...),
		birthOnlyContract(feasibilityProjectionContractName, feasibilityProjectionMessageType, cfg.FeasibilityIDPrefix+".*", feasibility...),
		birthOnlyContract(schemaArtifactProjectionContractName, schemaArtifactProjectionMessageType, cfg.SchemaArtifactIDPrefix+".*"),
	}
}

// registerProjectionVocabulary declares the legacy SemStreams geo predicates
// at this application composition boundary. The constants intentionally do not
// self-register upstream, while beta.160 projection contracts require every
// authored predicate to be present in the vocabulary registry.
func registerProjectionVocabulary() {
	if vocabulary.GetPredicateMetadata(vocabulary.EntityIndexingProfile) == nil {
		vocabulary.Register(vocabulary.EntityIndexingProfile,
			vocabulary.WithDescription("Graph-ingest indexing eligibility profile stamped at entity birth"),
			vocabulary.WithDataType("string"))
	}
	geoPredicates := []struct {
		name, description, units, valueRange string
	}{
		{vocabulary.GeoLocationLongitude, "Longitude of an entity location", "degrees", "-180 to 180"},
		{vocabulary.GeoLocationLatitude, "Latitude of an entity location", "degrees", "-90 to 90"},
		{vocabulary.GeoLocationAltitude, "Altitude of an entity location", "meters", ""},
	}
	for _, predicate := range geoPredicates {
		if vocabulary.GetPredicateMetadata(predicate.name) != nil {
			continue
		}
		options := []vocabulary.Option{
			vocabulary.WithDescription(predicate.description),
			vocabulary.WithDataType("float64"),
			vocabulary.WithUnits(predicate.units),
		}
		if predicate.valueRange != "" {
			options = append(options, vocabulary.WithRange(predicate.valueRange))
		}
		vocabulary.Register(predicate.name, options...)
	}
}

func (c *Component) bindProjectionContracts(_ context.Context) error {
	contracts := projectionContracts(c.cfg)
	if err := projection.ValidateContracts(contracts); err != nil {
		return fmt.Errorf("validate cs-api projection contracts: %w", err)
	}
	c.logger.Info("validated cs-api projection contracts", "count", len(contracts))
	return nil
}
