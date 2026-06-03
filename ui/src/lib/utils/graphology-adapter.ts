import type AbstractGraph from 'graphology';
import type { DemoEntity, DemoRelationship } from '$lib/types/demo';
import { colorForKind, colorForPredicate } from '$lib/utils/colors';

type Graph = AbstractGraph;

export function syncDemoGraph(
  graph: Graph,
  entities: DemoEntity[],
  relationships: DemoRelationship[]
): void {
  const positions = new Map<string, { x: number; y: number }>();
  graph.forEachNode((id: string, attrs: Record<string, unknown>) => {
    positions.set(id, { x: Number(attrs.x), y: Number(attrs.y) });
  });

  graph.clear();

  for (const entity of entities) {
    const previous = positions.get(entity.id);
    graph.addNode(entity.id, {
      label: truncate(entity.label),
      size: nodeSize(entity, relationships),
      color: colorForKind(entity.kind),
      entityKind: entity.kind,
      x: previous?.x ?? seededPosition(entity.id, 'x'),
      y: previous?.y ?? seededPosition(entity.id, 'y')
    });
  }

  for (const relationship of relationships) {
    if (!graph.hasNode(relationship.sourceId) || !graph.hasNode(relationship.targetId)) continue;
    if (graph.hasEdge(relationship.id)) continue;

    graph.addEdgeWithKey(relationship.id, relationship.sourceId, relationship.targetId, {
      label: relationship.label,
      color: colorForPredicate(relationship.predicate),
      size: 2,
      type: 'arrow'
    });
  }
}

function nodeSize(entity: DemoEntity, relationships: DemoRelationship[]): number {
  const degree = relationships.filter(
    (relationship) => relationship.sourceId === entity.id || relationship.targetId === entity.id
  ).length;
  const base = entity.kind === 'observation' ? 6 : 10;
  return Math.min(base + Math.sqrt(degree) * 2, 20);
}

function truncate(label: string): string {
  if (label.length <= 28) return label;
  return `${label.slice(0, 27)}...`;
}

function seededPosition(id: string, axis: 'x' | 'y'): number {
  let hash = axis === 'x' ? 17 : 31;
  for (let i = 0; i < id.length; i += 1) {
    hash = (hash * 33 + id.charCodeAt(i)) % 997;
  }
  return (hash / 997) * 220 - 110;
}
