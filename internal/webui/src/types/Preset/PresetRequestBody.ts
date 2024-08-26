export type PresetRequestBody = {
  Name: string;
  Data: {
    Description?: string;
    Metrics: Record<string, number>;
  },
};
