import type AbstractGraph from 'graphology';
import FA2Layout from 'graphology-layout-forceatlas2/worker';

type Graph = AbstractGraph;

const SETTINGS = {
  gravity: 0.7,
  scalingRatio: 2.4,
  slowDown: 5,
  barnesHutOptimize: true,
  barnesHutTheta: 0.5,
  adjustSizes: true
};

export class LayoutController {
  private layout: FA2Layout | null = null;
  private stopTimer: ReturnType<typeof setTimeout> | null = null;

  start(graph: Graph): void {
    if (typeof window === 'undefined' || graph.order === 0) return;

    this.stop();
    this.layout = new FA2Layout(graph, { settings: SETTINGS });
    this.layout.start();
    this.stopTimer = setTimeout(() => this.stop(), 2600);
  }

  stop(): void {
    if (this.stopTimer) {
      clearTimeout(this.stopTimer);
      this.stopTimer = null;
    }
    if (this.layout) {
      this.layout.stop();
      this.layout.kill();
      this.layout = null;
    }
  }
}
