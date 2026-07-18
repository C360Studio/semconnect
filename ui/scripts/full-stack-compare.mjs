#!/usr/bin/env node

import { spawn } from 'node:child_process';
import { promises as fs } from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import process from 'node:process';
import { fileURLToPath } from 'node:url';

const SCRIPT_DIR = path.dirname(fileURLToPath(import.meta.url));
const UI_DIR = path.resolve(SCRIPT_DIR, '..');
const REPO_ROOT = path.resolve(UI_DIR, '..');
const DEMO_PREFIX = 'c360.demo.water.plant';
const GRAPH_PREFIX = DEMO_PREFIX;
const DEFAULT_SEMSTREAMS_DIR = path.resolve(REPO_ROOT, '..', 'semstreams');
const DIGEST_INSTANCE_BYTES = 66;
const ID_PREFIX_TEMPLATES = {
  system_id_prefix: { template: '${DEMO_PREFIX}.system', resolved: `${DEMO_PREFIX}.system` },
  datastream_id_prefix: { template: '${DEMO_PREFIX}.datastream', resolved: `${DEMO_PREFIX}.datastream` },
  controlstream_id_prefix: { template: '${DEMO_PREFIX}.controlstream', resolved: `${DEMO_PREFIX}.controlstream` },
  command_id_prefix: { template: '${DEMO_PREFIX}.command', resolved: `${DEMO_PREFIX}.command` },
  feasibility_id_prefix: { template: '${DEMO_PREFIX}.feasibility', resolved: `${DEMO_PREFIX}.feasibility` },
  schema_artifact_id_prefix: { template: '${DEMO_PREFIX}.schema', resolved: `${DEMO_PREFIX}.schema` }
};

const PROFILE_CONFIG = {
  statistical: {
    serviceUrl: 'http://127.0.0.1:38080',
    graphCandidates: [
      'http://127.0.0.1:38082/graphql',
      'http://127.0.0.1:38080/graph-gateway/graphql'
    ],
    uiAssist: null
  },
  semantic: {
    serviceUrl: 'http://127.0.0.1:38180',
    graphCandidates: [
      'http://127.0.0.1:38182/graphql',
      'http://127.0.0.1:38180/graph-gateway/graphql'
    ],
    uiAssist: {
      semembedEndpoint: 'http://127.0.0.1:38081/v1',
      seminstructEndpoint: 'http://127.0.0.1:38084/v1'
    }
  }
};

const IDS = {
  pump: `${DEMO_PREFIX}.system.pump-alpha`,
  probe: `${DEMO_PREFIX}.system.temp-probe-t17`,
  valve: `${DEMO_PREFIX}.system.valve-controller-v2`,
  tempStream: `${DEMO_PREFIX}.datastream.inlet-temperature`,
  pressureStream: `${DEMO_PREFIX}.datastream.discharge-pressure`,
  control: `${DEMO_PREFIX}.controlstream.valve-position`,
  feasibility: `${DEMO_PREFIX}.feasibility.valve-position-ready`
};

const SEARCH_QUERY = 'latest water temperature telemetry';

main().catch((error) => {
  console.error(`\nfull-stack compare failed: ${error.message}`);
  process.exitCode = 1;
});

async function main() {
  const options = parseArgs(process.argv.slice(2));
  if (options.help) {
    printHelp();
    return;
  }
  if (options.validateIdentities) {
    printIdentityClassifications();
    return;
  }

  await ensureSemstreamsDir(options.semstreamsDir);
  const runDir = await fs.mkdtemp(path.join(tmpRoot(), 'semconnect-full-stack-'));
  const runFiles = await writeRunFiles(runDir, options);
  const profiles = expandProfiles(options.profile);
  const summary = {
    runDir,
    repoRoot: REPO_ROOT,
    semstreamsDir: options.semstreamsDir,
    profiles: {}
  };

  let uiProcess = null;
  try {
    uiProcess = await startUiServer(options.uiPort);

    for (let index = 0; index < profiles.length; index += 1) {
      const profile = profiles[index];
      const keepThisStack = options.keepStack && index === profiles.length - 1;
      summary.profiles[profile] = await runProfile(profile, options, runFiles, runDir);
      if (!keepThisStack) {
        await composeDown(profile, options, runFiles);
      }
    }

    const summaryPath = path.join(runDir, 'summary.json');
    await fs.writeFile(summaryPath, `${JSON.stringify(summary, null, 2)}\n`);
    console.log(`\nsummary written to ${summaryPath}`);
    console.log(JSON.stringify(summary, null, 2));
  } finally {
    if (uiProcess) {
      await stopProcess(uiProcess, 'ui dev server');
    }
    if (!options.keepStack) {
      await composeDown('statistical', options, runFiles).catch(() => {});
      await composeDown('semantic', options, runFiles).catch(() => {});
    }
  }
}

