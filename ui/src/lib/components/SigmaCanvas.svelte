<script lang="ts">
  import { Maximize2, RefreshCw, ZoomIn, ZoomOut } from '@lucide/svelte';
  import type { MultiDirectedGraph } from 'graphology';
  import type Sigma from 'sigma';
  import type { DemoEntity, DemoRelationship } from '$lib/types/demo';
  import { syncDemoGraph } from '$lib/utils/graphology-adapter';
  import { LayoutController } from '$lib/utils/sigma-layout';

  interface Props {
    entities: DemoEntity[];
    relationships: DemoRelationship[];
    selectedEntityId?: string | null;
    focusedEntityIds?: Set<string>;
    onEntitySelect?: (entityId: string | null) => void;
  }

  let {
    entities,
    relationships,
    selectedEntityId = null,
    focusedEntityIds = new Set<string>(),
    onEntitySelect
  }: Props = $props();

  let containerElement: HTMLDivElement;
  let sigma: Sigma | null = null;
  let graph: MultiDirectedGraph | null = null;
  let layout: LayoutController | null = null;
  let previousEntityCount = 0;
  let graphReady = $state(false);
  let renderedGraphSignature = $state('');
  const graphSignature = $derived(
    `${entities.map((entity) => entity.id).join('|')}::${relationships.map((relationship) => relationship.id).join('|')}`
  );
  const visibleFocusedIds = $derived(
    new Set(entities.filter((entity) => focusedEntityIds.has(entity.id)).map((entity) => entity.id))
  );
  const focusSignature = $derived(`${selectedEntityId ?? ''}::${[...visibleFocusedIds].join('|')}`);

  $effect(() => {
    if (typeof window === 'undefined' || !containerElement) return;

    let disposed = false;

    async function initializeGraph() {
      const [{ MultiDirectedGraph }, { default: SigmaRenderer }] = await Promise.all([
        import('graphology'),
        import('sigma')
      ]);

      if (disposed || !containerElement) return;

      graph = new MultiDirectedGraph();
      layout = new LayoutController();

      sigma = new SigmaRenderer(graph, containerElement, {
        allowInvalidContainer: true,
        defaultEdgeType: 'arrow',
        labelRenderedSizeThreshold: 7,
        renderEdgeLabels: false,
        zIndex: true,
        labelColor: { color: '#172033' },
        nodeReducer: (node, data) => {
          const next = { ...data };
          const hasFocus = visibleFocusedIds.size > 0;
          const isFocused = visibleFocusedIds.has(node);
          const isSelected = node === selectedEntityId;

          if (hasFocus && !isFocused && !isSelected) {
            next.color = '#b8c0cf';
            next.label = '';
            next.zIndex = 0;
          }

          if (isSelected) {
            next.highlighted = true;
            next.zIndex = 3;
          } else if (hasFocus && isFocused) {
            next.zIndex = 2;
          }

          return next;
        },
        edgeReducer: (edge, data) => {
          const next = { ...data };
          if (!graph) return next;

          const hasFocus = visibleFocusedIds.size > 0;
          if (hasFocus) {
            const source = graph.source(edge);
            const target = graph.target(edge);
            if (!visibleFocusedIds.has(source) || !visibleFocusedIds.has(target)) {
              next.color = '#c7ceda';
            }
          }
          return next;
        }
      });

      sigma.on('clickNode', ({ node }) => {
        onEntitySelect?.(node === selectedEntityId ? null : node);
      });

      sigma.on('clickStage', () => {
        onEntitySelect?.(null);
      });

      syncDemoGraph(graph, entities, relationships);
      layout.start(graph);
      previousEntityCount = entities.length;
      renderedGraphSignature = graphSignature;
      sigma.clear();
      sigma.refresh({ schedule: false });
      graphReady = true;
    }

    initializeGraph().catch((error: unknown) => {
      console.error('Failed to initialize Sigma graph', error);
    });

    return () => {
      disposed = true;
      graphReady = false;
      layout?.stop();
      sigma?.kill();
      sigma = null;
      graph = null;
      layout = null;
    };
  });

  $effect(() => {
    const nextGraphSignature = graphSignature;
    if (!graph || !sigma || !layout) return;

    syncDemoGraph(graph, entities, relationships);
    if (entities.length !== previousEntityCount) {
      layout.start(graph);
      previousEntityCount = entities.length;
    }
    renderedGraphSignature = nextGraphSignature;
    sigma.clear();
    sigma.refresh({ schedule: false });
    graphReady = true;
  });

  $effect(() => {
    void focusSignature;
    if (!sigma) return;
    sigma.refresh({ schedule: false, skipIndexation: true });
  });

  function zoomIn() {
    sigma?.getCamera().animatedZoom({ duration: 180 });
  }

  function zoomOut() {
    sigma?.getCamera().animatedUnzoom({ duration: 180 });
  }

  function fitToContent() {
    sigma?.getCamera().animatedReset({ duration: 260 });
  }

  function refreshLayout() {
    if (!graph || !layout || !sigma) return;
    layout.start(graph);
    sigma.refresh();
  }
</script>

<section
  class="graph-surface"
  aria-label="Telemetry knowledge graph"
  data-testid="graph-surface"
  data-ready={graphReady}
  data-rendered-graph={renderedGraphSignature}
>
  <div class="graph-toolbar" role="toolbar" aria-label="Graph controls">
    <button class="icon-button" onclick={zoomIn} aria-label="Zoom in" title="Zoom in">
      <ZoomIn size={17} />
    </button>
    <button class="icon-button" onclick={zoomOut} aria-label="Zoom out" title="Zoom out">
      <ZoomOut size={17} />
    </button>
    <button class="icon-button" onclick={fitToContent} aria-label="Fit graph" title="Fit graph">
      <Maximize2 size={17} />
    </button>
    <button class="icon-button" onclick={refreshLayout} aria-label="Refresh layout" title="Refresh layout">
      <RefreshCw size={17} />
    </button>
  </div>

  <div
    class="sigma-target"
    bind:this={containerElement}
    role="img"
    aria-label="Graph of systems, datastreams, observations, and command resources"
    data-testid="sigma-target"
  ></div>

  <div class="graph-stats" aria-live="polite" data-testid="graph-stats">
    <span>{entities.length} entities</span>
    <span>{relationships.length} relationships</span>
    {#if visibleFocusedIds.size > 0}
      <span>{visibleFocusedIds.size} focused</span>
    {/if}
  </div>
</section>

<style>
  .graph-surface {
    position: relative;
    min-height: 0;
    height: 100%;
    background:
      linear-gradient(90deg, rgba(34, 47, 72, 0.055) 1px, transparent 1px),
      linear-gradient(rgba(34, 47, 72, 0.055) 1px, transparent 1px),
      #fbfcff;
    background-size: 28px 28px;
    overflow: hidden;
  }

  .sigma-target {
    width: 100%;
    height: 100%;
  }

  .graph-toolbar {
    position: absolute;
    top: 12px;
    right: 12px;
    display: flex;
    gap: 6px;
    z-index: 5;
  }

  .graph-stats {
    position: absolute;
    left: 12px;
    bottom: 12px;
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
    padding: 6px 8px;
    border: 1px solid #d7dce8;
    border-radius: 7px;
    background: rgba(255, 255, 255, 0.92);
    color: #4f5f78;
    font-size: 12px;
    font-weight: 650;
  }
</style>
