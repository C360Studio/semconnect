import {
  DATASTREAM_PRESSURE_ID,
  DATASTREAM_TEMP_ID,
  SYSTEM_PUMP_ID,
  SYSTEM_TEMP_PROBE_ID
} from '$lib/data/demoGraph';
import type { RuntimeConfig } from '$lib/config/runtimeConfig';
import { SEMANTIC_PREDICATES, semanticRelationshipLabel } from '$lib/semantics/semanticCatalog';
import type { DemoEntity, DemoFact, DemoRelationship, ResourceKind, TelemetrySample } from '$lib/types/demo';

interface Snapshot {
  entities: DemoEntity[];
  relationships: DemoRelationship[];
  samples: TelemetrySample[];
}

interface FetchError {
  endpoint: string;
  error: string;
}

export interface CsApiSnapshot extends Snapshot {
  errors: FetchError[];
}

type JsonObject = Record<string, unknown>;

export async function fetchCsApiSnapshot(
  config: RuntimeConfig,
  signal?: AbortSignal
): Promise<CsApiSnapshot> {
  const endpoints = [
    ['systems', '/systems'],
    ['datastreams', '/datastreams'],
    ['observations', `/observations?limit=${config.limits.observations}`],
    ['controlstreams', '/controlstreams'],
    ['feasibility', '/feasibility']
  ] as const;

  const results = await Promise.allSettled(
    endpoints.map(async ([name, path]) => {
      const response = await fetch(joinUrl(config.csApiBaseUrl, path), {
        headers: { Accept: 'application/json' },
        signal
      });
      if (!response.ok) {
        throw new Error(`${response.status} ${response.statusText}`);
      }
      return [name, await response.json()] as const;
    })
  );

  const payloads = new Map<string, unknown>();
  const errors: FetchError[] = [];

  results.forEach((result, index) => {
    const [name] = endpoints[index];
    if (result.status === 'fulfilled') {
      payloads.set(name, result.value[1]);
    } else {
      errors.push({
        endpoint: name,
        error: result.reason instanceof Error ? result.reason.message : String(result.reason)
      });
    }
  });

  if (payloads.size === 0) {
    throw new Error(`CS API unavailable: ${errors.map((item) => `${item.endpoint} ${item.error}`).join('; ')}`);
  }

  return {
    ...snapshotFromPayloads(payloads),
    errors
  };
}

export async function postObservationSample(
  config: RuntimeConfig,
  sample: TelemetrySample,
  signal?: AbortSignal
): Promise<void> {
  const response = await fetch(
    joinUrl(config.csApiBaseUrl, `/datastreams/${encodeURIComponent(sample.datastreamId)}/observations`),
    {
      method: 'POST',
      headers: {
        Accept: 'application/json',
        'Content-Type': 'application/om+json'
      },
      body: JSON.stringify(observationPayload(sample)),
      signal
    }
  );

  if ([200, 201, 202, 204].includes(response.status)) return;

  const text = await response.text();
  throw new Error(`${response.status} ${response.statusText}${text ? `: ${text.slice(0, 240)}` : ''}`);
}

function snapshotFromPayloads(payloads: Map<string, unknown>): Snapshot {
  const entityMap = new Map<string, DemoEntity>();
  const relationshipMap = new Map<string, DemoRelationship>();
  const samples: TelemetrySample[] = [];

  for (const item of readItems(payloads.get('systems'))) {
    upsertEntity(entityMap, entityFromResource(item, 'system'));
  }

  for (const item of readItems(payloads.get('datastreams'))) {
    const entity = entityFromResource(item, 'datastream');
    upsertEntity(entityMap, entity);

    const systemId = stringValue(item['system@id']) ?? linkHref(item['system@link']) ?? stringValue(item.system);
    if (systemId) {
      addSemanticRelationship(relationshipMap, entity.id, systemId, SEMANTIC_PREDICATES.datastreamProducedBy);
    }

    const observedPropertyIds = observedProperties(item);
    for (const property of observedPropertyIds) {
      upsertEntity(entityMap, {
        id: property.id,
        kind: 'property',
        label: property.label,
        summary: 'ObservableProperty linked from a CS API Datastream.',
        status: 'active',
        updatedAt: nowIso(),
        facts: [
          { predicate: SEMANTIC_PREDICATES.processType, object: 'sosa:ObservableProperty', source: 'cs-api' },
          { predicate: 'definition', object: property.definition, source: 'cs-api' }
        ]
      });
      addSemanticRelationship(
        relationshipMap,
        entity.id,
        property.id,
        SEMANTIC_PREDICATES.datastreamObservedProperty
      );
    }
  }

  for (const item of readItems(payloads.get('observations'))) {
    const entity = entityFromResource(item, 'observation');
    upsertEntity(entityMap, entity);

    const datastreamId =
      stringValue(item['datastream@id']) ??
      linkHref(item['datastream@link']) ??
      stringValue(item.datastream) ??
      stringValue(item.datastreamId);
    if (datastreamId) {
      addRelationship(relationshipMap, entity.id, datastreamId, 'csapi.observation.datastream', 'from stream');
    }

    const sample = sampleFromObservation(item, entity.id, datastreamId);
    if (sample) samples.push(sample);
  }

  for (const item of readItems(payloads.get('controlstreams'))) {
    const entity = entityFromResource(item, 'controlstream');
    upsertEntity(entityMap, entity);

    const systemId = stringValue(item['system@id']) ?? linkHref(item['system@link']) ?? stringValue(item.system);
    if (systemId) {
      addSemanticRelationship(relationshipMap, entity.id, systemId, SEMANTIC_PREDICATES.controlstreamControlsSystem);
    }
  }

  for (const item of readItems(payloads.get('feasibility'))) {
    const entity = entityFromResource(item, 'feasibility');
    upsertEntity(entityMap, entity);

    const controlstreamId =
      stringValue(item['controlstream@id']) ??
      linkHref(item['controlstream@link']) ??
      stringValue(item.controlstream);
    if (controlstreamId) {
      addSemanticRelationship(relationshipMap, entity.id, controlstreamId, SEMANTIC_PREDICATES.feasibilityControlstream);
    }
  }

  return {
    entities: [...entityMap.values()],
    relationships: [...relationshipMap.values()],
    samples: samples.sort((a, b) => Date.parse(b.resultTime) - Date.parse(a.resultTime))
  };
}

