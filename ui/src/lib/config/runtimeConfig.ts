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
    }
  };
}

function isRuntimeMode(value: unknown): value is RuntimeMode {
  return value === 'demo' || value === 'live' || value === 'hybrid';
}

function positiveNumber(value: unknown, fallback: number): number {
  return typeof value === 'number' && Number.isFinite(value) && value >= 0 ? value : fallback;
}

function trimTrailingSlash(value: string): string {
  return value.replace(/\/+$/, '');
}
