import { useMemo, useState } from "react";

import { Box, Toolbar } from "@mui/material";
import CssBaseline from "@mui/material/CssBaseline";
import { ThemeProvider, createTheme } from "@mui/material/styles";

import { QueryClientProvider } from "QueryClient";

import { Alert } from "components/Alert/Alert";
import { Route, Routes } from "react-router-dom";
import { PrivateRoute } from "layout/PrivateRoute";
import { privateRoutes, publicRoutes } from "layout/Routes";

import { AppBar } from "./layout/AppBar";
import { CopilotDrawer } from "components/CopilotDrawer/CopilotDrawer";
const mdTheme = createTheme();

export default function App() {
  //Initialize the Copilot visibility state
  const [isCopilotOpen, setIsCopilotOpen] = useState(false);
  const publicRoutesItems = useMemo(
    () =>
      publicRoutes.map((route) => (
        <Route key={route.link} path={route.link} element={<route.element />} />
      )),
    []
  );

  const privateRoutesItems = useMemo(
    () =>
      privateRoutes.map((route) => (
        <Route key={route.link} path={route.link} element={<PrivateRoute><route.element /></PrivateRoute>} />
      )),
    []
  );

  return (
    <QueryClientProvider>
      <ThemeProvider theme={mdTheme}>
        <Box sx={{ display: "flex" }}>
          <CssBaseline />
          {/*Pass the toggle function to the AppBar */}
          <AppBar onToggleCopilot={() => setIsCopilotOpen(!isCopilotOpen)} />
          <CopilotDrawer
            open={isCopilotOpen}
            onClose={() => setIsCopilotOpen(false)}
          />
          <Box
            component="main"
            sx={{
              flexGrow: 1,
              height: "100vh",
              overflow: "auto",
              p: 2,
              flexFlow: "column",
              display: "flex"
            }}
          >
            <Toolbar />
            <Routes>
              {publicRoutesItems}
              {privateRoutesItems}
            </Routes>
            <Alert />
          </Box>
        </Box>
      </ThemeProvider>
    </QueryClientProvider>
  );
}