function parseArgs(argv) {
  const options = {
    profile: 'both',
    keepStack: false,
    screenshots: true,
    uiSemanticAssist: false,
    semstreamsDir: process.env.SEMSTREAMS_DIR || DEFAULT_SEMSTREAMS_DIR,
    uiPort: numberFromEnv('SEMFLOW_UI_PORT', 5179),
    csApiPort: numberFromEnv('SEMFLOW_CS_API_PORT', 48080),
    proxyPort: numberFromEnv('SEMFLOW_PROXY_PORT', 48081),
    help: false,
    validateIdentities: false
  };

  for (let i = 0; i < argv.length; i += 1) {
    const arg = argv[i];
    const [key, inlineValue] = arg.split('=', 2);
    const nextValue = () => inlineValue ?? argv[++i];

    switch (key) {
      case '--profile':
        options.profile = nextValue();
        break;
      case '--semstreams-dir':
        options.semstreamsDir = path.resolve(nextValue());
        break;
      case '--ui-port':
        options.uiPort = Number(nextValue());
        break;
      case '--cs-api-port':
        options.csApiPort = Number(nextValue());
        break;
      case '--proxy-port':
        options.proxyPort = Number(nextValue());
        break;
      case '--keep-stack':
        options.keepStack = true;
        break;
      case '--no-screenshots':
        options.screenshots = false;
        break;
      case '--ui-semantic-assist':
        options.uiSemanticAssist = true;
        break;
      case '--help':
      case '-h':
        options.help = true;
        break;
      case '--validate-identities':
        options.validateIdentities = true;
        break;
      default:
        throw new Error(`unknown argument: ${arg}`);
    }
  }

  if (!['statistical', 'semantic', 'both'].includes(options.profile)) {
    throw new Error('--profile must be statistical, semantic, or both');
  }
  if (!Number.isInteger(options.uiPort) || options.uiPort <= 0) {
    throw new Error('--ui-port must be a positive integer');
  }
  if (!Number.isInteger(options.csApiPort) || options.csApiPort <= 0) {
    throw new Error('--cs-api-port must be a positive integer');
  }
  if (!Number.isInteger(options.proxyPort) || options.proxyPort <= 0) {
    throw new Error('--proxy-port must be a positive integer');
  }
  return options;
}

function printHelp() {
  console.log(`Usage: npm run compare:full-stack -- [options]

Options:
  --profile statistical|semantic|both  Stack profile to run. Default: both.
  --semstreams-dir PATH                Sibling semstreams checkout. Default: ../semstreams.
  --ui-port PORT                       Vite dev-server port. Default: 5179.
  --cs-api-port PORT                   semconnect CS API host port. Default: 48080.
  --proxy-port PORT                    Caddy browser/proxy port. Default: 48081.
  --ui-semantic-assist                 Also route UI semembed/seminstruct assist in semantic profile.
  --validate-identities                Validate and classify the six rendered entity-ID prefixes, then exit.
  --no-screenshots                     Skip Playwright screenshots.
  --keep-stack                         Leave the last compose stack running.
  --help                               Show this help.

The runner uses semstreams/docker/compose/tiered.yml, adds semconnect plus a
Caddy browser proxy, and tears the stack down between profiles because the
statistical and semantic tiers share NATS buckets.`);
}

