import { axiosInstance } from "axiosInstance";
import { CreatePresetConfigRequestForm, UpdatePresetConfigRequestForm } from "queries/types/PresetTypes";


export default class PresetService {
  private static _instance: PresetService;

  public static getInstance(): PresetService {
    if (!PresetService._instance) {
      PresetService._instance = new PresetService();
    }

    return PresetService._instance;
  };

  public async getPresets() {
    return await axiosInstance.get("preset").
      then(response => response.data);
  };

  public async deletePreset(pc_name: string) {
    return await axiosInstance.delete("preset", { params: { "id": pc_name } }).
      then(response => response.data);
  };

  public async addPreset(data: CreatePresetConfigRequestForm) {
    return await axiosInstance.post("preset", data).
      then(response => response);
  };

  public async editPreset(data: UpdatePresetConfigRequestForm) {
    return await axiosInstance.patch("preset", data.data, { params: { "id": data.pc_name } }).
      then(response => response);
  };
};
