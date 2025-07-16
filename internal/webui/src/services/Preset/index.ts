import { apiClient } from "api";
import { AxiosInstance } from "axios";
import { PresetRequestBody } from "types/Preset/PresetRequestBody";


export default class PresetService {
  private api: AxiosInstance;
  private static _instance: PresetService;

  constructor() {
    this.api = apiClient();
  }

  public static getInstance(): PresetService {
    if (!PresetService._instance) {
      PresetService._instance = new PresetService();
    }

    return PresetService._instance;
  };

  public async getPresets() {
    return await this.api.get("/preset").
      then(response => response.data);
  };

  public async getPreset(name: string) {
    return await this.api.get(`/preset/${encodeURIComponent(name)}`).
      then(response => response.data);
  };

  public async deletePreset(name: string) {
    return await this.api.delete(`/preset/${encodeURIComponent(name)}`).
      then(response => response.data);
  };

  public async addPreset(data: PresetRequestBody) {
    return await this.api.post("/preset", data.Data, { params: { "name": data.Name } }).
      then(response => response);
  };

  public async editPreset(data: PresetRequestBody) {
    return await this.api.put(`/preset/${encodeURIComponent(data.Name)}`, data.Data).
      then(response => response);
  };
};
