import { MetricsPage } from "pages/MetricsPage/MetricsPage";
import { PresetsPage } from "pages/PresetsPage/PresetsPage";
import { SignIn } from "./Authentication/SignIn";
import { Dashboard } from "./Dashboard";
import { Logs } from "./Logs";
import { StatsSummary } from "./StatsSummary";

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
    title: "Presets",
    link: "/presets",
    element: PresetsPage,
  },
  {
    title: "Stats summary",
    link: "/stats_summary",
    element: StatsSummary,
  },
  {
    title: "Logs",
    link: "/logs",
    element: Logs,
  },
];