function entityFromResource(item: JsonObject, kind: ResourceKind): DemoEntity {
  const id = resourceId(item, kind);
  const label = resourceLabel(item, id);
  const updatedAt = stringValue(item.resultTime) ?? stringValue(item.phenomenonTime) ?? stringValue(item.time) ?? nowIso();

  return {
    id,
    kind,
    label,
    summary: resourceSummary(item, kind),
    status: resourceStatus(item, kind),
    updatedAt,
    facts: factsFromResource(item, kind)
  };
}

function resourceId(item: JsonObject, kind: ResourceKind): string {
  return (
    stringValue(item.id) ??
    stringValue(item['@id']) ??
    stringValue(item.uid) ??
    stringValue(item.uniqueId) ??
    `${kind}.${Math.random().toString(36).slice(2)}`
  );
}

function resourceLabel(item: JsonObject, id: string): string {
  return (
    stringValue(item.name) ??
    stringValue(item.title) ??
    stringValue(item.label) ??
    stringValue(item.message) ??
    id.split('.').slice(-2).join(' ')
  );
}

function resourceSummary(item: JsonObject, kind: ResourceKind): string {
  return (
    stringValue(item.description) ??
    stringValue(item.message) ??
    `${kind} resource loaded from the Connected Systems API.`
  );
}

function resourceStatus(item: JsonObject, kind: ResourceKind): DemoEntity['status'] {
  const status = `${stringValue(item.status) ?? stringValue(item.statusCode) ?? ''}`.toLowerCase();
  if (status.includes('warn') || status.includes('error') || status.includes('failed')) return 'warning';
  if (status.includes('ready') || kind === 'feasibility' || kind === 'controlstream') return 'ready';
  if (kind === 'observation') return 'nominal';
  return 'active';
}

function factsFromResource(item: JsonObject, kind: ResourceKind): DemoFact[] {
  const typePredicate = kind === 'observation'
    ? SEMANTIC_PREDICATES.observationType
    : SEMANTIC_PREDICATES.processType;
  const facts: DemoFact[] = [{ predicate: typePredicate, object: rdfType(kind), source: 'cs-api' }];
  for (const key of [
    'id',
    'uid',
    'uniqueId',
    'name',
    'description',
    'system@id',
    'datastream@id',
    'controlstream@id',
    'phenomenonTime',
    'resultTime',
    'eventTime',
    'status',
    'statusCode',
    'obsFormat',
    'commandFormat'
  ]) {
    const value = item[key];
    if (value == null || typeof value === 'object') continue;
    facts.push({ predicate: key, object: String(value), source: 'cs-api' });
  }
  return facts.slice(0, 12);
}

function rdfType(kind: ResourceKind): string {
  switch (kind) {
    case 'system':
      return 'ssn:System';
    case 'datastream':
      return 'csapi:Datastream';
    case 'observation':
      return 'om:Observation';
    case 'controlstream':
      return 'csapi:ControlStream';
    case 'feasibility':
      return 'csapi:Feasibility';
    case 'property':
      return 'sosa:ObservableProperty';
  }
}

function readItems(payload: unknown): JsonObject[] {
  if (Array.isArray(payload)) return payload.filter(isObject);
  if (!isObject(payload)) return [];

  for (const key of ['items', 'systems', 'datastreams', 'observations', 'controlstreams', 'feasibility']) {
    const value = payload[key];
    if (Array.isArray(value)) return value.filter(isObject);
  }

  if (Array.isArray(payload.features)) {
    return payload.features.filter(isObject).map((feature) => {
      const properties = isObject(feature.properties) ? feature.properties : {};
      return { ...properties, id: stringValue(feature.id) ?? stringValue(properties.id), geometry: feature.geometry };
    });
  }

  return [];
}

