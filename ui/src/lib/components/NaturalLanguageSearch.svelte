<script lang="ts">
  import { Search, Sparkles } from '@lucide/svelte';
  import type { DemoStore } from '$lib/stores/demoStore.svelte';

  interface Props {
    store: DemoStore;
  }

  let { store }: Props = $props();

  const suggestions = [
    'temperature telemetry',
    'command feasibility for the valve',
    'systems attached to the pump station'
  ];

  function submit(event: SubmitEvent) {
    event.preventDefault();
    void store.runSearch();
  }
</script>

<section class="search-panel" aria-label="Natural language graph search">
  <header>
    <h2>Semantic Search</h2>
    <Sparkles size={18} aria-hidden="true" />
  </header>

  <form onsubmit={submit} role="search" data-testid="nl-search-form">
    <input
      type="search"
      value={store.searchQuery}
      aria-label="Natural language graph query"
      placeholder="Ask the graph"
      disabled={store.searching}
      oninput={(event) => (store.searchQuery = (event.currentTarget as HTMLInputElement).value)}
      data-testid="nl-query"
    />
    <button
      type="submit"
      class="icon-button"
      aria-label="Search graph"
      title="Search graph"
      disabled={store.searching}
      data-testid="run-search"
    >
      <Search size={17} />
    </button>
  </form>

  <div class="suggestions" aria-label="Saved graph queries">
    {#each suggestions as suggestion}
      <button type="button" disabled={store.searching} onclick={() => void store.runSearch(suggestion)}>
        {suggestion}
      </button>
    {/each}
  </div>

  {#if store.searchResult}
    <article class="result" data-testid="search-result">
      <div class="result-head">
        <span data-testid="search-intent">{store.searchResult.intent}</span>
        <strong>{Math.round(store.searchResult.confidence * 100)}%</strong>
      </div>
      <p>{store.searchResult.explanation}</p>
      <ul>
        {#each store.searchResult.supportingFacts as fact}
          <li>{fact}</li>
        {/each}
      </ul>
    </article>
  {/if}
</section>

<style>
  .search-panel {
    display: flex;
    flex-direction: column;
    gap: 12px;
  }

  header,
  .result-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
  }

  h2 {
    margin: 0;
    font-size: 15px;
  }

  form {
    display: grid;
    grid-template-columns: minmax(0, 1fr) 34px;
    gap: 8px;
  }

  input {
    min-width: 0;
    min-height: 34px;
    padding: 0 10px;
    border: 1px solid #cbd3e2;
    border-radius: 7px;
    color: #172033;
    background: #ffffff;
  }

  .suggestions {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
  }

  .suggestions button {
    min-height: 28px;
    padding: 0 8px;
    border: 1px solid #d7dce8;
    border-radius: 7px;
    background: #ffffff;
    color: #41516c;
    cursor: pointer;
    font-size: 12px;
    font-weight: 650;
  }

  .suggestions button:hover {
    border-color: #9ba9c2;
    background: #eef3fb;
  }

  .result {
    border: 1px solid #d7dce8;
    border-radius: 7px;
    background: #fbfcff;
    padding: 10px;
  }

  .result-head span {
    color: #146c73;
    font-size: 12px;
    font-weight: 760;
  }

  .result-head strong {
    color: #243047;
    font-size: 12px;
  }

  p {
    margin: 8px 0;
    color: #3f4f68;
    font-size: 12px;
    line-height: 1.45;
  }

  ul {
    margin: 0;
    padding-left: 18px;
    color: #59677f;
    font-size: 12px;
    line-height: 1.45;
  }
</style>
