import type { DemoEntity, DemoRelationship, TelemetrySample } from '$lib/types/demo';

const NOW = '2026-06-03T14:30:00.000Z';

export const SYSTEM_PUMP_ID = 'c360.demo.water.plant.system.pump-alpha';
export const SYSTEM_TEMP_PROBE_ID = 'c360.demo.water.plant.system.temp-probe-t17';
export const SYSTEM_VALVE_ID = 'c360.demo.water.plant.system.valve-controller-v2';

export const PROPERTY_TEMPERATURE_ID = 'c360.demo.water.plant.property.water-temperature';
export const PROPERTY_PRESSURE_ID = 'c360.demo.water.plant.property.discharge-pressure';
export const PROPERTY_VALVE_ID = 'c360.demo.water.plant.property.valve-position';

export const DATASTREAM_TEMP_ID = 'c360.demo.water.plant.datastream.inlet-temperature';
export const DATASTREAM_PRESSURE_ID = 'c360.demo.water.plant.datastream.discharge-pressure';

export const CONTROLSTREAM_VALVE_ID = 'c360.demo.water.plant.controlstream.valve-position';
export const FEASIBILITY_VALVE_ID = 'c360.demo.water.plant.feasibility.valve-position-ready';

export function createBaseEntities(): DemoEntity[] {
  return [
    {
      id: SYSTEM_PUMP_ID,
      kind: 'system',
      label: 'Pump Station Alpha',
      summary: 'Connected Systems API System with hosted telemetry and command surfaces.',
      status: 'nominal',
      updatedAt: NOW,
      facts: [
        { predicate: 'rdf.type', object: 'ssn:System', source: 'cs-api' },
        { predicate: 'sensorml.process.uid', object: 'urn:c360:demo:pump-alpha', source: 'semstreams' },
        { predicate: 'sensorml.process.position', object: 'POINT(-90.0715 29.9511)', source: 'semstreams' }
      ]
    },
    {
      id: SYSTEM_TEMP_PROBE_ID,
      kind: 'system',
      label: 'Temperature Probe T-17',
      summary: 'Subsystem hosted by Pump Station Alpha and producing inlet water temperature.',
      status: 'active',
      updatedAt: NOW,
      facts: [
        { predicate: 'rdf.type', object: 'ssn:System', source: 'cs-api' },
        { predicate: 'sensorml.process.uid', object: 'urn:c360:demo:temp-probe-t17', source: 'semstreams' },
        { predicate: 'sensorml.system.isHostedBy', object: SYSTEM_PUMP_ID, source: 'semstreams' }
      ]
    },
    {
      id: SYSTEM_VALVE_ID,
      kind: 'system',
      label: 'Valve Controller V2',
      summary: 'Control-capable system used by the feasibility check and command stream.',
      status: 'ready',
      updatedAt: NOW,
      facts: [
        { predicate: 'rdf.type', object: 'ssn:System', source: 'cs-api' },
        { predicate: 'sensorml.process.uid', object: 'urn:c360:demo:valve-controller-v2', source: 'semstreams' },
        { predicate: 'sensorml.system.isHostedBy', object: SYSTEM_PUMP_ID, source: 'semstreams' }
      ]
    },
    {
      id: PROPERTY_TEMPERATURE_ID,
      kind: 'property',
      label: 'Water Temperature',
      summary: 'ObservableProperty used by the inlet temperature datastream.',
      status: 'active',
      updatedAt: NOW,
      facts: [
        { predicate: 'rdf.type', object: 'sosa:ObservableProperty', source: 'cs-api' },
        { predicate: 'definition', object: 'https://qudt.org/vocab/quantitykind/Temperature', source: 'demo' }
      ]
    },
    {
      id: PROPERTY_PRESSURE_ID,
      kind: 'property',
      label: 'Discharge Pressure',
      summary: 'ObservableProperty used by the discharge pressure datastream.',
      status: 'active',
      updatedAt: NOW,
      facts: [
        { predicate: 'rdf.type', object: 'sosa:ObservableProperty', source: 'cs-api' },
        { predicate: 'definition', object: 'https://qudt.org/vocab/quantitykind/Pressure', source: 'demo' }
      ]
    },
    {
      id: PROPERTY_VALVE_ID,
      kind: 'property',
      label: 'Valve Position',
      summary: 'ControlledProperty accepted by the valve command stream.',
      status: 'ready',
      updatedAt: NOW,
      facts: [
        { predicate: 'rdf.type', object: 'sosa:ObservableProperty', source: 'cs-api' },
        { predicate: 'definition', object: 'https://example.c360.dev/props/valve-position', source: 'demo' }
      ]
    },
    {
      id: DATASTREAM_TEMP_ID,
      kind: 'datastream',
      label: 'Inlet Water Temperature',
      summary: 'CS API Datastream backed by the temperature probe and SWE scalar values.',
      status: 'active',
      updatedAt: NOW,
      facts: [
        { predicate: 'rdf.type', object: 'csapi:Datastream', source: 'cs-api' },
        { predicate: 'csapi.datastream.observedProperty', object: PROPERTY_TEMPERATURE_ID, source: 'semstreams' },
        { predicate: 'csapi.datastream.obsFormat', object: 'application/om+json', source: 'cs-api' }
      ]
    },
    {
      id: DATASTREAM_PRESSURE_ID,
      kind: 'datastream',
      label: 'Discharge Pressure',
      summary: 'CS API Datastream tied to the pump station discharge sensor.',
      status: 'active',
      updatedAt: NOW,
      facts: [
        { predicate: 'rdf.type', object: 'csapi:Datastream', source: 'cs-api' },
        { predicate: 'csapi.datastream.observedProperty', object: PROPERTY_PRESSURE_ID, source: 'semstreams' },
        { predicate: 'csapi.datastream.obsFormat', object: 'application/om+json', source: 'cs-api' }
      ]
    },
    {
      id: CONTROLSTREAM_VALVE_ID,
      kind: 'controlstream',
      label: 'Valve Position Commands',
      summary: 'Part 2 ControlStream exposing command schema and controlled property evidence.',
      status: 'ready',
      updatedAt: NOW,
      facts: [
        { predicate: 'rdf.type', object: 'csapi:ControlStream', source: 'cs-api' },
        { predicate: 'csapi.controlstream.commandFormat', object: 'application/swe+json', source: 'cs-api' },
        { predicate: 'csapi.controlstream.controlledProperties', object: PROPERTY_VALVE_ID, source: 'semstreams' }
      ]
    },
    {
      id: FEASIBILITY_VALVE_ID,
      kind: 'feasibility',
      label: 'Valve Feasibility Ready',
      summary: 'Read-side feasibility resource linked to the valve control stream.',
      status: 'ready',
      updatedAt: NOW,
      facts: [
        { predicate: 'rdf.type', object: 'csapi:Feasibility', source: 'cs-api' },
        { predicate: 'cs-api.feasibility.status', object: 'ready', source: 'semstreams' },
        { predicate: 'cs-api.feasibility.params', object: '{"targetPosition":42}', source: 'demo' }
      ]
    }
  ];
}

