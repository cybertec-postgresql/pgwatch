import { useWebSocket } from "react-use-websocket/dist/lib/use-websocket";
import { getToken } from "services/Token";

const socketUrl = `ws://${window.location.host}/log`;
const token = getToken();

export const useLogs = () => useWebSocket(socketUrl, {
  queryParams: {
    "Token": token ? token : ""
  }
});