function printIdentityClassifications() {
  const classifications = Object.entries(ID_PREFIX_TEMPLATES).map(([configKey, identity]) =>
    classifyEntityPrefix(configKey, identity)
  );
  const invalid = classifications.filter((classification) => classification.classification !== 'valid-five-part-prefix');

  console.log(JSON.stringify({
    contract: {
      parts: 5,
      segmentPattern: '^[A-Za-z0-9][A-Za-z0-9_-]*$',
      maxEntityBytes: 256,
      reservedInstanceBytes: DIGEST_INSTANCE_BYTES
    },
    classifications
  }, null, 2));

  if (invalid.length > 0) {
    throw new Error(`invalid demo entity-ID prefixes: ${invalid.map((item) => item.configKey).join(', ')}`);
  }
}

function classifyEntityPrefix(configKey, identity) {
  const parts = identity.resolved.split('.');
  const bytes = Buffer.byteLength(identity.resolved, 'utf8');
  const digestEntityBytes = bytes + 1 + DIGEST_INSTANCE_BYTES;
  const valid =
    parts.length === 5 &&
    parts.every((part) => /^[A-Za-z0-9][A-Za-z0-9_-]*$/.test(part)) &&
    digestEntityBytes <= 256;

  return {
    configKey,
    template: identity.template,
    resolved: identity.resolved,
    parts: parts.length,
    prefixBytes: bytes,
    digestEntityBytes,
    classification: valid ? 'valid-five-part-prefix' : 'invalid-five-part-prefix'
  };
}

function expandProfiles(profile) {
  if (profile === 'both') return ['statistical', 'semantic'];
  return [profile];
}

async function ensureSemstreamsDir(dir) {
  const composePath = path.join(dir, 'docker', 'compose', 'tiered.yml');
  try {
    await fs.access(composePath);
  } catch {
    throw new Error(`semstreams tiered compose not found at ${composePath}`);
  }
}

async function writeRunFiles(runDir, options) {
  const configPath = path.join(runDir, 'cs-api.config.json');
  const composePath = path.join(runDir, 'semconnect.cs-api.override.yml');
  const caddyfilePath = path.join(runDir, 'Caddyfile');
  const csApiConfig = {
    nats_url: 'nats://nats:4222',
    bind_address: ':8080',
    log_level: 'info',
    system_id_prefix: ID_PREFIX_TEMPLATES.system_id_prefix.resolved,
    datastream_id_prefix: ID_PREFIX_TEMPLATES.datastream_id_prefix.resolved,
    controlstream_id_prefix: ID_PREFIX_TEMPLATES.controlstream_id_prefix.resolved,
    command_id_prefix: ID_PREFIX_TEMPLATES.command_id_prefix.resolved,
    feasibility_id_prefix: ID_PREFIX_TEMPLATES.feasibility_id_prefix.resolved,
    schema_artifact_id_prefix: ID_PREFIX_TEMPLATES.schema_artifact_id_prefix.resolved
  };

  const compose = `services:
  cs-api-server:
    build:
      context: "${escapeYamlPath(REPO_ROOT)}"
      dockerfile: Dockerfile
    image: semconnect/cs-api-server:demo-compare
    command: ["-config", "/etc/cs-api-server/config.json"]
    depends_on:
      nats:
        condition: service_healthy
    volumes:
      - "${escapeYamlPath(configPath)}:/etc/cs-api-server/config.json:ro"
    ports:
      - "${options.csApiPort}:8080"
    networks:
      - semstreams-tiered-net

  caddy:
    image: caddy:2.8-alpine
    depends_on:
      cs-api-server:
        condition: service_started
    volumes:
      - "${escapeYamlPath(caddyfilePath)}:/etc/caddy/Caddyfile:ro"
    ports:
      - "${options.proxyPort}:8080"
    extra_hosts:
      - "host.docker.internal:host-gateway"
    networks:
      - semstreams-tiered-net
`;

  await fs.writeFile(configPath, `${JSON.stringify(csApiConfig, null, 2)}\n`);
  await fs.writeFile(caddyfilePath, caddyfileForProfile('statistical', options));
  await fs.writeFile(composePath, compose);
  return { configPath, composePath, caddyfilePath };
}

async function writeCaddyfile(runFiles, profile, options) {
  await fs.writeFile(runFiles.caddyfilePath, caddyfileForProfile(profile, options));
}

