import { axiosInstance } from "axiosInstance";
import { Source } from "types/Source/Source";
import { SourceRequestBody } from "types/Source/SourceRequestBody";

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
    return await axiosInstance.delete("source", { params: { "name": uniqueName } }).
      then(response => response.data);
  };

  public async addSource(data: Source) {
    return await axiosInstance.post("source", data).
      then(response => response);
  };

  public async editSource(data: SourceRequestBody) {
    return await axiosInstance.post("source", data.data, { params: { "name": data.Name } }).
      then(response => response);
  };

  public async editSourceEnable(data: Source) {
    return await axiosInstance.post("source", data, { params: { "name": data.Name } }).
      then(response => response);
  };

  public async editSourceHostConfig(data: Source) {
    return await axiosInstance.post("source", data, { params: { "name": data.Name } });
  };

  public async testSourceConnection(data: string) {
    return await axiosInstance.post("test-connect", data);
  };
}