export function createBaseRelationships(): DemoRelationship[] {
  return [
    edge(SYSTEM_TEMP_PROBE_ID, SYSTEM_PUMP_ID, 'sensorml.system.isHostedBy', 'hosted by'),
    edge(SYSTEM_VALVE_ID, SYSTEM_PUMP_ID, 'sensorml.system.isHostedBy', 'hosted by'),
    edge(DATASTREAM_TEMP_ID, SYSTEM_TEMP_PROBE_ID, 'csapi.datastream.producedBy', 'produced by'),
    edge(DATASTREAM_PRESSURE_ID, SYSTEM_PUMP_ID, 'csapi.datastream.producedBy', 'produced by'),
    edge(DATASTREAM_TEMP_ID, PROPERTY_TEMPERATURE_ID, 'csapi.datastream.observedProperty', 'observes'),
    edge(DATASTREAM_PRESSURE_ID, PROPERTY_PRESSURE_ID, 'csapi.datastream.observedProperty', 'observes'),
    edge(CONTROLSTREAM_VALVE_ID, SYSTEM_VALVE_ID, 'csapi.controlstream.controlsSystem', 'controls'),
    edge(CONTROLSTREAM_VALVE_ID, PROPERTY_VALVE_ID, 'csapi.controlstream.controlledProperty', 'controls property'),
    edge(FEASIBILITY_VALVE_ID, CONTROLSTREAM_VALVE_ID, 'csapi.feasibility.controlstream', 'for stream')
  ];
}

