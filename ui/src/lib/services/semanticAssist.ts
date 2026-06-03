import type { RuntimeConfig } from '$lib/config/runtimeConfig';
import type { DemoEntity } from '$lib/types/demo';

export interface SemanticClassification {
  intent: string;
  confidence: number;
  slots: Record<string, string>;
}

export interface SemanticMatch {
  entityId: string;
  label: string;
  score: number;
}

export interface SemanticAssistResult {
  classification: SemanticClassification | null;
  matches: SemanticMatch[];
  matchedEntityIds: string[];
  supportingFacts: string[];
  errors: string[];
}

interface ChatCompletionResponse {
  choices?: Array<{
    message?: {
      content?: string;
    };
  }>;
}

interface EmbeddingResponse {
  data?: Array<{
    index?: number;
    embedding?: number[];
  }>;
}

const EMPTY_RESULT: SemanticAssistResult = {
  classification: null,
  matches: [],
  matchedEntityIds: [],
  supportingFacts: [],
  errors: []
};

export async function runSemanticAssist(
  config: RuntimeConfig,
  query: string,
  entities: DemoEntity[],
  signal?: AbortSignal
): Promise<SemanticAssistResult> {
  if (!config.semanticAssist.enabled || !query.trim()) {
    return EMPTY_RESULT;
  }

  const [classificationResult, embeddingResult] = await Promise.allSettled([
    classifyQuery(config, query, signal),
    rankEntities(config, query, entities, signal)
  ]);

  const errors: string[] = [];
  let classification: SemanticClassification | null = null;
  let matches: SemanticMatch[] = [];

  if (classificationResult.status === 'fulfilled') {
    classification = classificationResult.value;
  } else {
    errors.push(`seminstruct: ${errorMessage(classificationResult.reason)}`);
  }

  if (embeddingResult.status === 'fulfilled') {
    matches = embeddingResult.value;
  } else {
    errors.push(`semembed: ${errorMessage(embeddingResult.reason)}`);
  }

  const supportingFacts: string[] = [];
  if (classification) {
    supportingFacts.push(
      `Semantic classifier read the query as ${classification.intent} (${Math.round(classification.confidence * 100)}%).`
    );
  }
  if (matches.length > 0) {
    supportingFacts.push(
      `Semantic similarity matched ${matches.length} graph entities above ${config.semanticAssist.similarityThreshold.toFixed(2)}.`
    );
    supportingFacts.push(
      `Top semantic matches: ${matches.slice(0, 3).map((match) => `${match.label} ${match.score.toFixed(2)}`).join(', ')}.`
    );
  }

  return {
    classification,
    matches,
    matchedEntityIds: matches.map((match) => match.entityId),
    supportingFacts,
    errors
  };
}

async function classifyQuery(
  config: RuntimeConfig,
  query: string,
  signal?: AbortSignal
): Promise<SemanticClassification | null> {
  const endpoint = config.semanticAssist.seminstructEndpoint;
  if (!endpoint) return null;

  const response = await fetch(joinOpenAIPath(endpoint, '/chat/completions'), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      model: config.semanticAssist.seminstructModel,
      temperature: 0,
      max_tokens: 220,
      messages: [
        {
          role: 'system',
          content:
            'Classify Connected Systems graph queries. Return strict JSON with intent, confidence, and slots. Do not include prose.'
        },
        {
          role: 'user',
          content: `Query: ${query}\nKnown intents: telemetry.temperature, telemetry.pressure, command.feasibility, system.topology, graph.keyword.`
        }
      ]
    }),
    signal
  });

  if (!response.ok) {
    throw new Error(`${response.status} ${response.statusText}`);
  }

  const payload = (await response.json()) as ChatCompletionResponse;
  const content = payload.choices?.[0]?.message?.content;
  if (!content) return null;

  const parsed = parseJsonObject(content);
  if (!parsed) return null;

  const intent = stringValue(parsed.intent) ?? 'seminstruct.intent';
  const confidence = numberValue(parsed.confidence) ?? 0.72;
  const slots = isRecord(parsed.slots)
    ? Object.fromEntries(
        Object.entries(parsed.slots)
          .filter(([, value]) => typeof value === 'string')
          .map(([key, value]) => [key, value as string])
      )
    : {};

  return { intent, confidence, slots };
}

async function rankEntities(
  config: RuntimeConfig,
  query: string,
  entities: DemoEntity[],
  signal?: AbortSignal
): Promise<SemanticMatch[]> {
  const endpoint = config.semanticAssist.semembedEndpoint;
  if (!endpoint || entities.length === 0) return [];

  const input = [query, ...entities.map(entitySemanticText)];
  const response = await fetch(joinOpenAIPath(endpoint, '/embeddings'), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      model: config.semanticAssist.semembedModel,
      input
    }),
    signal
  });

  if (!response.ok) {
    throw new Error(`${response.status} ${response.statusText}`);
  }

  const payload = (await response.json()) as EmbeddingResponse;
  const vectors = new Map<number, number[]>();
  for (const item of payload.data ?? []) {
    if (typeof item.index === 'number' && Array.isArray(item.embedding)) {
      vectors.set(item.index, item.embedding);
    }
  }

  const queryVector = vectors.get(0);
  if (!queryVector) return [];

  const threshold = config.semanticAssist.similarityThreshold;
  const maxMatches = Math.max(0, Math.floor(config.semanticAssist.maxMatches));

  return entities
    .map((entity, index) => {
      const entityVector = vectors.get(index + 1);
      return entityVector
        ? {
            entityId: entity.id,
            label: entity.label,
            score: cosineSimilarity(queryVector, entityVector)
          }
        : null;
    })
    .filter((match): match is SemanticMatch => match !== null && match.score >= threshold)
    .sort((left, right) => right.score - left.score)
    .slice(0, maxMatches);
}

function entitySemanticText(entity: DemoEntity): string {
  const facts = entity.facts.map((fact) => `${fact.predicate}: ${fact.object}`).join('\n');
  return `${entity.kind}\n${entity.label}\n${entity.summary}\n${facts}`;
}

function cosineSimilarity(left: number[], right: number[]): number {
  const length = Math.min(left.length, right.length);
  let dot = 0;
  let leftMagnitude = 0;
  let rightMagnitude = 0;

  for (let index = 0; index < length; index += 1) {
    dot += left[index] * right[index];
    leftMagnitude += left[index] * left[index];
    rightMagnitude += right[index] * right[index];
  }

  if (leftMagnitude === 0 || rightMagnitude === 0) return 0;
  return dot / (Math.sqrt(leftMagnitude) * Math.sqrt(rightMagnitude));
}

function joinOpenAIPath(base: string, path: string): string {
  if (!base) return path;
  return `${base}${path.startsWith('/') ? path : `/${path}`}`;
}

function parseJsonObject(content: string): Record<string, unknown> | null {
  try {
    const parsed = JSON.parse(content);
    return isRecord(parsed) ? parsed : null;
  } catch {
    const start = content.indexOf('{');
    const end = content.lastIndexOf('}');
    if (start < 0 || end <= start) return null;
    try {
      const parsed = JSON.parse(content.slice(start, end + 1));
      return isRecord(parsed) ? parsed : null;
    } catch {
      return null;
    }
  }
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function stringValue(value: unknown): string | undefined {
  return typeof value === 'string' && value.trim() ? value : undefined;
}

function numberValue(value: unknown): number | undefined {
  return typeof value === 'number' && Number.isFinite(value) ? value : undefined;
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}
