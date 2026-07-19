import { readFileSync } from 'node:fs';
import { expect, test } from '@playwright/test';
import {
  CONTROLSTREAM_VALVE_ID,
  createBaseEntities,
  createBaseRelationships
} from '../src/lib/data/demoGraph';
import {
  MIGRATED_SEMANTIC_FIELDS,
  SEMANTIC_PREDICATES,
  semanticField
} from '../src/lib/semantics/semanticCatalog';

const EXPECTED_MIGRATED_LABELS: Readonly<Record<string, string>> = {
  'oms.observation.has-feature-of-interest': 'Feature of interest',
  'oms.observation.has-simple-result': 'Simple result',
  'oms.observation.observed-property': 'Observed property',
  'oms.observation.phenomenon-time': 'Phenomenon time',
  'oms.observation.result-time': 'Result time',
  'oms.observation.used-procedure': 'Procedure',
  'sensorml.component.is-hosted-by': 'Hosted by',
  'sensorml.process.attached-to': 'Attached to',
  'sensorml.process.has-sub-system': 'Subsystem',
  'sensorml.process.used-procedure': 'Procedure',
  'csapi.command.part-of-control-stream': 'Control stream',
  'csapi.controlstream.command-schema': 'Command schema',
  'csapi.controlstream.controls-system': 'Controlled system',
  'csapi.datastream.phenomenon-time-range': 'Phenomenon time range',
  'csapi.datastream.produced-by': 'Producing system',
  'csapi.datastream.result-schema': 'Result schema',
  'csapi.datastream.result-time-range': 'Result time range',
  'csapi.datastream.result-type': 'Result type',
  'csapi.systemevent.for-system': 'System',
  'cs-api.controlstream.input-name': 'Input name',
  'cs-api.controlstream.command-format': 'Command format',
  'cs-api.controlstream.controlled-properties': 'Controlled properties',
  'cs-api.controlstream.issue-time': 'Issue time',
  'cs-api.controlstream.execution-time': 'Execution time',
  'cs-api.command.issue-time': 'Issue time',
  'cs-api.command.execution-time': 'Execution time',
  'cs-api.datastream.phenomenon-time': 'Phenomenon time',
  'cs-api.datastream.result-time': 'Result time',
  'cs-api.property.base-property': 'Base property',
  'cs-api.deployment.deployed-systems': 'Deployed systems',
  'cs-api.samplingfeature.hosted-procedure': 'Hosted procedure',
  'csapi.datastream.observed-property': 'Observed property'
};

interface SemanticLedger {
  transferredRenames: Array<[string, string]>;
  localRenames: Array<[string, string]>;
  fullIriCorrections: Array<{ canonicalInternalPredicate: string }>;
}

const semanticLedger = JSON.parse(
  readFileSync(
    new URL(
      '../../openspec/changes/migrate-semstreams-beta147/evidence/architecture/semantic-ledger.json',
      import.meta.url
    ),
    'utf8'
  )
) as SemanticLedger;

const ledgerPredicates = [
  ...semanticLedger.transferredRenames.map(([, canonical]) => canonical),
  ...semanticLedger.localRenames.map(([, canonical]) => canonical),
  ...semanticLedger.fullIriCorrections.map(({ canonicalInternalPredicate }) => canonicalInternalPredicate)
].sort();

test('owns explicit product labels for the complete frozen semantic ledger', () => {
  expect(ledgerPredicates).toHaveLength(32);
  expect(Object.keys(MIGRATED_SEMANTIC_FIELDS).sort()).toEqual(ledgerPredicates);
  expect(Object.keys(EXPECTED_MIGRATED_LABELS).sort()).toEqual(ledgerPredicates);

  for (const predicate of ledgerPredicates) {
    const explicitField = MIGRATED_SEMANTIC_FIELDS[predicate];
    expect(semanticField(predicate)).toBe(explicitField);
    expect(explicitField.label).toBe(EXPECTED_MIGRATED_LABELS[predicate]);
    expect(explicitField.description.trim()).not.toBe('');
    expect(explicitField.description).not.toContain(predicate);
  }
});

test('keeps controlled properties as scalar metadata rather than a graph edge', () => {
  const controlstream = createBaseEntities().find((entity) => entity.id === CONTROLSTREAM_VALVE_ID);
  const controlledProperties = controlstream?.facts.find(
    (fact) => fact.predicate === SEMANTIC_PREDICATES.controlstreamControlledProperties
  );

  expect(controlledProperties).toBeDefined();
  expect(JSON.parse(controlledProperties?.object ?? 'null')).toEqual([
    {
      definition: 'https://example.c360.dev/props/valve-position',
      label: 'Valve Position'
    }
  ]);
  expect(
    createBaseRelationships().some(
      (relationship) => relationship.predicate === SEMANTIC_PREDICATES.controlstreamControlledProperties
    )
  ).toBe(false);
});

