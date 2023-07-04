export type StatsSummary = {
  main: {
    [key: string]: string | number;
  },
  metrics: {
    [key: string]: string | number | null;
  },
  datastore: {
    [key: string]: string | number | null;
  },
  general: {
    [key: string]: string | number | null;
  }
};
