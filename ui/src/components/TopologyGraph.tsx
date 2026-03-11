import { useEffect, useCallback } from 'react';
import {
  ColaLayout,
  DefaultEdge,
  DefaultNode,
  EdgeStyle,
  GraphComponent,
  ModelKind,
  NodeShape,
  NodeStatus,
  GRAPH_LAYOUT_END_EVENT,
  TopologyView,
  Visualization,
  VisualizationProvider,
  VisualizationSurface,
  useVisualizationController,
} from '@patternfly/react-topology';
import type {
  ComponentFactory,
  Model,
  NodeModel,
  EdgeModel,
} from '@patternfly/react-topology';
import '@patternfly/react-topology/dist/esm/css/topology-components.css';
import '@patternfly/react-topology/dist/esm/css/topology-view.css';
import type { Component } from '../api/client';

const TYPE_COLORS: Record<string, NodeStatus> = {
  container: NodeStatus.info,
  postgres: NodeStatus.success,
  redis: NodeStatus.warning,
  dns: NodeStatus.default,
  ip: NodeStatus.default,
  vm: NodeStatus.info,
};

const TYPE_SHAPES: Record<string, NodeShape> = {
  container: NodeShape.rect,
  postgres: NodeShape.stadium,
  redis: NodeShape.stadium,
  dns: NodeShape.hexagon,
  ip: NodeShape.hexagon,
  vm: NodeShape.rect,
};

const LAYOUT_ID = 'app-topology-layout';

const componentFactory: ComponentFactory = (kind, type) => {
  switch (kind) {
    case ModelKind.graph:
      return GraphComponent;
    case ModelKind.node:
      return DefaultNode;
    case ModelKind.edge:
      return DefaultEdge;
    default:
      return undefined as never;
  }
};

function buildModel(components: Component[]): Model {
  const nodes: NodeModel[] = components.map(c => ({
    id: c.name,
    type: 'node',
    label: `${c.name} (${c.type})`,
    width: 120,
    height: 50,
    shape: TYPE_SHAPES[c.type] ?? NodeShape.ellipse,
    status: TYPE_COLORS[c.type] ?? NodeStatus.default,
    data: { component: c },
  }));

  const edges: EdgeModel[] = [];
  for (const c of components) {
    if (c.dependsOn) {
      for (const dep of c.dependsOn) {
        edges.push({
          id: `${dep}->${c.name}`,
          type: 'edge',
          source: dep,
          target: c.name,
          edgeStyle: EdgeStyle.default,
        });
      }
    }
    if (c.colocateWith) {
      edges.push({
        id: `${c.name}~${c.colocateWith}`,
        type: 'edge',
        source: c.name,
        target: c.colocateWith,
        edgeStyle: EdgeStyle.dashed,
      });
    }
  }

  return {
    graph: {
      id: 'app-topology',
      type: 'graph',
      layout: LAYOUT_ID,
    },
    nodes,
    edges,
  };
}

function TopologyContent({ components }: { components: Component[] }) {
  const controller = useVisualizationController();

  const handleLayoutEnd = useCallback(() => {
    // Fit the graph to the view once layout completes.
    const graph = controller.getGraph();
    graph.fit(60);
  }, [controller]);

  useEffect(() => {
    controller.addEventListener(GRAPH_LAYOUT_END_EVENT, handleLayoutEnd);
    return () => {
      controller.removeEventListener(GRAPH_LAYOUT_END_EVENT, handleLayoutEnd);
    };
  }, [controller, handleLayoutEnd]);

  useEffect(() => {
    const model = buildModel(components);

    controller.registerLayoutFactory((_type, graph) => {
      return new ColaLayout(graph, { layoutOnDrag: false });
    });
    controller.registerComponentFactory(componentFactory);
    controller.fromModel(model, false);
  }, [controller, components]);

  return (
    <TopologyView>
      <VisualizationSurface />
    </TopologyView>
  );
}

export default function TopologyGraph({ components }: { components: Component[] }) {
  const controller = new Visualization();

  return (
    <div style={{ height: 400, border: '1px solid var(--pf-t--global--border--color--default)' }}>
      <VisualizationProvider controller={controller}>
        <TopologyContent components={components} />
      </VisualizationProvider>
    </div>
  );
}