test('renders the telemetry knowledge graph demo', async ({ page }) => {
  await page.goto('/');

  await expect(page.getByRole('heading', { name: 'Live Telemetry Knowledge Graph' })).toBeVisible();
  await expect(page.getByTestId('graph-surface')).toBeVisible();
  await expect(page.getByTestId('graph-surface')).toHaveAttribute('data-ready', 'true');
  await expect(page.getByTestId('graph-stats')).toContainText('13 entities');
  await expect(page.getByTestId('graph-stats')).not.toContainText('focused');
  await expect(page.getByTestId('runtime-status')).toContainText('demo mode');
  await expect(page.getByTestId('search-result')).toHaveCount(0);
  await expect(page.getByTestId('nl-query')).toHaveValue('');
  await expect(page.getByTestId('demo-story')).toContainText('Telemetry Becomes Graph Context');
  await expect(page.getByTestId('demo-story')).toContainText('SemStreams indexes');

  const canvasBox = await page.getByTestId('sigma-target').boundingBox();
  expect(canvasBox?.width ?? 0).toBeGreaterThan(400);
  expect(canvasBox?.height ?? 0).toBeGreaterThan(300);
});

test('starts and pauses the telemetry stream', async ({ page }) => {
  await page.goto('/');

  await expect(page.getByTestId('stream-state')).toContainText('3 observations');
  await page.getByTestId('start-stream').click();

  await expect(page.getByTestId('stream-state')).toContainText('receiving / 4 observations');
  await expect(page.getByTestId('sample-row').first()).toContainText(/degC|kPa/);
  await expect(page.getByTestId('entity-detail')).toContainText('observation');

  await page.getByTestId('pause-stream').click();
  await expect(page.getByTestId('stream-state')).toContainText('paused / 4 observations');
});

test('natural language search focuses command feasibility evidence', async ({ page }) => {
  await page.goto('/');

  await page.getByTestId('nl-query').fill('command feasibility for the valve');
  await page.getByTestId('run-search').click();

  await expect(page.getByTestId('search-intent')).toContainText('command.feasibility');
  await expect(page.getByTestId('search-result')).toContainText('ControlStream');
  await expect(page.getByTestId('entity-detail')).toContainText('Valve');
  await expect(page.getByTestId('graph-stats')).toContainText('focused');
});

test('graph filters can isolate telemetry resources', async ({ page }) => {
  await page.goto('/');

  await expect(page.getByTestId('graph-surface')).toHaveAttribute(
    'data-rendered-graph',
    /csapi\.controlstream\.controls-system/
  );
  await expect(page.getByTestId('graph-surface')).not.toHaveAttribute(
    'data-rendered-graph',
    /controlsSystem/
  );

  await page.getByTestId('filter-controlstream').click();
  await expect(page.getByTestId('filter-controlstream')).toHaveAttribute('aria-pressed', 'true');
  await expect(page.getByTestId('filter-system')).toHaveAttribute('aria-pressed', 'false');
  await expect(page.getByTestId('graph-stats')).toContainText('1 entities');
  await expect(page.getByTestId('graph-stats')).not.toContainText('focused');
  await expect(page.getByTestId('graph-surface')).toHaveAttribute(
    'data-rendered-graph',
    /controlstream\.valve-position/
  );
  await expect(page.getByTestId('graph-surface')).not.toHaveAttribute(
    'data-rendered-graph',
    /system\.pump-alpha/
  );
  await expect(page.getByTestId('entity-detail')).toContainText('Valve Position Commands');
  await expect(page.getByTestId('entity-detail')).toContainText('Command format');
  await expect(page.getByTestId('entity-detail')).toContainText('Controlled properties');
  await expect(page.getByTestId('entity-detail')).not.toContainText('csapi.controlstream.command-format');
  await expect(page.getByTestId('entity-detail')).not.toContainText('cs-api.controlstream.commandFormat');

  await page.getByTestId('filter-controlstream').click();
  await expect(page.getByTestId('graph-stats')).toContainText('13 entities');

  await page.getByTestId('graph-filters').getByRole('button', { name: 'Telemetry' }).click();
  await expect(page.getByTestId('graph-stats')).toContainText('11 entities');
  await expect(page.getByLabel('Filter graph')).toHaveCount(0);
});

