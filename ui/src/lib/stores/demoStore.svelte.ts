import {
  DEFAULT_RUNTIME_CONFIG,
  loadRuntimeConfig,
  type RuntimeConfig
} from '$lib/config/runtimeConfig';
import {
  createBaseEntities,
  createBaseRelationships,
  createInitialSamples,
  createNextSample,
  observationEntityFromSample,
  relationshipForSample
} from '$lib/data/demoGraph';
import { runNaturalLanguageSearch } from '$lib/search/nlSearch';
import { fetchCsApiSnapshot } from '$lib/services/csApi';
import { fetchGraphPrefixes, searchGraphGateway } from '$lib/services/graphGateway';
import { runSemanticAssist, type SemanticAssistResult } from '$lib/services/semanticAssist';
import {
  RESOURCE_KINDS,
  type DemoEntity,
  type DemoRelationship,
  type ResourceKind,
  type SearchResult,
  type TelemetrySample
} from '$lib/types/demo';

const MAX_OBSERVATIONS = 18;

type ConnectionStatus = 'demo' | 'loading' | 'live' | 'hybrid' | 'error';

export class DemoStore {
  config = $state<RuntimeConfig>(DEFAULT_RUNTIME_CONFIG);
  entities = $state<DemoEntity[]>([]);
  relationships = $state<DemoRelationship[]>([]);
  samples = $state<TelemetrySample[]>([]);
  loading = $state(false);
  searching = $state(false);
  running = $state(false);
  connectionStatus = $state<ConnectionStatus>('demo');
  connectionMessage = $state('Demo fixtures ready.');
  liveErrors = $state<string[]>([]);
  selectedEntityId = $state<string | null>(null);
  visibleKinds = $state<Set<ResourceKind>>(new Set(RESOURCE_KINDS));
  searchQuery = $state('');
  searchResult = $state<SearchResult | null>(null);
  private timer: ReturnType<typeof setInterval> | null = null;
  private pollTimer: ReturnType<typeof setInterval> | null = null;
  private abortController: AbortController | null = null;
  private sequence = 4;

  constructor() {
    this.reset();
  }

  get selectedEntity(): DemoEntity | null {
    if (!this.selectedEntityId) return null;
    return this.entities.find((entity) => entity.id === this.selectedEntityId) ?? null;
  }

  get visibleEntities(): DemoEntity[] {
    return this.entities.filter((entity) => this.visibleKinds.has(entity.kind));
  }

  get visibleRelationships(): DemoRelationship[] {
    const visibleIds = new Set(this.visibleEntities.map((entity) => entity.id));
    return this.relationships.filter(
      (relationship) => visibleIds.has(relationship.sourceId) && visibleIds.has(relationship.targetId)
    );
  }

  get kindCounts(): Map<ResourceKind, number> {
    const counts = new Map<ResourceKind, number>();
    for (const kind of RESOURCE_KINDS) counts.set(kind, 0);
    for (const entity of this.entities) {
      counts.set(entity.kind, (counts.get(entity.kind) ?? 0) + 1);
    }
    return counts;
  }

  get focusedEntityIds(): Set<string> {
    return new Set(this.searchResult?.matchedEntityIds ?? []);
  }

  get observationCount(): number {
    return this.entities.filter((entity) => entity.kind === 'observation').length;
  }

  async initialize(): Promise<void> {
    this.config = await loadRuntimeConfig();
    this.liveErrors = [];

    if (this.config.mode === 'demo') {
      this.connectionStatus = 'demo';
      this.connectionMessage = 'Demo fixtures ready.';
      return;
    }

    this.connectionStatus = 'loading';
    this.connectionMessage = 'Connecting to CS API and SemStreams graph gateway.';

    try {
      await this.refreshLiveData();
      this.startPolling();
    } catch (error) {
      this.handleLiveFailure(error);
      if (this.config.mode === 'hybrid') {
        this.reset();
        this.connectionStatus = 'hybrid';
        this.connectionMessage = 'Live services unavailable; showing demo graph.';
      }
    }
  }

