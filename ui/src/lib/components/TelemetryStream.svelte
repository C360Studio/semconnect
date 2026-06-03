<script lang="ts">
  import { Pause, Play, RefreshCw, RotateCcw } from '@lucide/svelte';
  import type { DemoStore } from '$lib/stores/demoStore.svelte';

  interface Props {
    store: DemoStore;
  }

  let { store }: Props = $props();
</script>

<section class="telemetry-panel" aria-label="Telemetry stream">
  <header>
    <div>
      <h2>Telemetry Stream</h2>
      <p class="stream-state" data-testid="stream-state">
        {store.running ? 'receiving' : 'paused'} / {store.observationCount} observations
      </p>
      <p class="connection-state" data-testid="connection-state">
        {store.connectionMessage}
      </p>
    </div>
    <span
      class:running={store.running || store.connectionStatus === 'live'}
      class:warning={store.connectionStatus === 'hybrid' || store.connectionStatus === 'error'}
      class="pulse"
      aria-hidden="true"
    ></span>
  </header>

  <div class="stream-actions">
    {#if store.running}
      <button type="button" class="text-button warn" onclick={() => store.stop()} data-testid="pause-stream">
        <Pause size={16} />
        Pause
      </button>
    {:else}
      <button type="button" class="text-button primary" onclick={() => store.start()} data-testid="start-stream">
        <Play size={16} />
        Start
      </button>
    {/if}
    <button
      type="button"
      class="icon-button"
      onclick={() => void store.refreshLiveData()}
      disabled={store.config.mode === 'demo' || store.loading}
      aria-label="Refresh live graph"
      title="Refresh live graph"
      data-testid="refresh-live"
    >
      <RefreshCw size={16} />
    </button>
    <button type="button" class="icon-button" onclick={() => store.reset()} aria-label="Reset demo" title="Reset demo">
      <RotateCcw size={16} />
    </button>
  </div>

  {#if store.liveErrors.length > 0}
    <ul class="integration-errors" aria-label="Live integration warnings" data-testid="integration-errors">
      {#each store.liveErrors.slice(0, 3) as error}
        <li>{error}</li>
      {/each}
    </ul>
  {/if}

  <ol class="sample-list" aria-label="Latest telemetry samples" data-testid="sample-list">
    {#each store.samples.slice(0, 8) as sample (sample.id)}
      <li class:watch={sample.quality !== 'good'} data-testid="sample-row">
        <div class="sample-main">
          <span class="sample-value">{sample.value.toFixed(1)}</span>
          <span class="sample-unit">{sample.unit}</span>
        </div>
        <div class="sample-meta">
          <strong>{sample.observedProperty}</strong>
          <span>{sample.resultTime.slice(11, 19)}Z</span>
        </div>
      </li>
    {/each}
  </ol>
</section>

<style>
  .telemetry-panel {
    display: flex;
    flex-direction: column;
    gap: 12px;
    min-height: 0;
  }

  header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
  }

  h2 {
    margin: 0;
    font-size: 15px;
    letter-spacing: 0;
  }

  .stream-state {
    margin: 3px 0 0;
    color: #647089;
    font-size: 12px;
    font-weight: 650;
  }

  .connection-state {
    margin: 3px 0 0;
    color: #41516c;
    font-size: 11px;
    line-height: 1.35;
   }

  .pulse {
    width: 12px;
    height: 12px;
    border-radius: 50%;
    background: #98a2b7;
  }

  .pulse.running {
    background: #12a37f;
    box-shadow: 0 0 0 5px rgba(18, 163, 127, 0.16);
  }

  .pulse.warning {
    background: #d39c61;
    box-shadow: 0 0 0 5px rgba(211, 156, 97, 0.18);
  }

  .stream-actions {
    display: flex;
    gap: 8px;
    flex-wrap: wrap;
  }

  .sample-list {
    display: flex;
    flex-direction: column;
    gap: 8px;
    min-height: 0;
    overflow: auto;
    padding: 0;
    margin: 0;
    list-style: none;
  }

  .integration-errors {
    display: flex;
    flex-direction: column;
    gap: 4px;
    margin: 0;
    padding-left: 18px;
    color: #8a5b21;
    font-size: 11px;
    line-height: 1.35;
  }

  li {
    display: grid;
    grid-template-columns: 70px 1fr;
    gap: 10px;
    align-items: center;
    min-height: 58px;
    padding: 8px;
    border: 1px solid #d7dce8;
    border-radius: 7px;
    background: #ffffff;
  }

  li.watch {
    border-color: #d39c61;
    background: #fff8ee;
  }

  .sample-main {
    display: flex;
    align-items: baseline;
    gap: 3px;
    color: #172033;
  }

  .sample-value {
    font-size: 23px;
    font-weight: 760;
  }

  .sample-unit {
    color: #69768d;
    font-size: 11px;
    font-weight: 700;
  }

  .sample-meta {
    display: flex;
    flex-direction: column;
    gap: 4px;
    min-width: 0;
  }

  .sample-meta strong {
    color: #243047;
    font-size: 12px;
  }

  .sample-meta span {
    color: #69768d;
    font-size: 12px;
    font-variant-numeric: tabular-nums;
  }
</style>
