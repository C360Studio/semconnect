<script lang="ts">
  import { onDestroy, onMount } from 'svelte';
  import { Activity, BrainCircuit, GitBranch, RadioTower } from '@lucide/svelte';
  import EntityDetail from '$lib/components/EntityDetail.svelte';
  import GraphFilters from '$lib/components/GraphFilters.svelte';
  import NaturalLanguageSearch from '$lib/components/NaturalLanguageSearch.svelte';
  import SigmaCanvas from '$lib/components/SigmaCanvas.svelte';
  import TelemetryStream from '$lib/components/TelemetryStream.svelte';
  import { demoStore as store } from '$lib/stores/demoStore.svelte';

  const metrics = $derived([
    { label: 'Systems', value: store.kindCounts.get('system') ?? 0, icon: RadioTower },
    { label: 'Telemetry', value: store.samples.length, icon: Activity },
    { label: 'Graph edges', value: store.relationships.length, icon: GitBranch },
    { label: 'Search focus', value: store.focusedEntityIds.size, icon: BrainCircuit }
  ]);

  onMount(() => {
    void store.initialize();
  });

  onDestroy(() => {
    store.dispose();
  });
</script>

<svelte:head>
  <title>semconnect demo</title>
</svelte:head>

<main class="demo-shell">
  <header class="topbar">
    <div>
      <div class="title-meta">
        <p class="eyebrow">semconnect demo</p>
        <div class="runtime-status" aria-label="Runtime status" data-testid="runtime-status">
          <span>{store.config.mode} mode</span>
          {#if store.connectionStatus !== store.config.mode}
            <span>{store.connectionStatus}</span>
          {/if}
        </div>
      </div>
      <h1>Live Telemetry Knowledge Graph</h1>
    </div>
    <div class="metric-strip" aria-label="Demo metrics">
      {#each metrics as metric}
        {@const MetricIcon = metric.icon}
        <div class="metric">
          <MetricIcon size={17} />
          <span>{metric.label}</span>
          <strong>{metric.value}</strong>
        </div>
      {/each}
    </div>
  </header>

  <section class="workspace">
    <aside class="left-pane">
      <TelemetryStream {store} />
      <NaturalLanguageSearch {store} />
    </aside>

    <section class="graph-pane">
      <GraphFilters
        visibleKinds={store.visibleKinds}
        kindCounts={store.kindCounts}
        onToggleKind={(kind) => store.toggleKind(kind)}
        onShowAll={() => store.showAllKinds()}
        onShowTelemetry={() => store.showTelemetryKinds()}
      />
      <SigmaCanvas
        entities={store.visibleEntities}
        relationships={store.visibleRelationships}
        selectedEntityId={store.selectedEntityId}
        focusedEntityIds={store.focusedEntityIds}
        onEntitySelect={(entityId) => store.selectEntity(entityId)}
      />
    </section>

    <aside class="right-pane">
      <EntityDetail
        entity={store.selectedEntity}
        relationships={store.relationships}
        onSelect={(entityId) => store.selectEntity(entityId)}
      />
    </aside>
  </section>
</main>

<style>
  .demo-shell {
    display: grid;
    grid-template-rows: auto minmax(0, 1fr);
    min-height: 100vh;
    background: #f6f7fb;
  }

  .topbar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 18px;
    min-height: 86px;
    padding: 14px 18px;
    border-bottom: 1px solid #d7dce8;
    background: #ffffff;
  }

  .eyebrow {
    margin: 0;
    color: #146c73;
    font-size: 12px;
    font-weight: 780;
  }

  .title-meta {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 8px;
    margin-bottom: 4px;
  }

  .runtime-status {
    display: flex;
    align-items: center;
    gap: 5px;
  }

  .runtime-status span {
    min-height: 20px;
    padding: 2px 7px;
    border: 1px solid #d7dce8;
    border-radius: 7px;
    background: #f8f9fc;
    color: #526078;
    font-size: 11px;
    font-weight: 720;
  }

  h1 {
    margin: 0;
    color: #172033;
    font-size: 25px;
    line-height: 1.1;
    letter-spacing: 0;
  }

  .metric-strip {
    display: flex;
    flex-wrap: wrap;
    justify-content: flex-end;
    gap: 8px;
  }

  .metric {
    display: grid;
    grid-template-columns: 18px auto;
    grid-template-rows: auto auto;
    column-gap: 7px;
    align-items: center;
    min-width: 118px;
    min-height: 52px;
    padding: 8px;
    border: 1px solid #d7dce8;
    border-radius: 7px;
    background: #fbfcff;
  }

  .metric :global(svg) {
    grid-row: 1 / span 2;
    color: #526078;
  }

  .metric span {
    color: #69768d;
    font-size: 11px;
    font-weight: 700;
  }

  .metric strong {
    color: #172033;
    font-size: 18px;
    line-height: 1;
  }

  .workspace {
    display: grid;
    grid-template-columns: minmax(280px, 330px) minmax(0, 1fr) minmax(280px, 340px);
    min-height: 0;
  }

  .left-pane,
  .right-pane {
    display: flex;
    flex-direction: column;
    gap: 18px;
    min-height: 0;
    padding: 14px;
    overflow: auto;
    background: #f8f9fc;
  }

  .left-pane {
    border-right: 1px solid #d7dce8;
  }

  .right-pane {
    border-left: 1px solid #d7dce8;
  }

  .graph-pane {
    display: grid;
    grid-template-rows: auto minmax(0, 1fr);
    min-height: 0;
  }

  @media (max-width: 1180px) {
    .workspace {
      grid-template-columns: minmax(260px, 310px) minmax(0, 1fr);
    }

    .right-pane {
      grid-column: 1 / -1;
      min-height: 300px;
      border-left: 0;
      border-top: 1px solid #d7dce8;
    }
  }

  @media (max-width: 820px) {
    .topbar {
      align-items: flex-start;
      flex-direction: column;
    }

    .metric-strip {
      width: 100%;
      justify-content: stretch;
    }

    .metric {
      flex: 1;
      min-width: 130px;
    }

    .workspace {
      grid-template-columns: 1fr;
    }

    .left-pane,
    .right-pane {
      border: 0;
      border-bottom: 1px solid #d7dce8;
    }

    .graph-pane {
      min-height: 560px;
    }
  }
</style>
