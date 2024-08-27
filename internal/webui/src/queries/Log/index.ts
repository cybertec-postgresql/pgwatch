import { useWebSocket } from "react-use-websocket/dist/lib/use-websocket";
import { getToken } from "services/Token";

export const useLogs = () => {
  const socketProtocol = window.location.protocol === "https:" ? "wss://" : "ws://";
  const token = getToken() || "";
  
  return useWebSocket(
    `${socketProtocol}${window.location.host}/log`,
    {
      queryParams: {
        "Token": token,
      },
    },
  );
};
