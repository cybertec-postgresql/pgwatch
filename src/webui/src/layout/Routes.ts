import { MetricsPage } from "pages/MetricsPage/MetricsPage";
import { PresetsPage } from "pages/PresetsPage/PresetsPage";
import { SourcesPage } from "pages/SourcesPage/SourcesPage";
import { SignIn } from "./Authentication/SignIn";
import { Logs } from "./Logs";

export const publicRoutes = [
  {
    title: "Sign in",
    link: "/",
    element: SignIn
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
    element: Logs,
  },
];
