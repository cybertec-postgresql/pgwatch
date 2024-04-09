export type Metric = {
  SQLs: Record<number, string>;
  Enabled: boolean;
  InitSQL: string;
  NodeStatus: string;
  Gauges: string[];
  IsInstanceLevel: boolean;
  StorageName: string;
  Description: string;
};

export type Metrics = Record<string, Metric>;

export type MetricRow = {
  Key: string;
  Metric: Metric;
};