  async refreshLiveData(): Promise<void> {
    if (this.config.mode === 'demo') {
      this.reset();
      return;
    }

    this.loading = true;
    this.connectionStatus = 'loading';
    this.connectionMessage = 'Refreshing CS API resources and graph triples.';
    this.abortController?.abort();
    const controller = new AbortController();
    this.abortController = controller;

    try {
      const [csApiResult, graphResult] = await Promise.allSettled([
        fetchCsApiSnapshot(this.config, controller.signal),
        fetchGraphPrefixes(this.config, controller.signal)
      ]);

      const liveErrors: string[] = [];
      const entities: DemoEntity[] = [];
      const relationships: DemoRelationship[] = [];
      let samples: TelemetrySample[] = [];

      if (csApiResult.status === 'fulfilled') {
        entities.push(...csApiResult.value.entities);
        relationships.push(...csApiResult.value.relationships);
        samples = csApiResult.value.samples;
        liveErrors.push(...csApiResult.value.errors.map((item) => `${item.endpoint}: ${item.error}`));
      } else {
        liveErrors.push(`cs-api: ${errorMessage(csApiResult.reason)}`);
      }

      if (graphResult.status === 'fulfilled') {
        entities.push(...graphResult.value.entities);
        relationships.push(...graphResult.value.relationships);
      } else {
        liveErrors.push(`semstreams: ${errorMessage(graphResult.reason)}`);
      }

      if (entities.length === 0 && relationships.length === 0) {
        throw new Error(liveErrors.join('; ') || 'No live graph data returned.');
      }

      this.entities = mergeEntities(entities);
      this.relationships = mergeRelationships(relationships);
      this.samples = samples.slice(0, observationLimit(this.config));
      this.liveErrors = liveErrors;
      this.connectionStatus = liveErrors.length > 0 ? 'hybrid' : 'live';
      this.connectionMessage = liveErrors.length > 0
        ? `Loaded live graph with ${liveErrors.length} partial integration warning${liveErrors.length === 1 ? '' : 's'}.`
        : 'Live CS API and SemStreams graph gateway connected.';

      if (this.selectedEntityId && !this.entities.some((entity) => entity.id === this.selectedEntityId)) {
        this.selectedEntityId = null;
      }
    } finally {
      if (this.abortController === controller) {
        this.abortController = null;
      }
      this.loading = false;
    }
  }

  reset(): void {
    const samples = createInitialSamples();
    this.samples = [...samples];
    this.entities = [...createBaseEntities(), ...samples.map(observationEntityFromSample)];
    this.relationships = [...createBaseRelationships(), ...samples.map(relationshipForSample)];
    this.running = false;
    this.selectedEntityId = null;
    this.visibleKinds = new Set(RESOURCE_KINDS);
    this.searchQuery = '';
    this.searchResult = null;
    this.loading = false;
    this.searching = false;
    this.connectionStatus = this.config.mode === 'demo' ? 'demo' : 'hybrid';
    this.connectionMessage = this.config.mode === 'demo'
      ? 'Demo fixtures ready.'
      : 'Demo fixtures loaded while live integration is unavailable.';
    this.liveErrors = [];
    this.sequence = 4;
    this.stop();
    this.stopPolling();
  }

  start(): void {
    if (this.running) return;
    this.running = true;
    this.injectSample();
    this.timer = setInterval(() => this.injectSample(), 1800);
  }

  stop(): void {
    if (this.timer) {
      clearInterval(this.timer);
      this.timer = null;
    }
    this.running = false;
  }

  dispose(): void {
    this.stop();
    this.stopPolling();
    this.abortController?.abort();
    this.abortController = null;
  }

  injectSample(): void {
    const sample = createNextSample(this.sequence);
    this.sequence += 1;

    const entity = observationEntityFromSample(sample);
    const relationship = relationshipForSample(sample);

    this.samples = [sample, ...this.samples].slice(0, MAX_OBSERVATIONS);
    this.entities = pruneObservations([entity, ...this.entities.filter((item) => item.id !== entity.id)]);
    this.relationships = pruneObservationRelationships([
      relationship,
      ...this.relationships.filter((item) => item.id !== relationship.id)
    ], this.entities);

    this.selectedEntityId = entity.id;
    if (this.searchResult) {
      this.searchResult = runNaturalLanguageSearch(
        this.searchResult.query,
        this.entities,
        this.relationships,
        this.samples
      );
    }
  }

  selectEntity(entityId: string | null): void {
    this.selectedEntityId = entityId;
  }

  toggleKind(kind: ResourceKind): void {
    if (this.visibleKinds.size === 1 && this.visibleKinds.has(kind)) {
      this.showAllKinds();
      return;
    }
    this.visibleKinds = new Set<ResourceKind>([kind]);
    this.keepSelectionVisible();
  }

  showAllKinds(): void {
    this.visibleKinds = new Set(RESOURCE_KINDS);
  }

  showTelemetryKinds(): void {
    this.visibleKinds = new Set<ResourceKind>(['system', 'datastream', 'observation', 'property']);
    this.keepSelectionVisible();
  }

