import { axiosInstance } from "axiosInstance";
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
    return await axiosInstance.get("stats")
      .then(response => response.data);
  };
};
