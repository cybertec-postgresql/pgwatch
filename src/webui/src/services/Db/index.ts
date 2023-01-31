import axios from "axios";

export default class DbService {
  private static _instance: DbService;

  public static getInstance(): DbService {
    if (!DbService._instance) {
      DbService._instance = new DbService();
    }

    return DbService._instance;
  }

  public async getMonitoredDb() {
    return await axios.get("/db").
      then(result => result.data).
      catch(error => {
        throw error;
      });
  };

  public async deleteMonitoredDb(uniqueName: string) {
    return await axios.delete("/db", { params: { "id": uniqueName } }).
      then(result => result.data).
      catch(error => {
        throw error;
      });
  };
}
