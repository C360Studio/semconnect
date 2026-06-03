export type RuntimeMode = 'demo' | 'live' | 'hybrid';

export interface RuntimeConfig {
  mode: RuntimeMode;
  csApiBaseUrl: string;
  graphqlEndpoint: string;
  graphPrefixes: string[];
  pollMs: number;
  limits: {
    observations: number;
  };
  semanticAssist: {
    enabled: boolean;
    semembedEndpoint: string;
    semembedModel: string;
    seminstructEndpoint: string;
    seminstructModel: string;
    similarityThreshold: number;
    maxMatches: number;
  };
}

const CONFIG_PATH = '/semconnect-demo.config.json';

export const DEFAULT_RUNTIME_CONFIG: RuntimeConfig = {
  mode: 'demo',
  csApiBaseUrl: '',
  graphqlEndpoint: '/graphql',
  graphPrefixes: ['c360.demo.water.plant'],
  pollMs: 0,
  limits: {
    observations: 25
  },
  semanticAssist: {
    enabled: false,
    semembedEndpoint: '',
    semembedModel: 'all-MiniLM-L6-v2',
    seminstructEndpoint: '',
    seminstructModel: 'qwen3-0.6b',
    similarityThreshold: 0.62,
    maxMatches: 8
  }
};

export async function loadRuntimeConfig(fetcher: typeof fetch = fetch): Promise<RuntimeConfig> {
  try {
    const response = await fetcher(CONFIG_PATH, { cache: 'no-store' });
    if (!response.ok) {
      return DEFAULT_RUNTIME_CONFIG;
    }
    const payload = (await response.json()) as Partial<RuntimeConfig>;
    return normalizeConfig(payload);
  } catch {
    return DEFAULT_RUNTIME_CONFIG;
  }
}

function normalizeConfig(payload: Partial<RuntimeConfig>): RuntimeConfig {
  const mode = isRuntimeMode(payload.mode) ? payload.mode : DEFAULT_RUNTIME_CONFIG.mode;

  return {
    mode,
    csApiBaseUrl: trimTrailingSlash(payload.csApiBaseUrl ?? DEFAULT_RUNTIME_CONFIG.csApiBaseUrl),
    graphqlEndpoint: payload.graphqlEndpoint || DEFAULT_RUNTIME_CONFIG.graphqlEndpoint,
    graphPrefixes:
      Array.isArray(payload.graphPrefixes) && payload.graphPrefixes.length > 0
        ? payload.graphPrefixes.filter((prefix) => typeof prefix === 'string')
        : DEFAULT_RUNTIME_CONFIG.graphPrefixes,
    pollMs: positiveNumber(payload.pollMs, DEFAULT_RUNTIME_CONFIG.pollMs),
    limits: {
      observations: positiveNumber(
        payload.limits?.observations,
        DEFAULT_RUNTIME_CONFIG.limits.observations
      )
    },
    semanticAssist: {
      enabled: payload.semanticAssist?.enabled === true,
      semembedEndpoint: trimTrailingSlash(
        payload.semanticAssist?.semembedEndpoint ?? DEFAULT_RUNTIME_CONFIG.semanticAssist.semembedEndpoint
      ),
      semembedModel: stringValue(
        payload.semanticAssist?.semembedModel,
        DEFAULT_RUNTIME_CONFIG.semanticAssist.semembedModel
      ),
      seminstructEndpoint: trimTrailingSlash(
        payload.semanticAssist?.seminstructEndpoint ?? DEFAULT_RUNTIME_CONFIG.semanticAssist.seminstructEndpoint
      ),
      seminstructModel: stringValue(
        payload.semanticAssist?.seminstructModel,
        DEFAULT_RUNTIME_CONFIG.semanticAssist.seminstructModel
      ),
      similarityThreshold: positiveNumber(
        payload.semanticAssist?.similarityThreshold,
        DEFAULT_RUNTIME_CONFIG.semanticAssist.similarityThreshold
      ),
      maxMatches: positiveNumber(
        payload.semanticAssist?.maxMatches,
        DEFAULT_RUNTIME_CONFIG.semanticAssist.maxMatches
      )
    }
  };
}

function isRuntimeMode(value: unknown): value is RuntimeMode {
  return value === 'demo' || value === 'live' || value === 'hybrid';
}

function positiveNumber(value: unknown, fallback: number): number {
  return typeof value === 'number' && Number.isFinite(value) && value >= 0 ? value : fallback;
}

function stringValue(value: unknown, fallback: string): string {
  return typeof value === 'string' && value.trim() ? value : fallback;
}

function trimTrailingSlash(value: string): string {
  return value.replace(/\/+$/, '');
}
