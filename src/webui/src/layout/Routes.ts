import { SignIn } from "./Authentication/SignIn";
import { SignUp } from "./Authentication/SignUp";
import Dashboard from "./Dashboard";
import { Logs } from "./Logs";
import { MetricDefinitions } from "./MetricDefinitions";
import { PresetConfigs } from "./PresetConfigs";
import { StatsSummary } from "./StatsSummary";

/*export const routes = [
  {
    title: "Databases",
    link: "/dashboard",
    element: Dashboard,
  },
  {
    title: "Metric definitions",
    link: "/metrics",
    element: MetricDefinitions,
  },
  {
    title: "Preset configs",
    link: "/presets",
    element: PresetConfigs,
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
  {
    title: "Sing up",
    link: "/sign_up",
    element: SignUp
  },
  {
    title: "Sign in",
    link: "/sign_in",
    element: SignIn
  }
];*/

export const publicRoutes = [
  {
    title: "Sign in",
    link: "/",
    element: SignIn
  },
  {
    title: "Sign up",
    link: "/sign_up",
    element: SignUp
  }
];

export const privateRoutes = [
  {
    title: "Dashboard",
    link: "/dashboard",
    element: Dashboard
  },
  {
    title: "Metric definitions",
    link: "/metrics",
    element: MetricDefinitions,
  },
  {
    title: "Preset configs",
    link: "/presets",
    element: PresetConfigs,
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
