
export type MetricRequestBody = {
  Name: string;
  Data: {
    StorageName?: string | null;
    NodeStatus?: string | null;
    Description?: string | null;
    Gauges?: string[] | null;
    InitSQL?: string | null;
    IsInstanceLevel: boolean;
    SQLs: Record<number, string>;
  };
};
