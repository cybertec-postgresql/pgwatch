export type Preset = {
  Description: string;
  Metrics: Record<string, number>;
  SortOrder: number;
};

export type Presets = Record<string, Preset>;
