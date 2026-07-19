export interface SemanticField {
  label: string;
  description: string;
  relationshipLabel?: string;
}

export const SEMANTIC_PREDICATES = {
  processType: 'sensorml.process.type',
  observationType: 'oms.observation.type',
  hostedBy: 'sensorml.component.is-hosted-by',
  datastreamProducedBy: 'csapi.datastream.produced-by',
  datastreamObservedProperty: 'csapi.datastream.observed-property',
  datastreamResultSchema: 'csapi.datastream.result-schema',
  controlstreamControlsSystem: 'csapi.controlstream.controls-system',
  controlstreamCommandSchema: 'csapi.controlstream.command-schema',
  controlstreamCommandFormat: 'cs-api.controlstream.command-format',
  controlstreamControlledProperties: 'cs-api.controlstream.controlled-properties',
  commandPartOfControlStream: 'csapi.command.part-of-control-stream',
  commandIssueTime: 'cs-api.command.issue-time',
  commandExecutionTime: 'cs-api.command.execution-time',
  systemEventForSystem: 'csapi.systemevent.for-system',
  feasibilityControlstream: 'cs-api.feasibility.controlstream',
  deploymentDeployedSystems: 'cs-api.deployment.deployed-systems',
  samplingFeatureHostedProcedure: 'cs-api.samplingfeature.hosted-procedure'
} as const;

export const MIGRATED_SEMANTIC_FIELDS: Readonly<Record<string, SemanticField>> = {
  'oms.observation.has-feature-of-interest': {
    label: 'Feature of interest',
    description: 'The feature whose property was observed.'
  },
  'oms.observation.has-simple-result': {
    label: 'Simple result',
    description: 'The scalar value produced by the observation.'
  },
  'oms.observation.observed-property': {
    label: 'Observed property',
    description: 'The property evaluated by the observation.'
  },
  'oms.observation.phenomenon-time': {
    label: 'Phenomenon time',
    description: 'The time at which the observed phenomenon occurred.'
  },
  'oms.observation.result-time': {
    label: 'Result time',
    description: 'The time at which the observation result became available.'
  },
  'oms.observation.used-procedure': {
    label: 'Procedure',
    description: 'The procedure used to make the observation.'
  },
  [SEMANTIC_PREDICATES.hostedBy]: {
    label: 'Hosted by',
    description: 'The parent System that hosts this component.',
    relationshipLabel: 'hosted by'
  },
  'sensorml.process.attached-to': {
    label: 'Attached to',
    description: 'The platform or component to which this process is attached.'
  },
  'sensorml.process.has-sub-system': {
    label: 'Subsystem',
    description: 'A child System contained by this aggregate process.',
    relationshipLabel: 'contains subsystem'
  },
  'sensorml.process.used-procedure': {
    label: 'Procedure',
    description: 'The procedure implemented or used by this process.'
  },
  [SEMANTIC_PREDICATES.commandPartOfControlStream]: {
    label: 'Control stream',
    description: 'The ControlStream through which this Command was issued.',
    relationshipLabel: 'issued through'
  },
  [SEMANTIC_PREDICATES.controlstreamCommandSchema]: {
    label: 'Command schema',
    description: 'The SWE Common schema artifact describing accepted commands.',
    relationshipLabel: 'uses command schema'
  },
  [SEMANTIC_PREDICATES.controlstreamControlsSystem]: {
    label: 'Controlled system',
    description: 'The System targeted by this ControlStream.',
    relationshipLabel: 'controls'
  },
  'csapi.datastream.phenomenon-time-range': {
    label: 'Phenomenon time range',
    description: 'The temporal range covered by phenomena observed in this Datastream.'
  },
  [SEMANTIC_PREDICATES.datastreamProducedBy]: {
    label: 'Producing system',
    description: 'The System that produces observations for this Datastream.',
    relationshipLabel: 'produced by'
  },
  [SEMANTIC_PREDICATES.datastreamResultSchema]: {
    label: 'Result schema',
    description: 'The SWE Common schema artifact used to interpret observation results.',
    relationshipLabel: 'uses result schema'
  },
  'csapi.datastream.result-time-range': {
    label: 'Result time range',
    description: 'The temporal range in which Datastream results became available.'
  },
  'csapi.datastream.result-type': {
    label: 'Result type',
    description: 'The observation-result structure produced by this Datastream.'
  },
  [SEMANTIC_PREDICATES.systemEventForSystem]: {
    label: 'System',
    description: 'The System described by this event.',
    relationshipLabel: 'event for'
  },
  'cs-api.controlstream.input-name': {
    label: 'Input name',
    description: 'The name of the command input exposed by this ControlStream.'
  },
  [SEMANTIC_PREDICATES.controlstreamCommandFormat]: {
    label: 'Command format',
    description: 'The media format used to encode commands.'
  },
  [SEMANTIC_PREDICATES.controlstreamControlledProperties]: {
    label: 'Controlled properties',
    description: 'Serialized metadata describing the properties that commands can change.'
  },
  'cs-api.controlstream.issue-time': {
    label: 'Issue time',
    description: 'The time range in which commands may be issued.'
  },
  'cs-api.controlstream.execution-time': {
    label: 'Execution time',
    description: 'The time range in which commands may execute.'
  },
  [SEMANTIC_PREDICATES.commandIssueTime]: {
    label: 'Issue time',
    description: 'The time at which the Command was issued.'
  },
  [SEMANTIC_PREDICATES.commandExecutionTime]: {
    label: 'Execution time',
    description: 'The requested or recorded Command execution time.'
  },
  'cs-api.datastream.phenomenon-time': {
    label: 'Phenomenon time',
    description: 'The temporal evidence describing when Datastream phenomena occurred.'
  },
  'cs-api.datastream.result-time': {
    label: 'Result time',
    description: 'The temporal evidence describing when Datastream results became available.'
  },
  'cs-api.property.base-property': {
    label: 'Base property',
    description: 'The property from which this derived Property is defined.'
  },
  [SEMANTIC_PREDICATES.deploymentDeployedSystems]: {
    label: 'Deployed systems',
    description: 'The Systems associated with this Deployment.'
  },
  [SEMANTIC_PREDICATES.samplingFeatureHostedProcedure]: {
    label: 'Hosted procedure',
    description: 'The Procedure associated with this SamplingFeature.'
  },
  [SEMANTIC_PREDICATES.datastreamObservedProperty]: {
    label: 'Observed property',
    description: 'The property measured by this Datastream.',
    relationshipLabel: 'observes'
  }
};

