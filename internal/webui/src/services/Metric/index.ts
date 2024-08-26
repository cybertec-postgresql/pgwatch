import { axiosInstance } from "axiosInstance";
import { Metrics } from "types/Metric/Metric";
import { MetricRequestBody } from "types/Metric/MetricRequestBody";


export default class MetricService {
  private static _instance: MetricService;

  public static getInstance(): MetricService {
    if (!MetricService._instance) {
      MetricService._instance = new MetricService();
    }

    return MetricService._instance;
  };

  public async getMetrics(): Promise<Metrics> {
    return await axiosInstance.get("metric").
      then(response => response.data);
  };

  public async deleteMetric(data: string) {
    return await axiosInstance.delete("metric", { params: { "key": data } }).
      then(response => response.data);
  };

  public async addMetric(data: MetricRequestBody) {
    return await axiosInstance.post("metric", data.Data, { params: { "name": data.Name } }).
      then(response => response);
  };

  public async editMetric(data: MetricRequestBody) {
    return await axiosInstance.post("metric", data.Data, { params: { "name": data.Name } }).
      then(response => response);
  };
}
