import { useWebSocket } from "react-use-websocket/dist/lib/use-websocket";
import { getToken } from "services/Token";

const socketUrl = "ws://localhost:8080/log";
const token = getToken();

export const useLogs = () => useWebSocket(socketUrl, {
  queryParams: {
    "Token": token ? token : ""
  }
});
