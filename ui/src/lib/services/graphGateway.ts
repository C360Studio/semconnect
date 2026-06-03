import type { RuntimeConfig } from '$lib/config/runtimeConfig';
import type { DemoEntity, DemoFact, DemoRelationship, ResourceKind, SearchResult } from '$lib/types/demo';

interface BackendTriple {
  subject: string;
  predicate: string;
  object: unknown;
}

interface BackendEntity {
  id: string;
  triples: BackendTriple[];
}

interface GlobalSearchResponse {
  entities: BackendEntity[];
  community_summaries?: Array<{
    communityId: string;
    text: string;
    keywords: string[];
  }>;
  relationships?: Array<{
    from: string;
    to: string;
    predicate: string;
  }>;
  count?: number;
  duration_ms?: number;
}

interface GraphQLResponse<T> {
  data?: T;
  errors?: Array<{ message: string }>;
  extensions?: Record<string, unknown>;
}

export interface GraphSearchSnapshot {
  entities: DemoEntity[];
  relationships: DemoRelationship[];
  searchResult: SearchResult;
}

export async function searchGraphGateway(
  config: RuntimeConfig,
  query: string,
  signal?: AbortSignal
): Promise<GraphSearchSnapshot> {
  const gqlQuery = `
    query GlobalSearch($query: String!, $level: Int, $maxCommunities: Int) {
      globalSearch(query: $query, level: $level, maxCommunities: $maxCommunities) {
        entities {
          id
          triples {
            subject
            predicate
            object
          }
        }
        community_summaries {
          communityId
          text
          keywords
        }
        count
        duration_ms
      }
    }
  `;

  const response = await executeGraphQL<{ globalSearch: GlobalSearchResponse }>(
    config.graphqlEndpoint,
    gqlQuery,
    { query, level: 0, maxCommunities: 8 },
    signal
  );

  const result = response.data.globalSearch;
  const transformed = transformGraphEntities(result.entities ?? [], []);
  const classification = classificationFromExtensions(response.extensions);

  return {
    ...transformed,
    searchResult: {
      query,
      intent: classification.intent || 'semstreams.globalSearch',
      confidence: classification.confidence || 0.82,
      matchedEntityIds: transformed.entities.map((entity) => entity.id),
      explanation:
        result.community_summaries?.[0]?.text ??
        `SemStreams graph gateway returned ${result.count ?? transformed.entities.length} matching entities.`,
      supportingFacts: [
        `${transformed.entities.length} graph entities returned from globalSearch.`,
        `${transformed.relationships.length} graph relationships derived from returned triples.`,
        `${result.duration_ms ?? 0}ms gateway duration.`
      ]
    }
  };
}

export async function fetchGraphPrefixes(
  config: RuntimeConfig,
  signal?: AbortSignal
): Promise<{ entities: DemoEntity[]; relationships: DemoRelationship[] }> {
  const snapshots = await Promise.all(
    config.graphPrefixes.map(async (prefix) => {
      const gqlQuery = `
        query GetEntitiesByPrefix($prefix: String!, $limit: Int!) {
          entitiesByPrefix(prefix: $prefix, limit: $limit) {
            id
            triples {
              subject
              predicate
              object
            }
          }
        }
      `;

      const response = await executeGraphQL<{ entitiesByPrefix: BackendEntity[] }>(
        config.graphqlEndpoint,
        gqlQuery,
        { prefix, limit: 80 },
        signal
      );
      return transformGraphEntities(response.data.entitiesByPrefix ?? [], []);
    })
  );

  return mergeSnapshots(snapshots);
}

async function executeGraphQL<T>(
  endpoint: string,
  query: string,
  variables: Record<string, unknown>,
  signal?: AbortSignal
): Promise<{ data: T; extensions?: Record<string, unknown> }> {
  const response = await fetch(endpoint, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ query, variables }),
    signal
  });

  if (!response.ok) {
    throw new Error(`GraphQL ${response.status} ${response.statusText}`);
  }

  const payload = (await response.json()) as GraphQLResponse<T>;
  if (payload.errors && payload.errors.length > 0) {
    throw new Error(payload.errors[0].message);
  }
  if (!payload.data) {
    throw new Error('GraphQL response missing data');
  }
  return { data: payload.data, extensions: payload.extensions };
}