test('loads live resources through CS API and SemStreams adapters', async ({ page }) => {
  const liveObservations: Array<Record<string, unknown>> = [
    {
      id: 'c360.demo.water.plant.observation.live-001',
      name: 'Live water temperature sample',
      'datastream@id': 'c360.demo.water.plant.datastream.live-temp',
      phenomenonTime: '2026-06-03T14:31:00.000Z',
      resultTime: '2026-06-03T14:31:00.000Z',
      observedProperty: 'water temperature',
      result: { value: 21.2, uom: 'degC' }
    }
  ];
  const postedObservations: Array<Record<string, unknown>> = [];
  const postedObservationContentTypes: string[] = [];

  await page.route('**/semconnect-demo.config.json', async (route) => {
    await route.fulfill({
      contentType: 'application/json',
      json: {
        mode: 'live',
        csApiBaseUrl: '/mock-cs',
        graphqlEndpoint: '/mock-graph',
        graphPrefixes: ['c360.demo.water.plant'],
        pollMs: 0,
        limits: { observations: 5 },
        semanticAssist: {
          enabled: true,
          semembedEndpoint: '/mock-embed/v1',
          semembedModel: 'all-MiniLM-L6-v2',
          seminstructEndpoint: '/mock-instruct/v1',
          seminstructModel: 'qwen3-0.6b',
          similarityThreshold: 0.72,
          maxMatches: 6
        }
      }
    });
  });

  await page.route('**/mock-cs/**', async (route) => {
    const url = new URL(route.request().url());
    const method = route.request().method();
    const observationPostPath =
      '/mock-cs/datastreams/c360.demo.water.plant.datastream.inlet-temperature/observations';

    if (method === 'POST' && url.pathname === observationPostPath) {
      const body = route.request().postDataJSON() as Record<string, unknown>;
      postedObservations.push(body);
      postedObservationContentTypes.push(route.request().headers()['content-type'] ?? '');
      liveObservations.unshift({
        ...body,
        name: 'Posted water temperature sample',
        'datastream@id': 'c360.demo.water.plant.datastream.inlet-temperature'
      });
      await route.fulfill({
        status: 201,
        contentType: 'application/json',
        json: {
          status: 'accepted',
          id: body.id,
          subject: 'cs-api.observations.c360.demo.water.plant.datastream.inlet-temperature'
        }
      });
      return;
    }

    const payloads: Record<string, unknown> = {
      '/mock-cs/systems': {
        items: [
          {
            id: 'c360.demo.water.plant.system.live-pump',
            name: 'Live Pump Alpha',
            description: 'Loaded from the Connected Systems API.'
          }
        ]
      },
      '/mock-cs/datastreams': {
        items: [
          {
            id: 'c360.demo.water.plant.datastream.live-temp',
            name: 'Live Temperature',
            'system@id': 'c360.demo.water.plant.system.live-pump',
            observedProperty: {
              id: 'https://qudt.org/vocab/quantitykind/Temperature',
              name: 'Water Temperature',
              definition: 'https://qudt.org/vocab/quantitykind/Temperature'
            }
          }
        ]
      },
      '/mock-cs/observations': {
        items: liveObservations
      },
      '/mock-cs/controlstreams': { items: [] },
      '/mock-cs/feasibility': { items: [] }
    };

    await route.fulfill({
      contentType: 'application/json',
      json: payloads[url.pathname] ?? { items: [] }
    });
  });

  await page.route('**/mock-graph', async (route) => {
    const request = route.request().postDataJSON() as { query?: string };
    const isPrefixQuery = request.query?.includes('entitiesByPrefix');

    await route.fulfill({
      contentType: 'application/json',
      json: isPrefixQuery
        ? {
            data: {
              entitiesByPrefix: [
                {
                  id: 'c360.demo.water.plant.system.live-pump',
                  triples: [
                    {
                      subject: 'c360.demo.water.plant.system.live-pump',
                      predicate: 'sensorml.process.label',
                      object: 'Live Pump Alpha'
                    },
                    {
                      subject: 'c360.demo.water.plant.system.live-pump',
                      predicate: 'sensorml.process.type',
                      object: 'ssn:System'
                    }
                  ]
                }
              ]
            }
          }
        : {
            data: {
              globalSearch: {
                entities: [
                  {
                    id: 'c360.demo.water.plant.datastream.live-temp',
                    triples: [
                      {
                        subject: 'c360.demo.water.plant.datastream.live-temp',
                        predicate: 'sensorml.process.label',
                        object: 'Live Temperature'
                      },
                      {
                        subject: 'c360.demo.water.plant.datastream.live-temp',
                        predicate: 'sensorml.process.type',
                        object: 'csapi:Datastream'
                      },
                      {
                        subject: 'c360.demo.water.plant.datastream.live-temp',
                        predicate: 'csapi.datastream.produced-by',
                        object: 'c360.demo.water.plant.system.live-pump'
                      }
                    ]
                  },
                  {
                    id: 'c360.demo.water.plant.system.live-pump',
                    triples: [
                      {
                        subject: 'c360.demo.water.plant.system.live-pump',
                        predicate: 'sensorml.process.label',
                        object: 'Live Pump Alpha'
                      }
                    ]
                  }
                ],
                community_summaries: [
                  {
                    communityId: 'telemetry',
                    text: 'Live temperature telemetry is attached to Pump Alpha.',
                    keywords: ['temperature', 'telemetry']
                  }
                ],
                relationships: [
                  {
                    from: 'c360.demo.water.plant.datastream.live-temp',
                    to: 'c360.demo.water.plant.system.live-pump',
                    predicate: 'csapi.datastream.produced-by'
                  }
                ],
                count: 2,
                duration_ms: 11
              }
            },
            extensions: {
              classification: {
                intent: 'semstreams.temperature',
                confidence: 0.93
              }
            }
          }
    });
  });

  await page.route('**/mock-instruct/v1/chat/completions', async (route) => {
    await route.fulfill({
      contentType: 'application/json',
      json: {
        choices: [
          {
            message: {
              content: JSON.stringify({
                intent: 'telemetry.temperature',
                confidence: 0.94,
                slots: { observedProperty: 'temperature' }
              })
            }
          }
        ]
      }
    });
  });

  await page.route('**/mock-embed/v1/embeddings', async (route) => {
    const request = route.request().postDataJSON() as { input?: string[] };
    const input = request.input ?? [];

    await route.fulfill({
      contentType: 'application/json',
      json: {
        data: input.map((text, index) => ({
          index,
          embedding: semanticEmbeddingFor(text)
        }))
      }
    });
  });

  await page.goto('/');

  await expect(page.getByTestId('stream-state')).toContainText('1 observations');
  await expect(page.getByTestId('connection-state')).toContainText('Live CS API and SemStreams graph gateway connected');
  await expect(page.getByTestId('search-result')).toHaveCount(0);
  await expect(page.getByTestId('sample-row').first()).toContainText('21.2');

  await page.getByTestId('start-stream').click();
  await expect.poll(() => postedObservations.length).toBe(1);
  expect(postedObservations[0]).toMatchObject({
    id: 'c360.demo.water.plant.observation.obs-004',
    procedure: 'c360.demo.water.plant.system.temp-probe-t17',
    observedProperty: 'water temperature'
  });
  expect(postedObservationContentTypes[0]).toContain('application/om+json');
  await expect(page.getByTestId('stream-state')).toContainText('receiving / 2 observations');
  await expect(page.getByTestId('connection-state')).toContainText('Posted telemetry through CS API');
  await expect(page.getByTestId('sample-row').first()).toContainText('degC');
  await page.getByTestId('pause-stream').click();

  await page.getByTestId('nl-query').fill('temperature telemetry');
  await page.getByTestId('run-search').click();

  await expect(page.getByTestId('connection-state')).toContainText('semantic assist');
  await expect(page.getByTestId('entity-detail')).toContainText('Live Temperature');
  await expect(page.getByTestId('entity-detail')).toContainText('system.live-pump');
  await expect(page.getByTestId('search-intent')).toContainText('telemetry.temperature');
  await expect(page.getByTestId('search-result')).toContainText('Semantic classifier');
  await expect(page.getByTestId('search-result')).toContainText('Semantic similarity matched');
  await expect(page.getByTestId('graph-surface')).toHaveAttribute('data-ready', 'true');
});

function semanticEmbeddingFor(text: string): number[] {
  const normalized = text.toLowerCase();
  if (normalized.includes('temperature')) return [0.98, 0.04, 0.02];
  if (normalized.includes('telemetry')) return [0.91, 0.08, 0.03];
  if (normalized.includes('pump')) return [0.74, 0.18, 0.08];
  if (normalized.includes('pressure')) return [0.58, 0.32, 0.08];
  return [0.08, 0.82, 0.1];
}
