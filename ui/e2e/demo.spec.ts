import { expect, test } from '@playwright/test';

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

  await page.getByTestId('filter-controlstream').click();
  await expect(page.getByTestId('graph-stats')).toContainText('13 entities');

  await page.getByTestId('graph-filters').getByRole('button', { name: 'Telemetry' }).click();
  await expect(page.getByTestId('graph-stats')).toContainText('11 entities');
  await expect(page.getByLabel('Filter graph')).toHaveCount(0);
});

test('loads live resources through CS API and SemStreams adapters', async ({ page }) => {
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
        items: [
          {
            id: 'c360.demo.water.plant.observation.live-001',
            name: 'Live water temperature sample',
            'datastream@id': 'c360.demo.water.plant.datastream.live-temp',
            phenomenonTime: '2026-06-03T14:31:00.000Z',
            resultTime: '2026-06-03T14:31:00.000Z',
            observedProperty: 'water temperature',
            result: { value: 21.2, uom: 'degC' }
          }
        ]
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
                      predicate: 'rdf.type',
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
                        predicate: 'rdf.type',
                        object: 'csapi:Datastream'
                      },
                      {
                        subject: 'c360.demo.water.plant.datastream.live-temp',
                        predicate: 'csapi.datastream.producedBy',
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
                    predicate: 'csapi.datastream.producedBy'
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
