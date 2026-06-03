export const RESOURCE_KINDS = [
  'system',
  'datastream',
  'observation',
  'controlstream',
  'feasibility',
  'property'
] as const;

export type ResourceKind = (typeof RESOURCE_KINDS)[number];

export interface DemoFact {
  predicate: string;
  object: string;
  source: 'cs-api' | 'semstreams' | 'demo';
}

export interface DemoEntity {
  id: string;
  kind: ResourceKind;
  label: string;
  summary: string;
  status: 'nominal' | 'warning' | 'active' | 'ready';
  updatedAt: string;
  facts: DemoFact[];
}

export interface DemoRelationship {
  id: string;
  sourceId: string;
  targetId: string;
  predicate: string;
  label: string;
}

export interface TelemetrySample {
  id: string;
  datastreamId: string;
  observedProperty: string;
  value: number;
  unit: string;
  quality: 'good' | 'watch' | 'poor';
  phenomenonTime: string;
  resultTime: string;
}

export interface SearchResult {
  query: string;
  intent: string;
  confidence: number;
  matchedEntityIds: string[];
  explanation: string;
  supportingFacts: string[];
}

export interface KindCount {
  kind: ResourceKind;
  count: number;
}