  async runSearch(query = this.searchQuery): Promise<void> {
    this.searchQuery = query;
    this.searching = true;

    try {
      let result: SearchResult | null = null;

      if (this.config.mode !== 'demo') {
        try {
          const graphSnapshot = await searchGraphGateway(this.config, query);
          this.entities = mergeEntities([...this.entities, ...graphSnapshot.entities]);
          this.relationships = mergeRelationships([...this.relationships, ...graphSnapshot.relationships]);
          result = graphSnapshot.searchResult;
          this.connectionMessage = 'SemStreams graph gateway answered the semantic search.';
        } catch (error) {
          const message = `semantic search: ${errorMessage(error)}`;
          this.liveErrors = [...this.liveErrors.filter((item) => !item.startsWith('semantic search:')), message];
          this.connectionStatus = this.config.mode === 'hybrid' ? 'hybrid' : 'error';
          this.connectionMessage = 'SemStreams search unavailable; using local graph search over loaded resources.';
        }
      }

      const semanticAssist = await runSemanticAssist(this.config, query, this.entities);
      if (semanticAssist.errors.length > 0) {
        this.liveErrors = [
          ...this.liveErrors.filter(
            (item) => !item.startsWith('semembed:') && !item.startsWith('seminstruct:')
          ),
          ...semanticAssist.errors
        ];
      }

      this.searchResult = applySemanticAssist(
        result ?? runNaturalLanguageSearch(query, this.entities, this.relationships, this.samples),
        semanticAssist
      );
      this.selectedEntityId = this.searchResult.matchedEntityIds[0] ?? null;

      if (semanticAssist.supportingFacts.length > 0) {
        this.connectionMessage = 'SemStreams search answered with semembed and seminstruct assist.';
      }
    } finally {
      this.searching = false;
    }
  }

  private startPolling(): void {
    this.stopPolling();
    if (this.config.pollMs <= 0 || this.config.mode === 'demo') return;
    this.pollTimer = setInterval(() => {
      void this.refreshLiveData();
    }, this.config.pollMs);
  }

  private stopPolling(): void {
    if (this.pollTimer) {
      clearInterval(this.pollTimer);
      this.pollTimer = null;
    }
  }

  private handleLiveFailure(error: unknown): void {
    const message = errorMessage(error);
    this.liveErrors = [message];
    this.connectionStatus = 'error';
    this.connectionMessage = `Live integration failed: ${message}`;
  }

  private keepSelectionVisible(): void {
    if (this.selectedEntityId && this.visibleEntities.some((entity) => entity.id === this.selectedEntityId)) {
      return;
    }
    this.selectedEntityId = this.visibleEntities[0]?.id ?? null;
  }
}

function applySemanticAssist(result: SearchResult, semanticAssist: SemanticAssistResult): SearchResult {
  if (!semanticAssist.classification && semanticAssist.matchedEntityIds.length === 0) {
    return result;
  }

  const matchedEntityIds = [
    ...new Set([...result.matchedEntityIds, ...semanticAssist.matchedEntityIds])
  ];

  return {
    ...result,
    intent: semanticAssist.classification?.intent ?? result.intent,
    confidence: Math.max(result.confidence, semanticAssist.classification?.confidence ?? 0),
    matchedEntityIds,
    explanation: semanticAssist.classification
      ? `${result.explanation} Seminstruct classified the natural-language intent before semembed expanded the graph focus.`
      : result.explanation,
    supportingFacts: [...semanticAssist.supportingFacts, ...result.supportingFacts].slice(0, 8)
  };
}

function mergeEntities(entities: DemoEntity[]): DemoEntity[] {
  const merged = new Map<string, DemoEntity>();
  for (const entity of entities) {
    const previous = merged.get(entity.id);
    if (!previous) {
      merged.set(entity.id, entity);
      continue;
    }
    merged.set(entity.id, {
      ...previous,
      ...entity,
      facts: [
        ...new Map([...previous.facts, ...entity.facts].map((fact) => [`${fact.source}:${fact.predicate}:${fact.object}`, fact])).values()
      ]
    });
  }
  return [...merged.values()];
}

function mergeRelationships(relationships: DemoRelationship[]): DemoRelationship[] {
  return [...new Map(relationships.map((relationship) => [relationship.id, relationship])).values()];
}

function observationLimit(config: RuntimeConfig): number {
  return config.limits.observations > 0 ? config.limits.observations : MAX_OBSERVATIONS;
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

function pruneObservations(entities: DemoEntity[]): DemoEntity[] {
  const observations = entities.filter((entity) => entity.kind === 'observation');
  const keepObservationIds = new Set(observations.slice(0, MAX_OBSERVATIONS).map((entity) => entity.id));
  return entities.filter((entity) => entity.kind !== 'observation' || keepObservationIds.has(entity.id));
}

function pruneObservationRelationships(
  relationships: DemoRelationship[],
  entities: DemoEntity[]
): DemoRelationship[] {
  const ids = new Set(entities.map((entity) => entity.id));
  return relationships.filter(
    (relationship) => ids.has(relationship.sourceId) && ids.has(relationship.targetId)
  );
}

export const demoStore = new DemoStore();
