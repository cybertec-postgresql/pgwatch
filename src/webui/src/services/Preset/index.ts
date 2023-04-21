import axios from "axios";
import { CreatePresetConfigForm } from "queries/types/PresetTypes";


export default class PresetService {
  private static _instance: PresetService;

  public static getInstance(): PresetService {
    if (!PresetService._instance) {
      PresetService._instance = new PresetService();
    }

    return PresetService._instance;
  };

  public async getPresets() {
    return await axios.get("/preset").
      then(response => response.data).
      catch(error => {
        throw error;
      });
  };

  public async deletePreset(pc_name: string) {
    return await axios.delete("/preset", { params: { "id": pc_name } }).
      then(response => response.data).
      catch(error => {
        throw error;
      });
  };

  public async addPreset(data: CreatePresetConfigForm) {
    return await axios.post("/preset", data).
      then(response => response).
      catch(error => {
        throw error;
      });
  };
}
