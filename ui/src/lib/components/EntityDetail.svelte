<script lang="ts">
  import { Activity, BrainCircuit, Database, GitBranch, Link2 } from '@lucide/svelte';
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
    <div class="demo-story" data-testid="demo-story">
      <header class="story-header">
        <span class="story-icon" aria-hidden="true">
          <Activity size={18} />
        </span>
        <div>
          <span class="kind">demo path</span>
          <h2>Telemetry Becomes Graph Context</h2>
        </div>
      </header>

      <p class="story-copy">
        Connected Systems resources land at the server as Systems, Datastreams, Observations,
        ControlStreams, and Feasibility evidence. SemStreams turns that feed into triples,
        relationships, summaries, and optional semantic signals. The graph renders the result while
        new readings keep changing the evidence.
      </p>

      <ol class="story-steps" aria-label="Demo data flow">
        <li>
          <Database size={15} />
          <span>CS API resources and telemetry readings arrive with canonical IDs.</span>
        </li>
        <li>
          <GitBranch size={15} />
          <span>SemStreams indexes predicates, edges, resource classes, and provenance.</span>
        </li>
        <li>
          <BrainCircuit size={15} />
          <span>Search can bring nearby graph context into focus while the evidence stays traceable.</span>
        </li>
      </ol>
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

  header:not(.story-header) {
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
  .story-copy {
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

  .demo-story {
    display: flex;
    flex-direction: column;
    gap: 14px;
    min-height: 260px;
    padding: 6px 0;
  }

  .story-header {
    display: grid;
    grid-template-columns: 34px minmax(0, 1fr);
    gap: 10px;
    align-items: center;
  }

  .story-icon {
    display: grid;
    width: 34px;
    height: 34px;
    place-items: center;
    border: 1px solid #c7d9dd;
    border-radius: 7px;
    background: #eef8f7;
    color: #146c73;
  }

  .story-steps {
    display: flex;
    flex-direction: column;
    gap: 8px;
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .story-steps li {
    display: grid;
    grid-template-columns: 18px minmax(0, 1fr);
    gap: 8px;
    align-items: start;
    padding: 9px;
    border: 1px solid #d7dce8;
    border-radius: 7px;
    background: #ffffff;
    color: #243047;
    font-size: 12px;
    line-height: 1.35;
  }

  .story-steps :global(svg) {
    margin-top: 1px;
    color: #526078;
  }
</style>
