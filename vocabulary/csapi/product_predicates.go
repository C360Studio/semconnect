package csapi

import "github.com/c360studio/semconnect/vocabulary/sosa"

// Product-owned CS API graph predicates. Internal identities are deliberately
// independent from their standards-shaped JSON and RDF boundary names.
const (
	ObservedProperty = "csapi.datastream.observed-property"

	DeploymentDeployedSystems      = "cs-api.deployment.deployed-systems"
	DeploymentParent               = "cs-api.deployment.parent"
	SamplingFeatureHostedProcedure = "cs-api.samplingfeature.hosted-procedure"

	ControlStreamInputName            = "cs-api.controlstream.input-name"
	ControlStreamAsync                = "cs-api.controlstream.async"
	ControlStreamCommandFormat        = "cs-api.controlstream.command-format"
	ControlStreamControlledProperties = "cs-api.controlstream.controlled-properties"
	ControlStreamIssueTime            = "cs-api.controlstream.issue-time"
	ControlStreamExecutionTime        = "cs-api.controlstream.execution-time"

	CommandIssueTime     = "cs-api.command.issue-time"
	CommandExecutionTime = "cs-api.command.execution-time"
	CommandStatus        = "cs-api.command.status"
	CommandSender        = "cs-api.command.sender"
	CommandParams        = "cs-api.command.params"

	DatastreamPhenomenonTime = "cs-api.datastream.phenomenon-time"
	DatastreamResultTime     = "cs-api.datastream.result-time"

	PropertyDefinition   = "cs-api.property.definition"
	PropertyBaseProperty = "cs-api.property.base-property"

	SystemEventTime     = "cs-api.systemevent.time"
	SystemEventType     = "cs-api.systemevent.type"
	SystemEventMessage  = "cs-api.systemevent.message"
	SystemEventSeverity = "cs-api.systemevent.severity"
	SystemEventSource   = "cs-api.systemevent.source"
	SystemEventPayload  = "cs-api.systemevent.payload"
	SystemEventKeywords = "cs-api.systemevent.keywords"

	FeasibilityControlStream = "cs-api.feasibility.controlstream"
	FeasibilityStatus        = "cs-api.feasibility.status"
	FeasibilityParams        = "cs-api.feasibility.params"
	FeasibilityResult        = "cs-api.feasibility.result"
)

// Product-owned boundary IRIs preserve the CS API/RDF vocabulary independently
// of the lower-kebab graph identities above.
const (
	ObservedPropertyIRI = sosa.ObservedProperty

	DeploymentDeployedSystemsIRI      = Namespace + "deployedSystems"
	DeploymentParentIRI               = Namespace + "parent"
	SamplingFeatureHostedProcedureIRI = Namespace + "hostedProcedure"

	ControlStreamInputNameIRI            = Namespace + "inputName"
	ControlStreamAsyncIRI                = Namespace + "async"
	ControlStreamCommandFormatIRI        = Namespace + "commandFormat"
	ControlStreamControlledPropertiesIRI = Namespace + "controlledProperties"
	ControlStreamIssueTimeIRI            = Namespace + "issueTime"
	ControlStreamExecutionTimeIRI        = Namespace + "executionTime"

	CommandIssueTimeIRI     = Namespace + "issueTime"
	CommandExecutionTimeIRI = Namespace + "executionTime"
	CommandStatusIRI        = Namespace + "status"
	CommandSenderIRI        = Namespace + "sender"
	CommandParamsIRI        = Namespace + "params"

	DatastreamPhenomenonTimeIRI = Namespace + "phenomenonTime"
	DatastreamResultTimeIRI     = Namespace + "resultTime"

	PropertyDefinitionIRI   = Namespace + "definition"
	PropertyBasePropertyIRI = Namespace + "baseProperty"

	SystemEventTimeIRI     = Namespace + "time"
	SystemEventTypeIRI     = Namespace + "eventType"
	SystemEventMessageIRI  = Namespace + "message"
	SystemEventSeverityIRI = Namespace + "severity"
	SystemEventSourceIRI   = Namespace + "source"
	SystemEventPayloadIRI  = Namespace + "payload"
	SystemEventKeywordsIRI = Namespace + "keywords"

	FeasibilityControlStreamIRI = Namespace + "controlstream"
	FeasibilityStatusIRI        = Namespace + "status"
	FeasibilityParamsIRI        = Namespace + "params"
	FeasibilityResultIRI        = Namespace + "result"
)
