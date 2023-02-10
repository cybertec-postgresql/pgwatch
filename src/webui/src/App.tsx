import { useMemo } from "react";

import { Box, Toolbar } from "@mui/material";
import CssBaseline from "@mui/material/CssBaseline";
import { ThemeProvider, createTheme } from "@mui/material/styles";

import { QueryClientProvider } from "@tanstack/react-query";
import { queryClient } from "queryClient";

import { Route, Routes } from "react-router-dom";

import { AppBar } from "./layout/AppBar";
import { routes } from "./layout/Routes";

const mdTheme = createTheme();

export default function App() {
  const routesItems = useMemo(
    () =>
      routes.map((route) => (
        <Route key={route.link} path={route.link} element={route.element()} />
      )),
    []
  );

  return (
    <QueryClientProvider client={queryClient}>
      <ThemeProvider theme={mdTheme}>
        <Box sx={{ display: "flex" }}>
          <CssBaseline />
          <AppBar />
          <Box
            component="main"
            sx={{
              flexGrow: 1,
              height: "100vh",
              overflow: "auto",
              p: 2,
            }}
          >
            <Toolbar />
            <Routes>{routesItems}</Routes>
          </Box>
        </Box>
      </ThemeProvider>
    </QueryClientProvider>
  );
}
