import { useWebSocket } from "react-use-websocket/dist/lib/use-websocket";


const socketUrl = "ws://localhost:8080";

export const useLogs = () => useWebSocket(socketUrl);