function caddyfileForProfile(profile, options) {
  const graphUpstream = profile === 'semantic' ? 'semstreams-ml:8080' : 'semstreams:8080';
  const enableAssist = options.uiSemanticAssist && profile === 'semantic';
  const runtimeConfig = {
    mode: 'live',
    csApiBaseUrl: '/cs-api',
    graphqlEndpoint: '/graphql',
    graphPrefixes: [GRAPH_PREFIX],
    pollMs: 0,
    limits: { observations: 25 },
    semanticAssist: {
      enabled: enableAssist,
      semembedEndpoint: '/semembed/v1',
      semembedModel: 'all-MiniLM-L6-v2',
      seminstructEndpoint: '/seminstruct/v1',
      seminstructModel: 'qwen3-0.6b',
      similarityThreshold: 0.62,
      maxMatches: 8
    }
  };

  return `:8080 {
  encode zstd gzip

  handle /health {
    respond "ok" 200
  }

  handle /semconnect-demo.config.json {
    header Content-Type application/json
    respond ${caddyQuote(JSON.stringify(runtimeConfig))} 200
  }

  handle_path /cs-api/* {
    reverse_proxy cs-api-server:8080
  }

  handle /graphql {
    rewrite * /graph-gateway/graphql
    reverse_proxy ${graphUpstream}
  }

  handle_path /semembed/* {
    reverse_proxy semembed:8081
  }

  handle_path /seminstruct/* {
    reverse_proxy seminstruct-fast:8083
  }

  handle {
    reverse_proxy host.docker.internal:${options.uiPort}
  }
}
`;
}

function caddyQuote(value) {
  return `"${value.replaceAll('\\', '\\\\').replaceAll('"', '\\"')}"`;
}

function escapeYamlPath(value) {
  return value.replaceAll('\\', '\\\\').replaceAll('"', '\\"');
}

async function runProfile(profile, options, runFiles, runDir) {
  const profileConfig = PROFILE_CONFIG[profile];
  const proxyBaseUrl = `http://127.0.0.1:${options.proxyPort}`;
  const csApiBaseUrl = `${proxyBaseUrl}/cs-api`;

  console.log(`\n=== ${profile} stack ===`);
  await writeCaddyfile(runFiles, profile, options);
  await composeDown(profile, options, runFiles);
  await runCommand(
    'docker',
    [
      'compose',
      '-f',
      'docker/compose/tiered.yml',
      '-f',
      runFiles.composePath,
      '--profile',
      profile,
      'up',
      '-d',
      '--build',
      '--wait'
    ],
    { cwd: options.semstreamsDir }
  );

  await waitForHttpOk(`${profileConfig.serviceUrl}/readyz`, `${profile} SemStreams readyz`, 180000);
  await waitForHttpOk(`${proxyBaseUrl}/health`, 'Caddy proxy health', 60000);
  await waitForHttpOk(`${csApiBaseUrl}/health`, 'semconnect CS API health', 90000);
  const graphEndpoint = await probeGraphEndpoint([`${proxyBaseUrl}/graphql`], 120000);

  await seedDemo(csApiBaseUrl);
  const csApi = await waitForCsApiData(csApiBaseUrl);
  const graph = await waitForGraphData(graphEndpoint, profile);
  const ui = await runUiCheck({
    profile,
    options,
    runDir,
    appBaseUrl: proxyBaseUrl
  });

  return {
    csApi,
    graph,
    ui
  };
}

async function composeDown(profile, options, runFiles) {
  await runCommand(
    'docker',
    [
      'compose',
      '-f',
      'docker/compose/tiered.yml',
      '-f',
      runFiles.composePath,
      '--profile',
      profile,
      'down',
      '-v',
      '--remove-orphans'
    ],
    { cwd: options.semstreamsDir, allowFailure: true }
  );
}

async function startUiServer(port) {
  console.log(`\nstarting SvelteKit dev server on http://127.0.0.1:${port}`);
  const child = spawn('npm', ['run', 'dev', '--', '--host', '127.0.0.1', '--port', String(port)], {
    cwd: UI_DIR,
    env: { ...process.env, FORCE_COLOR: '1' },
    stdio: ['ignore', 'pipe', 'pipe']
  });
  child.stdout.on('data', (chunk) => process.stdout.write(prefixLines('[ui] ', chunk)));
  child.stderr.on('data', (chunk) => process.stderr.write(prefixLines('[ui] ', chunk)));
  await waitForHttpOk(`http://127.0.0.1:${port}/`, 'SvelteKit UI', 60000);
  return child;
}