function transformGraphEntities(
  backendEntities: BackendEntity[],
  explicitRelationships: Array<{ from: string; to: string; predicate: string }>
): { entities: DemoEntity[]; relationships: DemoRelationship[] } {
  const ids = new Set(backendEntities.map((entity) => entity.id));
  const entities = backendEntities.map((entity) => entityFromBackend(entity, ids));
  const relationshipMap = new Map<string, DemoRelationship>();

  for (const entity of backendEntities) {
    for (const triple of entity.triples) {
      if (typeof triple.object === 'string' && ids.has(triple.object)) {
        const id = `${triple.subject}:${triple.predicate}:${triple.object}`;
        relationshipMap.set(id, {
          id,
          sourceId: triple.subject,
          targetId: triple.object,
          predicate: triple.predicate,
          label: labelFromPredicate(triple.predicate)
        });
      }
    }
  }

  for (const relationship of explicitRelationships) {
    const id = `${relationship.from}:${relationship.predicate}:${relationship.to}`;
    relationshipMap.set(id, {
      id,
      sourceId: relationship.from,
      targetId: relationship.to,
      predicate: relationship.predicate,
      label: labelFromPredicate(relationship.predicate)
    });
  }

  return { entities, relationships: [...relationshipMap.values()] };
}

function entityFromBackend(entity: BackendEntity, entityIds: Set<string>): DemoEntity {
  const facts = entity.triples
    .filter((triple) => !(typeof triple.object === 'string' && entityIds.has(triple.object)))
    .slice(0, 16)
    .map<DemoFact>((triple) => ({
      predicate: triple.predicate,
      object: String(triple.object),
      source: 'semstreams'
    }));

  const label =
    factValue(facts, ['sensorml.process.label', 'name', 'label', 'title', 'dc.terms.title']) ??
    entity.id.split('.').slice(-2).join(' ');

  return {
    id: entity.id,
    kind: inferKind(entity.id, facts),
    label,
    summary:
      factValue(facts, ['sensorml.process.description', 'description', 'summary']) ??
      'Entity returned by the SemStreams graph gateway.',
    status: 'active',
    updatedAt: nowIso(),
    facts
  };
}

function inferKind(id: string, facts: DemoFact[]): ResourceKind {
  const text = `${id} ${facts.map((fact) => `${fact.predicate} ${fact.object}`).join(' ')}`.toLowerCase();
  if (text.includes('controlstream') || text.includes('controlstream')) return 'controlstream';
  if (text.includes('datastream')) return 'datastream';
  if (text.includes('observation')) return 'observation';
  if (text.includes('feasibility')) return 'feasibility';
  if (text.includes('observableproperty') || text.includes('property')) return 'property';
  return 'system';
}

function factValue(facts: DemoFact[], predicates: string[]): string | undefined {
  const lowered = new Set(predicates.map((predicate) => predicate.toLowerCase()));
  return facts.find((fact) => lowered.has(fact.predicate.toLowerCase()))?.object;
}

function labelFromPredicate(predicate: string): string {
  return predicate.split(/[./]/).pop()?.replaceAll('_', ' ') ?? predicate;
}

function classificationFromExtensions(extensions: Record<string, unknown> | undefined): {
  intent: string;
  confidence: number;
} {
  if (!extensions || typeof extensions.classification !== 'object' || extensions.classification === null) {
    return { intent: '', confidence: 0 };
  }
  const classification = extensions.classification as Record<string, unknown>;
  return {
    intent: typeof classification.intent === 'string' ? classification.intent : '',
    confidence: typeof classification.confidence === 'number' ? classification.confidence : 0
  };
}

function mergeSnapshots(
  snapshots: Array<{ entities: DemoEntity[]; relationships: DemoRelationship[] }>
): { entities: DemoEntity[]; relationships: DemoRelationship[] } {
  return {
    entities: [...new Map(snapshots.flatMap((snapshot) => snapshot.entities).map((entity) => [entity.id, entity])).values()],
    relationships: [
      ...new Map(
        snapshots.flatMap((snapshot) => snapshot.relationships).map((relationship) => [relationship.id, relationship])
      ).values()
    ]
  };
}

function nowIso(): string {
  return new Date().toISOString();
}
