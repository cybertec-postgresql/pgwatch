import { useWebSocket } from "react-use-websocket/dist/lib/use-websocket";
import { getToken } from "services/Token";

export const useLogs = () => {
  const socketProtocol = window.location.protocol === "https:" ? "wss://" : "ws://";
  const token = getToken() || "";
  const basePath = (window as any).__PGWATCH_BASE_PATH__ || "";
  
  return useWebSocket(
    `${socketProtocol}${window.location.host}${basePath}/log`,
    {
      queryParams: {
        "Token": token,
      },
    },
  );
};