const SEMANTIC_FIELDS: Readonly<Record<string, SemanticField>> = {
  ...MIGRATED_SEMANTIC_FIELDS,
  [SEMANTIC_PREDICATES.processType]: {
    label: 'Resource type',
    description: 'The semantic class of this connected-systems resource.'
  },
  [SEMANTIC_PREDICATES.observationType]: {
    label: 'Observation type',
    description: 'The semantic class of this observation.'
  },
  [SEMANTIC_PREDICATES.feasibilityControlstream]: {
    label: 'Control stream',
    description: 'The ControlStream evaluated by this feasibility result.',
    relationshipLabel: 'for stream'
  },
  'cs-api.feasibility.status': {
    label: 'Status',
    description: 'The current command-feasibility status.'
  },
  'cs-api.feasibility.params': {
    label: 'Parameters',
    description: 'The parameters evaluated by the feasibility request.'
  },
  'cs-api.feasibility.result': {
    label: 'Result',
    description: 'The result returned by the feasibility evaluation.'
  },
  'csapi.observation.datastream': {
    label: 'Datastream',
    description: 'The Datastream that contains this observation.',
    relationshipLabel: 'from stream'
  },
  id: { label: 'Identifier', description: 'The public resource identifier.' },
  uid: { label: 'Unique identifier', description: 'The resource unique identifier.' },
  uniqueId: { label: 'Unique identifier', description: 'The resource unique identifier.' },
  name: { label: 'Name', description: 'The public resource name.' },
  description: { label: 'Description', description: 'The public resource description.' },
  'system@id': { label: 'System', description: 'The referenced System identifier.' },
  'datastream@id': { label: 'Datastream', description: 'The referenced Datastream identifier.' },
  'controlstream@id': { label: 'Control stream', description: 'The referenced ControlStream identifier.' },
  phenomenonTime: { label: 'Phenomenon time', description: 'The time at which the phenomenon occurred.' },
  resultTime: { label: 'Result time', description: 'The time at which the result became available.' },
  eventTime: { label: 'Event time', description: 'The time at which the SystemEvent occurred.' },
  status: { label: 'Status', description: 'The current resource status.' },
  statusCode: { label: 'Status code', description: 'The current Command status code.' },
  obsFormat: { label: 'Observation format', description: 'The media format used for observations.' },
  commandFormat: { label: 'Command format', description: 'The media format used for commands.' },
  definition: { label: 'Definition', description: 'The standards definition for this property.' },
  'result.value': { label: 'Result value', description: 'The value carried by this observation result.' },
  'result.uom': { label: 'Unit of measure', description: 'The unit used by this observation result.' }
};

export function semanticField(predicate: string): SemanticField {
  return SEMANTIC_FIELDS[predicate] ?? {
    label: humanizePredicate(predicate),
    description: 'A semantic field supplied by the connected-systems resource.'
  };
}

export function semanticRelationshipLabel(predicate: string): string {
  const field = semanticField(predicate);
  return field.relationshipLabel ?? field.label.toLocaleLowerCase();
}

function humanizePredicate(predicate: string): string {
  const tail = predicate.split(/[./]/).pop() ?? '';
  const words = tail
    .replace(/([a-z0-9])([A-Z])/g, '$1 $2')
    .replaceAll('-', ' ')
    .replaceAll('_', ' ')
    .trim();
  if (!words) return 'Semantic field';
  return `${words.charAt(0).toLocaleUpperCase()}${words.slice(1)}`;
}
