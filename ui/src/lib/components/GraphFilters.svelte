<script lang="ts">
  import type { ResourceKind } from '$lib/types/demo';
  import { RESOURCE_KINDS } from '$lib/types/demo';
  import { KIND_COLORS } from '$lib/utils/colors';

  interface Props {
    visibleKinds: Set<ResourceKind>;
    kindCounts: Map<ResourceKind, number>;
    onToggleKind: (kind: ResourceKind) => void;
    onShowAll: () => void;
    onShowTelemetry: () => void;
  }

  let {
    visibleKinds,
    kindCounts,
    onToggleKind,
    onShowAll,
    onShowTelemetry
  }: Props = $props();
</script>

<section class="filters" aria-label="Graph filters" data-testid="graph-filters">
  <div class="kind-list" role="group" aria-label="Resource type filters">
    {#each RESOURCE_KINDS as kind (kind)}
      {@const checked = visibleKinds.has(kind)}
      {@const solo = visibleKinds.size === 1 && checked}
      <button
        type="button"
        class:checked
        class:solo
        class="kind-filter"
        style={`--kind-color: ${KIND_COLORS[kind]}`}
        aria-pressed={checked}
        onclick={() => onToggleKind(kind)}
        data-testid={`filter-${kind}`}
      >
        <span class="swatch" aria-hidden="true"></span>
        <span>{kind}</span>
        <strong>{kindCounts.get(kind) ?? 0}</strong>
      </button>
    {/each}
  </div>

  <div class="filter-actions">
    <button type="button" class="text-button" onclick={onShowAll}>All</button>
    <button type="button" class="text-button" onclick={onShowTelemetry}>Telemetry</button>
  </div>
</section>

<style>
  .filters {
    display: flex;
    align-items: center;
    gap: 10px;
    min-height: 48px;
    padding: 8px 12px;
    border-bottom: 1px solid #d7dce8;
    background: #ffffff;
  }

  .kind-list {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
    min-width: 0;
    flex: 1;
  }

  .kind-filter {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    min-height: 28px;
    padding: 0 8px;
    border: 1px solid #d7dce8;
    border-radius: 7px;
    color: #526078;
    cursor: pointer;
    font-size: 12px;
    white-space: nowrap;
    background: #ffffff;
  }

  .kind-filter.checked {
    color: #172033;
    border-color: color-mix(in srgb, var(--kind-color) 55%, #d7dce8);
    background: color-mix(in srgb, var(--kind-color) 12%, #ffffff);
  }

  .kind-filter.solo {
    border-color: var(--kind-color);
    box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--kind-color) 72%, #ffffff);
  }

  .kind-filter:not(.checked) {
    opacity: 0.58;
  }

  .kind-filter:hover {
    border-color: var(--kind-color);
    background: color-mix(in srgb, var(--kind-color) 9%, #ffffff);
  }

  .swatch {
    width: 9px;
    height: 9px;
    border-radius: 50%;
    background: var(--kind-color);
  }

  .kind-filter strong {
    font-size: 11px;
    color: #69768d;
  }

  .filter-actions {
    display: flex;
    gap: 6px;
  }

  .filter-actions .text-button {
    min-height: 30px;
    padding: 0 10px;
    font-size: 12px;
  }

  @media (max-width: 980px) {
    .filters {
      align-items: flex-start;
      flex-direction: column;
    }
  }
</style>