async function stopProcess(child, label) {
  if (child.exitCode !== null || child.signalCode !== null) return;
  child.kill('SIGTERM');
  const exited = await Promise.race([
    new Promise((resolve) => child.once('exit', () => resolve(true))),
    sleep(5000).then(() => false)
  ]);
  if (!exited) {
    child.kill('SIGKILL');
  }
  console.log(`stopped ${label}`);
}

async function seedDemo(baseUrl) {
  console.log('seeding demo resources through CS API');
  await postJson(`${baseUrl}/systems`, systemFeature('pump-alpha', 'Pump Station Alpha', [
    -93.265,
    44.977,
    251
  ], 'Primary lift station for the demo telemetry graph.'));
  await postJson(`${baseUrl}/systems`, systemFeature('temp-probe-t17', 'Inlet Temperature Probe T17', [
    -93.2648,
    44.9772,
    250
  ], 'Temperature sensor feeding live water telemetry.', IDS.pump));
  await postJson(`${baseUrl}/systems`, systemFeature('valve-controller-v2', 'Valve Controller V2', [
    -93.2652,
    44.9768,
    249
  ], 'Controller exposed as a CS API control stream.', IDS.pump));

  await postJson(`${baseUrl}/datastreams`, {
    id: IDS.tempStream,
    type: 'Datastream',
    name: 'Inlet Temperature',
    description: 'Water temperature telemetry from the inlet probe.',
    system: IDS.probe,
    observedProperty: 'https://qudt.org/vocab/quantitykind/Temperature',
    phenomenonTime: '2026-06-03T14:30:00Z/2026-06-03T14:34:00Z',
    resultTime: '2026-06-03T14:30:05Z/2026-06-03T14:34:05Z'
  });
  await postJson(`${baseUrl}/datastreams`, {
    id: IDS.pressureStream,
    type: 'Datastream',
    name: 'Discharge Pressure',
    description: 'Pressure telemetry used as supporting context.',
    system: IDS.pump,
    observedProperty: 'https://qudt.org/vocab/quantitykind/Pressure',
    phenomenonTime: '2026-06-03T14:30:00Z/2026-06-03T14:34:00Z',
    resultTime: '2026-06-03T14:30:05Z/2026-06-03T14:34:05Z'
  });

  await postObservation(baseUrl, IDS.tempStream, {
    id: `${DEMO_PREFIX}.observation.temp-001`,
    procedure: IDS.probe,
    observedProperty: 'water temperature',
    phenomenonTime: '2026-06-03T14:31:00.000Z',
    resultTime: '2026-06-03T14:31:02.000Z',
    result: { value: 18.7, uom: 'degC' }
  });
  await postObservation(baseUrl, IDS.tempStream, {
    id: `${DEMO_PREFIX}.observation.temp-002`,
    procedure: IDS.probe,
    observedProperty: 'water temperature',
    phenomenonTime: '2026-06-03T14:32:00.000Z',
    resultTime: '2026-06-03T14:32:02.000Z',
    result: { value: 19.1, uom: 'degC' }
  });
  await postObservation(baseUrl, IDS.pressureStream, {
    id: `${DEMO_PREFIX}.observation.pressure-001`,
    procedure: IDS.pump,
    observedProperty: 'discharge pressure',
    phenomenonTime: '2026-06-03T14:32:30.000Z',
    resultTime: '2026-06-03T14:32:32.000Z',
    result: { value: 263.4, uom: 'kPa' }
  });

  await postJson(`${baseUrl}/controlstreams`, {
    id: IDS.control,
    name: 'Valve Position Commands',
    description: 'Command channel for valve position adjustment.',
    'system@id': IDS.valve,
    inputName: 'valve-position',
    issueTime: '2026-06-03T14:33:00Z',
    executionTime: '2026-06-03T14:34:00Z',
    async: false,
    schema: {
      commandFormat: 'application/json',
      parametersSchema: {
        type: 'DataRecord',
        fields: [
          {
            name: 'position',
            type: 'Quantity',
            definition: 'http://sensorml.com/ont/swe/property/ValvePosition',
            label: 'Valve Position'
          }
        ]
      }
    }
  });
  await postJson(`${baseUrl}/feasibility`, {
    id: IDS.feasibility,
    'controlstream@id': IDS.control,
    status: 'ready',
    params: { position: 42 },
    result: { feasible: true, reason: 'within travel limits' }
  });
}

