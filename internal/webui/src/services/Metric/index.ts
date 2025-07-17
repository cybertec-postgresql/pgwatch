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

  public async getMetric(name: string): Promise<any> {
    return await this.api.get(`/metric/${encodeURIComponent(name)}`).
      then(response => response.data);
  };

  public async deleteMetric(name: string) {
    return await this.api.delete(`/metric/${encodeURIComponent(name)}`).
      then(response => response.data);
  };

  public async addMetric(data: MetricRequestBody) {
    // New REST-compliant format: map of metric name to metric data
    const requestData = {
      [data.Name]: data.Data
    };
    return await this.api.post("/metric", requestData).
      then(response => response);
  };

  public async editMetric(data: MetricRequestBody) {
    return await this.api.put(`/metric/${encodeURIComponent(data.Name)}`, data.Data).
      then(response => response);
  };
}
