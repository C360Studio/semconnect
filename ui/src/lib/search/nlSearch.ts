import type { DemoEntity, DemoRelationship, SearchResult, TelemetrySample } from '$lib/types/demo';

export function runNaturalLanguageSearch(
  query: string,
  entities: DemoEntity[],
  relationships: DemoRelationship[],
  samples: TelemetrySample[]
): SearchResult {
  const normalized = query.trim().toLowerCase();

  if (!normalized) {
    return {
      query,
      intent: 'empty',
      confidence: 0,
      matchedEntityIds: [],
      explanation: 'No query submitted.',
      supportingFacts: []
    };
  }

  const wantsTemperature = /temp|temperature|thermal|heat/.test(normalized);
  const wantsPressure = /pressure|kpa|discharge/.test(normalized);
  const wantsCommand = /command|control|actuate|valve|feasible|feasibility/.test(normalized);
  const wantsTelemetry = /telemetry|observation|latest|incoming|reading|sample|stream/.test(normalized);
  const wantsSystem = /system|asset|sensor|device|pump|station|probe/.test(normalized);

  if (wantsTemperature) {
    return propertyResult(
      query,
      'observed-property.temperature',
      'Matched temperature language to the ObservableProperty, producing Datastream, hosting System, and recent Observation nodes.',
      'water temperature',
      entities,
      relationships,
      samples
    );
  }

  if (wantsPressure) {
    return propertyResult(
      query,
      'observed-property.pressure',
      'Matched pressure language to the ObservableProperty, producing Datastream, hosting System, and recent Observation nodes.',
      'discharge pressure',
      entities,
      relationships,
      samples
    );
  }

  if (wantsCommand) {
    const ids = entities
      .filter((entity) => {
        const haystack = entityText(entity);
        return entity.kind === 'controlstream' || entity.kind === 'feasibility' || haystack.includes('valve');
      })
      .map((entity) => entity.id);

    return {
      query,
      intent: 'command.feasibility',
      confidence: 0.88,
      matchedEntityIds: expandWithNeighbors(ids, relationships),
      explanation: 'Matched command language to the ControlStream, its controlled property, the target System, and Feasibility status.',
      supportingFacts: [
        'ControlStream exposes application/swe+json command schema evidence.',
        'Feasibility status is stored as graph metadata and linked back to the ControlStream.'
      ]
    };
  }

  if (wantsTelemetry || wantsSystem) {
    const ids = entities
      .filter((entity) => entity.kind === 'system' || entity.kind === 'datastream' || entity.kind === 'observation')
      .slice(0, 12)
      .map((entity) => entity.id);

    return {
      query,
      intent: wantsTelemetry ? 'telemetry.flow' : 'system.topology',
      confidence: 0.76,
      matchedEntityIds: expandWithNeighbors(ids, relationships),
      explanation: 'Matched topology and telemetry language to Systems, Datastreams, and recent Observations.',
      supportingFacts: [
        `${samples.length} observation resources are present in the demo stream.`,
        'System and Datastream relationships are graph edges, not UI-only joins.'
      ]
    };
  }

  const textMatches = entities
    .filter((entity) => entityText(entity).includes(normalized))
    .map((entity) => entity.id);

  return {
    query,
    intent: 'graph.keyword',
    confidence: textMatches.length > 0 ? 0.62 : 0.28,
    matchedEntityIds: expandWithNeighbors(textMatches, relationships),
    explanation:
      textMatches.length > 0
        ? 'Matched literal graph facts and labels.'
        : 'No strong semantic match; showing no focused nodes.',
    supportingFacts: textMatches.slice(0, 3).map((id) => `Matched ${id}`)
  };
}

function propertyResult(
  query: string,
  intent: string,
  explanation: string,
  phrase: string,
  entities: DemoEntity[],
  relationships: DemoRelationship[],
  samples: TelemetrySample[]
): SearchResult {
  const ids = new Set<string>();

  for (const entity of entities) {
    const text = entityText(entity);
    if (text.includes(phrase) || text.includes(phrase.replace('water ', ''))) {
      ids.add(entity.id);
    }
  }

  for (const sample of samples) {
    if (sample.observedProperty.includes(phrase) || phrase.includes(sample.observedProperty)) {
      ids.add(sample.id);
      ids.add(sample.datastreamId);
    }
  }

  return {
    query,
    intent,
    confidence: 0.91,
    matchedEntityIds: expandWithNeighbors([...ids], relationships),
    explanation,
    supportingFacts: [
      'ObservedProperty evidence is carried as graph triples.',
      'Observation nodes link back to their canonical Datastream.',
      'The hosting System is recovered through CS API relationship predicates.'
    ]
  };
}

function entityText(entity: DemoEntity): string {
  const facts = entity.facts.map((fact) => `${fact.predicate} ${fact.object}`).join(' ');
  return `${entity.id} ${entity.kind} ${entity.label} ${entity.summary} ${facts}`.toLowerCase();
}

function expandWithNeighbors(ids: string[], relationships: DemoRelationship[]): string[] {
  const seeds = new Set(ids);
  const focused = new Set(ids);
  for (const relationship of relationships) {
    if (seeds.has(relationship.sourceId)) {
      focused.add(relationship.targetId);
    }
    if (seeds.has(relationship.targetId)) {
      focused.add(relationship.sourceId);
    }
  }
  return [...focused];
}