function systemFeature(uid, name, coordinates, description, parentId = '') {
  return {
    type: 'Feature',
    geometry: {
      type: 'Point',
      coordinates
    },
    properties: {
      uid,
      name,
      description,
      ...(parentId ? { 'parent@id': parentId } : {})
    }
  };
}

async function postObservation(baseUrl, datastreamId, body) {
  await postJson(`${baseUrl}/datastreams/${datastreamId}/observations`, body, {
    contentType: 'application/om+json'
  });
}

async function waitForCsApiData(baseUrl) {
  let stable = 0;
  let previous = '';
  let lastCounts = null;
  await waitFor(
    async () => {
      const counts = await csApiCounts(baseUrl);
      const key = JSON.stringify(counts);
      stable = key === previous ? stable + 1 : 0;
      previous = key;
      lastCounts = counts;
      console.log(`cs-api counts ${key}, stable=${stable}`);
      return (
        counts.systems >= 3 &&
        counts.datastreams >= 2 &&
        counts.observations >= 3 &&
        counts.controlstreams >= 1 &&
        counts.feasibility >= 1 &&
        stable >= 2
      );
    },
    { label: 'CS API seeded resource counts', timeoutMs: 90000, intervalMs: 2500 }
  );
  return lastCounts;
}

async function waitForGraphData(endpoint, profile) {
  let stable = 0;
  let previous = -1;
  let lastPrefixCount = 0;
  let lastSearch = null;

  await waitFor(
    async () => {
      const entities = await graphPrefixEntities(endpoint);
      lastPrefixCount = entities.length;
      stable = entities.length === previous ? stable + 1 : 0;
      previous = entities.length;
      console.log(`${profile} graph prefix count ${entities.length}, stable=${stable}`);
      return entities.length >= 6 && stable >= 2;
    },
    { label: `${profile} graph prefix indexing`, timeoutMs: 120000, intervalMs: 3000 }
  );

  try {
    lastSearch = await graphSearch(endpoint, SEARCH_QUERY);
    console.log(
      `${profile} graph globalSearch count ${lastSearch.count}, duration ${lastSearch.durationMs ?? 0}ms`
    );
  } catch (error) {
    lastSearch = { count: 0, durationMs: null, error: error.message };
    console.warn(`${profile} graph globalSearch probe failed: ${error.message}`);
  }

  return {
    endpoint,
    prefixEntityCount: lastPrefixCount,
    search: lastSearch
  };
}

async function runUiCheck({ profile, options, runDir, appBaseUrl }) {
  const { chromium } = await import('@playwright/test');
  const browser = await chromium.launch();
  const page = await browser.newPage({ viewport: { width: 1440, height: 960 } });
  const screenshotPath = path.join(runDir, `${profile}.png`);

  try {
    await page.goto(appBaseUrl, { waitUntil: 'domcontentloaded' });
    await page.waitForFunction(
      () => document.querySelector('[data-testid="graph-surface"]')?.getAttribute('data-ready') === 'true',
      null,
      { timeout: 60000 }
    );
    const initialStreamText = cleanText(await page.getByTestId('stream-state').textContent());
    await page.getByTestId('start-stream').click();
    await page.waitForFunction(
      () => {
        const text = document.querySelector('[data-testid="stream-state"]')?.textContent ?? '';
        const match = text.match(/receiving\s*\/\s*(\d+)\s+observations/);
        return match && Number(match[1]) >= 4;
      },
      null,
      { timeout: 90000 }
    );
    const ingestStreamText = cleanText(await page.getByTestId('stream-state').textContent());
    await page.getByTestId('pause-stream').click();
    await page.waitForFunction(
      () => {
        const text = document.querySelector('[data-testid="stream-state"]')?.textContent ?? '';
        const match = text.match(/paused\s*\/\s*(\d+)\s+observations/);
        return match && Number(match[1]) >= 4;
      },
      null,
      { timeout: 30000 }
    );
    const pausedStreamText = cleanText(await page.getByTestId('stream-state').textContent());

    await page.getByTestId('nl-query').fill(SEARCH_QUERY);
    await page.getByTestId('run-search').click();
    await page.waitForFunction(
      () => (document.querySelector('[data-testid="search-result"]')?.textContent ?? '').length > 0,
      null,
      { timeout: 90000 }
    );
    await page.waitForTimeout(750);

    const statsText = await page.getByTestId('graph-stats').textContent();
    const connectionText = await page.getByTestId('connection-state').textContent();
    const searchText = await page.getByTestId('search-result').textContent();
    const intentText = await page.getByTestId('search-intent').textContent();
    const detailText = await page.getByTestId('entity-detail').textContent();
    const renderedGraph = await page.getByTestId('graph-surface').getAttribute('data-rendered-graph');

    if (options.screenshots) {
      await page.screenshot({ path: screenshotPath, fullPage: true });
    }

    return {
      url: appBaseUrl,
      screenshot: options.screenshots ? screenshotPath : null,
      statsText: cleanText(statsText),
      connectionText: cleanText(connectionText),
      initialStreamText,
      ingestStreamText,
      pausedStreamText,
      intentText: cleanText(intentText),
      searchText: cleanText(searchText),
      selectedDetailText: cleanText(detailText).slice(0, 500),
      renderedGraphNodeCount: countRenderedNodes(renderedGraph),
      uiSemanticAssist: options.uiSemanticAssist && profile === 'semantic'
    };
  } finally {
    await browser.close();
  }
}

