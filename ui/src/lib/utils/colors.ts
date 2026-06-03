import type { ResourceKind } from '$lib/types/demo';

export const KIND_COLORS: Record<ResourceKind, string> = {
  system: '#146c73',
  datastream: '#3366ff',
  observation: '#c0477f',
  controlstream: '#7c4dff',
  feasibility: '#8c4c10',
  property: '#2c7a3f'
};

export const PREDICATE_COLORS: Record<string, string> = {
  'sensorml.system.isHostedBy': '#146c73',
  'csapi.datastream.producedBy': '#3366ff',
  'csapi.datastream.observedProperty': '#2c7a3f',
  'csapi.controlstream.controlsSystem': '#7c4dff',
  'csapi.controlstream.controlledProperty': '#7c4dff',
  'csapi.feasibility.controlstream': '#8c4c10',
  'csapi.observation.datastream': '#c0477f'
};

export function colorForKind(kind: ResourceKind): string {
  return KIND_COLORS[kind] ?? '#647089';
}

export function colorForPredicate(predicate: string): string {
  return PREDICATE_COLORS[predicate] ?? '#8a93a8';
}
