export type Metric = {
  SQLs: Record<number, string>;
  InitSQL: string;
  NodeStatus: string;
  Gauges: string[];
  IsInstanceLevel: boolean;
  StorageName: string;
  Description: string;
};

export type Metrics = Record<string, Metric>;
