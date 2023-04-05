import axios from "axios";
import { createDbForm, updateDbForm, updateEnabledDbForm } from "queries/types/DbTypes";

export default class DbService {
  private static _instance: DbService;

  public static getInstance(): DbService {
    if (!DbService._instance) {
      DbService._instance = new DbService();
    }

    return DbService._instance;
  };

  public async getMonitoredDb() {
    return await axios.get("/db").
      then(response => response.data).
      catch(error => {
        throw error;
      });
  };

  public async deleteMonitoredDb(uniqueName: string) {
    return await axios.delete("/db", { params: { "id": uniqueName } }).
      then(response => response.data).
      catch(error => {
        throw error;
      });
  };

  public async addMonitoredDb(data: createDbForm) {
    return await axios.post("/db", data).
      then(response => response).
      catch(error => {
        throw error;
      });
  };

  public async editMonitoredDb(data: updateDbForm) {
    return await axios.patch("/db", data.data, { params: { "id": data.md_unique_name } }).
      then(response => response).
      catch(error => {
        throw error;
      });
  };

  public async editEnabledDb(data: updateEnabledDbForm) {
    return await axios.patch("/db", data.data, { params: { "id": data.md_unique_name } }).
      then(response => response).
      catch(error => {
        throw error;
      });
  };
}
