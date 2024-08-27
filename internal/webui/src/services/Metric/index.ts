import { apiClient } from "api";
import { AxiosInstance } from "axios";
import { Metrics } from "types/Metric/Metric";
import { MetricRequestBody } from "types/Metric/MetricRequestBody";


export default class MetricService {
  private api: AxiosInstance;
  private static _instance: MetricService;

  constructor() {
    this.api = apiClient();
  }

  public static getInstance(): MetricService {
    if (!MetricService._instance) {
      MetricService._instance = new MetricService();
    }

    return MetricService._instance;
  };

  public async getMetrics(): Promise<Metrics> {
    return await this.api.get("/metric").
      then(response => response.data);
  };

  public async deleteMetric(data: string) {
    return await this.api.delete("/metric", { params: { "key": data } }).
      then(response => response.data);
  };

  public async addMetric(data: MetricRequestBody) {
    return await this.api.post("/metric", data.Data, { params: { "name": data.Name } }).
      then(response => response);
  };

  public async editMetric(data: MetricRequestBody) {
    return await this.api.post("/metric", data.Data, { params: { "name": data.Name } }).
      then(response => response);
  };
}
