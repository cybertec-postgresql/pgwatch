export type StatsSummary = {
  main: {
    [key: string]: string | number;
  },
  metrics: {
    [key: string]: string | number;
  },
  datastore: {
    [key: string]: string | number;
  },
  general: {
    [key: string]: string | number;
  }
};
