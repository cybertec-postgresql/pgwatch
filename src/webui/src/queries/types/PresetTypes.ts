export type Preset = {
  pc_name: string,
  pc_config: {
    [key: string]: number
  },
  pc_description: string,
  pc_last_modified_on: string
};

export type PresetConfigRows = {
  id: string,
  metric: string,
  interval: number
};

export type CreatePresetConfigForm = {
  pc_name: string,
  pc_description: string,
  pc_config: {
    metric: string,
    update_interval: number
  }[]
};

export type CreatePresetConfigRequestForm = {
  pc_name: string,
  pc_description: string,
  pc_config: {
    [key: string]: number
  }
};

export type UpdatePresetConfigRequestForm = {
  pc_name: string,
  data: CreatePresetConfigRequestForm
};
