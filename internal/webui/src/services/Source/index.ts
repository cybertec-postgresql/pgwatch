import { apiClient } from "api";
import { AxiosInstance } from "axios";
import { Source } from "types/Source/Source";
import { SourceRequestBody } from "types/Source/SourceRequestBody";

export default class SourceService {
  private api: AxiosInstance;
  private static _instance: SourceService;

  constructor() {
    this.api = apiClient();
  }

  public static getInstance(): SourceService {
    if (!SourceService._instance) {
      SourceService._instance = new SourceService();
    }

    return SourceService._instance;
  };

  public async getSources() {
    return await this.api.get("/source").
      then(response => response.data);
  };

  public async getSource(uniqueName: string) {
    return await this.api.get(`/source/${encodeURIComponent(uniqueName)}`).
      then(response => response.data);
  };

  public async deleteSource(uniqueName: string) {
    return await this.api.delete(`/source/${encodeURIComponent(uniqueName)}`).
      then(response => response.data);
  };

  public async addSource(data: Source) {
    return await this.api.post("/source", data).
      then(response => response);
  };

  public async editSource(data: SourceRequestBody) {
    return await this.api.put(`/source/${encodeURIComponent(data.Name)}`, data.data).
      then(response => response);
  };

  public async editSourceEnable(data: Source) {
    return await this.api.put(`/source/${encodeURIComponent(data.Name)}`, data).
      then(response => response);
  };

  public async editSourceHostConfig(data: Source) {
    return await this.api.put(`/source/${encodeURIComponent(data.Name)}`, data);
  };

  public async testSourceConnection(data: string) {
    return await this.api.post("/test-connect", data);
  };
}
