import React from "react";
import ReactDOM from "react-dom/client";
import { BrowserRouter } from "react-router-dom";
import { AlertProvider } from "utils/AlertContext";
import App from "./App";

// Read base path from backend-injected configuration
declare global {
  interface Window {
    __PGWATCH_BASE_PATH__?: string;
  }
}

const basename = window.__PGWATCH_BASE_PATH__ || '';

const root = ReactDOM.createRoot(
  document.getElementById("root") as HTMLElement
);
root.render(
  <React.StrictMode>
    <BrowserRouter basename={basename}>
      <AlertProvider>
        <App />
      </AlertProvider>
    </BrowserRouter>
  </React.StrictMode>
);

