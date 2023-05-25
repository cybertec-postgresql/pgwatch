import { useWebSocket } from "react-use-websocket/dist/lib/use-websocket";


const socketUrl = "ws://localhost:8080/log";

export const useLogs = () => useWebSocket(socketUrl);