function countRenderedNodes(renderedGraphSignature) {
  if (!renderedGraphSignature) return 0;
  const [nodes] = renderedGraphSignature.split('::', 1);
  return nodes.split('|').filter(Boolean).length;
}

async function graphPrefixEntities(endpoint) {
  const response = await graphQL(endpoint, {
    query: `
      query EntitiesByPrefix($prefix: String!, $limit: Int!) {
        entitiesByPrefix(prefix: $prefix, limit: $limit) {
          id
          triples {
            subject
            predicate
            object
          }
        }
      }
    `,
    variables: { prefix: GRAPH_PREFIX, limit: 80 }
  });
  return response.data?.entitiesByPrefix ?? [];
}

async function graphSearch(endpoint, query) {
  const response = await graphQL(endpoint, {
    query: `
      query GlobalSearch($query: String!, $level: Int, $maxCommunities: Int) {
        globalSearch(query: $query, level: $level, maxCommunities: $maxCommunities) {
          entities {
            id
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
    `,
    variables: { query, level: 0, maxCommunities: 8 }
  });
  const result = response.data?.globalSearch ?? {};
  return {
    count: result.count ?? result.entities?.length ?? 0,
    entityCount: result.entities?.length ?? 0,
    relationshipCount: null,
    durationMs: result.duration_ms ?? null,
    summary: result.community_summaries?.[0]?.text ?? ''
  };
}

async function graphQL(endpoint, body) {
  const response = await requestJson(endpoint, {
    method: 'POST',
    body,
    timeoutMs: 60000
  });
  if (response.errors?.length) {
    throw new Error(response.errors.map((error) => error.message).join('; '));
  }
  return response;
}

async function probeGraphEndpoint(candidates, timeoutMs) {
  let chosen = null;
  let lastError = '';
  await waitFor(
    async () => {
      for (const endpoint of candidates) {
        try {
          await graphQL(endpoint, {
            query: '{ entitiesByPrefix(prefix: "", limit: 1) { id } }'
          });
          chosen = endpoint;
          console.log(`using GraphQL endpoint ${endpoint}`);
          return true;
        } catch (error) {
          lastError = `${endpoint}: ${error.message}`;
        }
      }
      console.log(`GraphQL not ready yet: ${lastError}`);
      return false;
    },
    { label: 'SemStreams GraphQL endpoint', timeoutMs, intervalMs: 3000 }
  );
  return chosen;
}

async function csApiCounts(baseUrl) {
  const [systems, datastreams, observations, controlstreams, feasibility] = await Promise.all([
    requestJson(`${baseUrl}/systems`),
    requestJson(`${baseUrl}/datastreams`),
    requestJson(`${baseUrl}/observations?limit=25`),
    requestJson(`${baseUrl}/controlstreams`),
    requestJson(`${baseUrl}/feasibility`)
  ]);
  return {
    systems: readItems(systems).length,
    datastreams: readItems(datastreams).length,
    observations: readItems(observations).length,
    controlstreams: readItems(controlstreams).length,
    feasibility: readItems(feasibility).length
  };
}

