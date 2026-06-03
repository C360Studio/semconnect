<script lang="ts">
  import { Crosshair, Link2 } from '@lucide/svelte';
  import type { DemoEntity, DemoRelationship } from '$lib/types/demo';
  import { colorForKind } from '$lib/utils/colors';

  interface Props {
    entity: DemoEntity | null;
    relationships: DemoRelationship[];
    onSelect: (entityId: string) => void;
  }

  let { entity, relationships, onSelect }: Props = $props();

  let connected = $derived(
    entity
      ? relationships.filter(
          (relationship) => relationship.sourceId === entity.id || relationship.targetId === entity.id
        )
      : []
  );
</script>

<section class="detail-panel" aria-label="Selected resource details" data-testid="entity-detail">
  {#if entity}
    <header style={`--kind-color: ${colorForKind(entity.kind)}`}>
      <span class="kind">{entity.kind}</span>
      <h2>{entity.label}</h2>
      <p>{entity.summary}</p>
    </header>

    <dl class="identity">
      <div>
        <dt>Status</dt>
        <dd>{entity.status}</dd>
      </div>
      <div>
        <dt>Updated</dt>
        <dd>{entity.updatedAt.slice(11, 19)}Z</dd>
      </div>
    </dl>

    <section class="facts" aria-label="Graph facts">
      <h3>Graph Facts</h3>
      <ul>
        {#each entity.facts as fact}
          <li>
            <span>{fact.predicate}</span>
            <strong>{fact.object}</strong>
            <em>{fact.source}</em>
          </li>
        {/each}
      </ul>
    </section>

    <section class="links" aria-label="Connected graph resources">
      <h3>Edges</h3>
      <ul>
        {#each connected as relationship}
          {@const targetId = relationship.sourceId === entity.id ? relationship.targetId : relationship.sourceId}
          <li>
            <button type="button" onclick={() => onSelect(targetId)}>
              <Link2 size={14} />
              <span>{relationship.label}</span>
              <strong>{targetId.split('.').slice(-2).join('.')}</strong>
            </button>
          </li>
        {/each}
      </ul>
    </section>
  {:else}
    <div class="empty">
      <Crosshair size={22} />
      <h2>No resource selected</h2>
      <p>Select a graph node or run a search.</p>
    </div>
  {/if}
</section>

<style>
  .detail-panel {
    display: flex;
    flex-direction: column;
    gap: 14px;
    min-height: 0;
  }

  header {
    border-left: 4px solid var(--kind-color);
    padding-left: 10px;
  }

  .kind {
    color: var(--kind-color);
    font-size: 11px;
    font-weight: 780;
    text-transform: uppercase;
  }

  h2,
  h3,
  p,
  dl,
  ul {
    margin: 0;
  }

  h2 {
    margin-top: 4px;
    color: #172033;
    font-size: 17px;
    letter-spacing: 0;
  }

  header p,
  .empty p {
    margin-top: 6px;
    color: #5d6b82;
    font-size: 12px;
    line-height: 1.45;
  }

  .identity {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 8px;
  }

  .identity div {
    padding: 8px;
    border: 1px solid #d7dce8;
    border-radius: 7px;
    background: #ffffff;
  }

  dt {
    color: #69768d;
    font-size: 11px;
  }

  dd {
    margin: 3px 0 0;
    color: #243047;
    font-size: 13px;
    font-weight: 720;
  }

  h3 {
    color: #243047;
    font-size: 13px;
  }

  .facts,
  .links {
    display: flex;
    flex-direction: column;
    gap: 8px;
    min-height: 0;
  }

  .facts ul,
  .links ul {
    display: flex;
    flex-direction: column;
    gap: 7px;
    padding: 0;
    list-style: none;
    overflow: auto;
  }

  .facts li {
    display: grid;
    gap: 4px;
    padding: 8px;
    border: 1px solid #d7dce8;
    border-radius: 7px;
    background: #ffffff;
  }

  .facts span {
    color: #526078;
    font-size: 11px;
    font-weight: 720;
  }

  .facts strong {
    min-width: 0;
    overflow-wrap: anywhere;
    color: #172033;
    font-size: 12px;
  }

  .facts em {
    color: #7a8598;
    font-size: 11px;
    font-style: normal;
  }

  .links button {
    display: grid;
    grid-template-columns: 16px minmax(0, 0.75fr) minmax(0, 1fr);
    gap: 8px;
    align-items: center;
    width: 100%;
    min-height: 36px;
    padding: 7px;
    border: 1px solid #d7dce8;
    border-radius: 7px;
    background: #ffffff;
    color: #243047;
    cursor: pointer;
    text-align: left;
  }

  .links button:hover {
    border-color: #9ba9c2;
    background: #eef3fb;
  }

  .links span,
  .links strong {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: 12px;
  }

  .links strong {
    color: #526078;
  }

  .empty {
    display: flex;
    min-height: 260px;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    text-align: center;
    color: #69768d;
  }
</style>
