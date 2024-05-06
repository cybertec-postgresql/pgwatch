export type Preset = {
  Description: string;
  Metrics: Record<string, number>;
};

export type Presets = Record<string, Preset>;
