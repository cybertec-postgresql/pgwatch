import axios from "axios";
import { createMetricForm, updateMetricForm } from "queries/types/MetricTypes";


export default class MetricService {
  private static _instance: MetricService;

  public static getInstance(): MetricService {
    if (!MetricService._instance) {
      MetricService._instance = new MetricService();
    }

    return MetricService._instance;
  };

  public async getMetrics() {
    return await axios.get("/metric").
      then(response => response.data).
      catch(error => {
        throw error;
      });
  };

  public async deleteMetric(m_id: number) {
    return await axios.delete("/metric", { params: { "id": m_id } }).
      then(response => response.data).
      catch(error => {
        throw error;
      });
  };

  public async addMetric(data: createMetricForm) {
    return await axios.post("/metric", data).
      then(response => response).
      catch(error => {
        throw error;
      });
  };

  public async editMetric(data: updateMetricForm) {
    return await axios.patch("/metric", data.data, { params: { "id": data.m_id } }).
      then(response => response).
      catch(error => {
        throw error;
      });
  };
}
