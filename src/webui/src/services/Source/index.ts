import { axiosInstance } from "axiosInstance";
import { Source } from "types/Source/Source";
import { SourceEditEnabled } from "types/Source/SourceEditEnabled";
import { SourceEditHostConfig } from "types/Source/SourceEditHostConfig";

export default class SourceService {
  private static _instance: SourceService;

  public static getInstance(): SourceService {
    if (!SourceService._instance) {
      SourceService._instance = new SourceService();
    }

    return SourceService._instance;
  };

  public async getSources() {
    return await axiosInstance.get("source").
      then(response => response.data);
  };

  public async deleteSource(uniqueName: string) {
    return await axiosInstance.delete("source", { params: { "id": uniqueName } }).
      then(response => response.data);
  };

  public async addSource(data: Source) {
    return await axiosInstance.post("source", data).
      then(response => response);
  };

  public async editSource(data: Source) {
    return await axiosInstance.patch("source", data, { params: { "id": data.DBUniqueName } }).
      then(response => response);
  };

  public async editSourceEnable(data: SourceEditEnabled) {
    return await axiosInstance.patch("source", data.data, { params: { "id": data.DBUniqueName } }).
      then(response => response);
  };

  public async editSourceHostConfig(data: SourceEditHostConfig) {
    return await axiosInstance.patch("source", data.data, { params: { "id": data.DBUniqueName } });
  };

  public async testSourceConnection(data: string) {
    return await axiosInstance.post("test-connect", data);
  };
}
