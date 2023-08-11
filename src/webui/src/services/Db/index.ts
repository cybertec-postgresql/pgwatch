import { axiosInstance } from "axiosInstance";
import { TestConnection, createDbForm, updateDbForm, updateEnabledDbForm } from "queries/types/DbTypes";

export default class DbService {
  private static _instance: DbService;

  public static getInstance(): DbService {
    if (!DbService._instance) {
      DbService._instance = new DbService();
    }

    return DbService._instance;
  };

  public async getMonitoredDb() {
    return await axiosInstance.get("db").
      then(response => response.data);
  };

  public async deleteMonitoredDb(uniqueName: string) {
    return await axiosInstance.delete("db", { params: { "id": uniqueName } }).
      then(response => response.data);
  };

  public async addMonitoredDb(data: createDbForm) {
    return await axiosInstance.post("db", data).
      then(response => response);
  };

  public async editMonitoredDb(data: updateDbForm) {
    return await axiosInstance.patch("db", data.data, { params: { "id": data.md_unique_name } }).
      then(response => response);
  };

  public async editEnabledDb(data: updateEnabledDbForm) {
    return await axiosInstance.patch("db", data.data, { params: { "id": data.md_unique_name } }).
      then(response => response);
  };

  public async testDbConnection(data: TestConnection) {
    const connectionString = Object.entries(data).
      filter(([_key, value]) => value !== "").
      map(([key, value]) => `${key}=${value}`).
      toString().
      replaceAll(",", " ");
    return await axiosInstance.post("test-connect", connectionString);
  };
}