function observedProperties(item: JsonObject): Array<{ id: string; label: string; definition: string }> {
  const result: Array<{ id: string; label: string; definition: string }> = [];
  const observedProperty = item.observedProperty;
  if (typeof observedProperty === 'string') {
    result.push({ id: observedProperty, label: observedProperty.split('/').pop() ?? observedProperty, definition: observedProperty });
  } else if (isObject(observedProperty)) {
    const definition = stringValue(observedProperty.definition) ?? stringValue(observedProperty.id) ?? 'observed-property';
    result.push({
      id: stringValue(observedProperty.id) ?? definition,
      label: stringValue(observedProperty.name) ?? definition.split('/').pop() ?? definition,
      definition
    });
  }

  if (Array.isArray(item.observedProperties)) {
    for (const entry of item.observedProperties) {
      if (typeof entry === 'string') {
        result.push({ id: entry, label: entry.split('/').pop() ?? entry, definition: entry });
      } else if (isObject(entry)) {
        const definition = stringValue(entry.definition) ?? stringValue(entry.id) ?? 'observed-property';
        result.push({
          id: stringValue(entry.id) ?? definition,
          label: stringValue(entry.name) ?? definition.split('/').pop() ?? definition,
          definition
        });
      }
    }
  }

  return dedupeById(result);
}

function sampleFromObservation(
  item: JsonObject,
  id: string,
  datastreamId: string | undefined
): TelemetrySample | null {
  const result = isObject(item.result) ? item.result : item;
  const value = numberValue(result.value) ?? numberValue(result.result) ?? numberValue(item.value);
  if (value == null) return null;

  const resultTime = stringValue(item.resultTime) ?? stringValue(item.phenomenonTime) ?? nowIso();
  return {
    id,
    datastreamId: datastreamId ?? 'unknown.datastream',
    observedProperty: stringValue(item.observedProperty) ?? stringValue(item.name) ?? 'observation',
    value,
    unit: stringValue(result.uom) ?? stringValue(result.unit) ?? '',
    quality: 'good',
    phenomenonTime: stringValue(item.phenomenonTime) ?? resultTime,
    resultTime
  };
}

function observationPayload(sample: TelemetrySample): JsonObject {
  return {
    id: sample.id,
    procedure: procedureForSample(sample),
    observedProperty: sample.observedProperty,
    phenomenonTime: sample.phenomenonTime,
    resultTime: sample.resultTime,
    result: {
      value: sample.value,
      uom: sample.unit,
      quality: sample.quality
    }
  };
}

function procedureForSample(sample: TelemetrySample): string {
  if (sample.datastreamId === DATASTREAM_TEMP_ID) return SYSTEM_TEMP_PROBE_ID;
  if (sample.datastreamId === DATASTREAM_PRESSURE_ID) return SYSTEM_PUMP_ID;
  return sample.datastreamId;
}

function addRelationship(
  relationships: Map<string, DemoRelationship>,
  sourceId: string,
  targetId: string,
  predicate: string,
  label: string
): void {
  if (!sourceId || !targetId) return;
  relationships.set(`${sourceId}:${predicate}:${targetId}`, {
    id: `${sourceId}:${predicate}:${targetId}`,
    sourceId,
    targetId,
    predicate,
    label
  });
}

function addSemanticRelationship(
  relationships: Map<string, DemoRelationship>,
  sourceId: string,
  targetId: string,
  predicate: string
): void {
  addRelationship(relationships, sourceId, targetId, predicate, semanticRelationshipLabel(predicate));
}

function upsertEntity(entities: Map<string, DemoEntity>, entity: DemoEntity): void {
  const previous = entities.get(entity.id);
  if (!previous) {
    entities.set(entity.id, entity);
    return;
  }
  entities.set(entity.id, {
    ...previous,
    ...entity,
    facts: dedupeFacts([...previous.facts, ...entity.facts])
  });
}

function joinUrl(base: string, path: string): string {
  if (!base) return path;
  return `${base}${path.startsWith('/') ? path : `/${path}`}`;
}

function linkHref(value: unknown): string | undefined {
  if (isObject(value)) return stringValue(value.href);
  return undefined;
}

function stringValue(value: unknown): string | undefined {
  return typeof value === 'string' && value.trim() ? value : undefined;
}

function numberValue(value: unknown): number | undefined {
  if (typeof value === 'number' && Number.isFinite(value)) return value;
  if (typeof value === 'string' && value.trim() && Number.isFinite(Number(value))) return Number(value);
  return undefined;
}

function isObject(value: unknown): value is JsonObject {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function nowIso(): string {
  return new Date().toISOString();
}

function dedupeById<T extends { id: string }>(items: T[]): T[] {
  return [...new Map(items.map((item) => [item.id, item])).values()];
}

function dedupeFacts(facts: DemoFact[]): DemoFact[] {
  return [...new Map(facts.map((fact) => [`${fact.predicate}:${fact.object}`, fact])).values()];
}
