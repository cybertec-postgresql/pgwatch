export type Preset = {
  pc_name: string,
  pc_config: Object,
  pc_description: string,
  pc_last_modified_on: string
};

export type PresetConfigRows = {
  id: string,
  metric: string,
  interval: number
};
