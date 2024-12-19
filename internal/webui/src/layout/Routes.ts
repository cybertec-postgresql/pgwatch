import { LoginPage } from "pages/LoginPage/LoginPage";
import { LogsPage } from "pages/LogsPage/LogsPage";
import { MetricsPage } from "pages/MetricsPage/MetricsPage";
import { PresetsPage } from "pages/PresetsPage/PresetsPage";
import { SourcesPage } from "pages/SourcesPage/SourcesPage";

export const publicRoutes = [
  {
    title: "Login",
    link: "/",
    element: LoginPage
  }
];

export const privateRoutes = [
  {
    title: "Sources",
    link: "/sources",
    element: SourcesPage,
  },
  {
    title: "Metrics",
    link: "/metrics",
    element: MetricsPage,
  },
  {
    title: "Presets",
    link: "/presets",
    element: PresetsPage,
  },
  {
    title: "Logs",
    link: "/logs",
    element: LogsPage,
  },
];
