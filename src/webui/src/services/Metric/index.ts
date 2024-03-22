import { axiosInstance } from "axiosInstance";
import { Metrics } from "layout/MetricDefinitions/MetricDefinitions.types";
import { Metric, createMetricForm, updateMetricForm } from "queries/types/MetricTypes";


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

  public async deleteMetric(m_id: number) {
    return await axiosInstance.delete("metric", { params: { "id": m_id } }).
      then(response => response.data);
  };

  public async addMetric(data: createMetricForm) {
    return await axiosInstance.post("metric", data).
      then(response => response);
  };

  public async editMetric(data: updateMetricForm) {
    return await axiosInstance.patch("metric", data.data, { params: { "id": data.m_id } }).
      then(response => response);
  };
}
