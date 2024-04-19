import { MetricsPage } from "pages/MetricsPage/MetricsPage";
import { SignIn } from "./Authentication/SignIn";
import { Dashboard } from "./Dashboard";
import { Logs } from "./Logs";
import { PresetConfigs } from "./PresetConfigs";

export const publicRoutes = [
  {
    title: "Sign in",
    link: "/",
    element: SignIn
  }
];

export const privateRoutes = [
  {
    title: "Dashboard",
    link: "/dashboard",
    element: Dashboard
  },
  {
    title: "Metrics",
    link: "/metrics",
    element: MetricsPage,
  },
  {
    title: "Preset configs",
    link: "/presets",
    element: PresetConfigs,
  },
  {
    title: "Logs",
    link: "/logs",
    element: Logs,
  },
];
