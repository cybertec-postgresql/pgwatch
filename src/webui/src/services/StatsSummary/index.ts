import axios from "axios";
import { StatsSummary } from "queries/types/StatsSummaryTypes";


export default class StatsSummaryService {
  private static _instance: StatsSummaryService;

  public static getInstance(): StatsSummaryService {
    if (!StatsSummaryService._instance) {
      StatsSummaryService._instance = new StatsSummaryService();
    }

    return StatsSummaryService._instance;
  };

  public async getStatsSummary(): Promise<StatsSummary> {
    return await axios.get("/stats")
      .then(response => response.data)
      .catch(error => {
        throw error;
      });
  };
};