export function createInitialSamples(): TelemetrySample[] {
  return [
    sample('seed-003', DATASTREAM_TEMP_ID, 'water temperature', 18.7, 'degC', 'good', 0),
    sample('seed-002', DATASTREAM_PRESSURE_ID, 'discharge pressure', 41.4, 'kPa', 'good', -1),
    sample('seed-001', DATASTREAM_TEMP_ID, 'water temperature', 18.5, 'degC', 'good', -2)
  ];
}

export function observationEntityFromSample(sampleValue: TelemetrySample): DemoEntity {
  return {
    id: sampleValue.id,
    kind: 'observation',
    label: `${sampleValue.value.toFixed(1)} ${sampleValue.unit}`,
    summary: `${sampleValue.observedProperty} sample on ${sampleValue.datastreamId.split('.').pop() ?? 'datastream'}.`,
    status: sampleValue.quality === 'good' ? 'nominal' : 'warning',
    updatedAt: sampleValue.resultTime,
    facts: [
      { predicate: 'rdf.type', object: 'om:Observation', source: 'cs-api' },
      { predicate: 'datastream@id', object: sampleValue.datastreamId, source: 'cs-api' },
      { predicate: 'phenomenonTime', object: sampleValue.phenomenonTime, source: 'cs-api' },
      { predicate: 'resultTime', object: sampleValue.resultTime, source: 'cs-api' },
      { predicate: 'result.value', object: String(sampleValue.value), source: 'demo' },
      { predicate: 'result.uom', object: sampleValue.unit, source: 'demo' }
    ]
  };
}

export function relationshipForSample(sampleValue: TelemetrySample): DemoRelationship {
  return edge(sampleValue.id, sampleValue.datastreamId, 'csapi.observation.datastream', 'from stream');
}

export function createNextSample(sequence: number): TelemetrySample {
  const useTemperature = sequence % 3 !== 0;
  const datastreamId = useTemperature ? DATASTREAM_TEMP_ID : DATASTREAM_PRESSURE_ID;
  const observedProperty = useTemperature ? 'water temperature' : 'discharge pressure';
  const unit = useTemperature ? 'degC' : 'kPa';
  const base = useTemperature ? 18.8 : 41.8;
  const drift = useTemperature ? Math.sin(sequence / 2) * 0.6 : Math.cos(sequence / 3) * 1.8;
  const value = Number((base + drift + sequence * 0.03).toFixed(2));
  const quality = useTemperature && value > 19.4 ? 'watch' : 'good';

  return sample(`obs-${String(sequence).padStart(3, '0')}`, datastreamId, observedProperty, value, unit, quality, sequence);
}

function sample(
  id: string,
  datastreamId: string,
  observedProperty: string,
  value: number,
  unit: string,
  quality: TelemetrySample['quality'],
  offsetSeconds: number
): TelemetrySample {
  const timestamp = new Date(Date.parse(NOW) + offsetSeconds * 1000).toISOString();
  return {
    id: `c360.demo.water.plant.observation.${id}`,
    datastreamId,
    observedProperty,
    value,
    unit,
    quality,
    phenomenonTime: timestamp,
    resultTime: timestamp
  };
}

function edge(sourceId: string, targetId: string, predicate: string, label: string): DemoRelationship {
  return {
    id: `${sourceId}:${predicate}:${targetId}`,
    sourceId,
    targetId,
    predicate,
    label
  };
}
