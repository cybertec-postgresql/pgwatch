import { axiosInstance } from "axiosInstance";
import { SourceEditEnabled } from "types/Source/SourceEditEnabled";
import { createDbForm, updateDbForm, updateEnabledDbForm } from "queries/types/DbTypes";

export default class SourceService {
  private static _instance: SourceService;

  public static getInstance(): SourceService {
    if (!SourceService._instance) {
      SourceService._instance = new SourceService();
    }

    return SourceService._instance;
  };

  public async getSources() {
    return await axiosInstance.get("db").
      then(response => response.data);
  };

  public async deleteSource(uniqueName: string) {
    return await axiosInstance.delete("db", { params: { "id": uniqueName } }).
      then(response => response.data);
  };

  public async addSource(data: createDbForm) {
    return await axiosInstance.post("db", data).
      then(response => response);
  };

  public async editSource(data: updateDbForm) {
    return await axiosInstance.patch("db", data.data, { params: { "id": data.md_unique_name } }).
      then(response => response);
  };

  public async editSourceEnable(data: SourceEditEnabled) {
    return await axiosInstance.patch("db", data.data, { params: { "id": data.uniqueName } }).
      then(response => response);
  };

  public async testSourceConnection(data: string) {
    return await axiosInstance.post("test-connect", data);
  };
}
