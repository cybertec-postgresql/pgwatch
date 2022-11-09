import Dashboard from "./Dashboard";
import { MetricDefinitions } from "./MetricDefinitions";
import { StatsSummary } from "./StatsSummary";

export const routes = [
  {
    title: "DBs",
    link: "/",
    element: Dashboard,
  },
  {
    title: "Metric definitions",
    link: "/metrics",
    element: MetricDefinitions,
  },
  {
    title: "Stats summary",
    link: "/stats_summary",
    element: StatsSummary,
  },
];
