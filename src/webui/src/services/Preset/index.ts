import { axiosInstance } from "axiosInstance";
import { PresetRequestBody } from "types/Preset/PresetRequestBody";


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

  public async deletePreset(name: string) {
    return await axiosInstance.delete("preset", { params: { name } }).
      then(response => response.data);
  };

  public async addPreset(data: PresetRequestBody) {
    return await axiosInstance.post("preset", data.Data, { params: { "name": data.Name } }).
      then(response => response);
  };

  public async editPreset(data: PresetRequestBody) {
    return await axiosInstance.patch("preset", data.Data, { params: { "name": data.Name } }).
      then(response => response);
  };
};