function readItems(payload) {
  if (Array.isArray(payload)) return payload;
  if (!payload || typeof payload !== 'object') return [];
  for (const key of ['items', 'systems', 'datastreams', 'observations', 'controlstreams', 'feasibility']) {
    if (Array.isArray(payload[key])) return payload[key];
  }
  return [];
}

async function waitForHttpOk(url, label, timeoutMs) {
  await waitFor(
    async () => {
      try {
        const response = await fetchWithTimeout(url, { timeoutMs: 5000 });
        return response.ok;
      } catch {
        return false;
      }
    },
    { label, timeoutMs, intervalMs: 1500 }
  );
}

async function postJson(url, body, options = {}) {
  try {
    return await requestJson(url, {
      method: 'POST',
      body,
      contentType: options.contentType ?? 'application/json',
      okStatuses: [200, 201, 202],
      timeoutMs: 30000
    });
  } catch (error) {
    if (error.status === 409) {
      console.log(`already seeded: ${url}`);
      return null;
    }
    throw error;
  }
}

async function requestJson(url, options = {}) {
  const response = await fetchWithTimeout(url, {
    method: options.method ?? 'GET',
    headers: {
      Accept: 'application/json',
      ...(options.body ? { 'Content-Type': options.contentType ?? 'application/json' } : {}),
      ...(options.headers ?? {})
    },
    body: options.body ? JSON.stringify(options.body) : undefined,
    timeoutMs: options.timeoutMs ?? 15000
  });
  const text = await response.text();
  const okStatuses = options.okStatuses ?? [200];
  if (!okStatuses.includes(response.status)) {
    const error = new Error(`${response.status} ${response.statusText}: ${text.slice(0, 500)}`);
    error.status = response.status;
    throw error;
  }
  return text ? JSON.parse(text) : null;
}

async function fetchWithTimeout(url, options = {}) {
  const timeoutMs = options.timeoutMs ?? 15000;
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), timeoutMs);
  try {
    return await fetch(url, { ...options, signal: controller.signal });
  } finally {
    clearTimeout(timer);
  }
}

async function waitFor(predicate, { label, timeoutMs, intervalMs }) {
  const started = Date.now();
  let attempts = 0;
  while (Date.now() - started < timeoutMs) {
    attempts += 1;
    if (await predicate()) {
      return;
    }
    await sleep(intervalMs);
  }
  throw new Error(`${label} did not become ready after ${attempts} attempts`);
}

function runCommand(command, args, options = {}) {
  return new Promise((resolve, reject) => {
    const child = spawn(command, args, {
      cwd: options.cwd,
      env: { ...process.env, ...(options.env ?? {}) },
      stdio: ['ignore', 'pipe', 'pipe']
    });
    let stderr = '';
    child.stdout.on('data', (chunk) => {
      if (!options.silent) process.stdout.write(prefixLines(`[${command}] `, chunk));
    });
    child.stderr.on('data', (chunk) => {
      stderr += chunk.toString();
      if (!options.silent) process.stderr.write(prefixLines(`[${command}] `, chunk));
    });
    child.on('error', reject);
    child.on('close', (code) => {
      if (code === 0 || options.allowFailure) {
        resolve({ code });
      } else {
        reject(new Error(`${command} ${args.join(' ')} exited ${code}: ${stderr.slice(-1000)}`));
      }
    });
  });
}

function prefixLines(prefix, chunk) {
  return chunk
    .toString()
    .split(/(?<=\n)/)
    .filter((part) => part.length > 0)
    .map((part) => `${prefix}${part}`)
    .join('');
}

function cleanText(value) {
  return (value ?? '').replace(/\s+/g, ' ').trim();
}

function numberFromEnv(name, fallback) {
  const raw = process.env[name];
  const parsed = raw ? Number(raw) : fallback;
  return Number.isFinite(parsed) ? parsed : fallback;
}

function tmpRoot() {
  return process.platform === 'darwin' ? '/private/tmp/' : `${os.tmpdir()}${path.sep}`;
}

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}
