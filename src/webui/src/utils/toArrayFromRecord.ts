export type ArrayFromRecord<T> = {
  Name: string;
  Value: T
};

export const toArrayFromRecord = <T extends string | number>(record?: Record<string, T> | null): ArrayFromRecord<T>[] => {
  if (record) {
    return Object.keys(record).map((key) => ({
      Name: key,
      Value: record[key],
    }));
  }
  return [];
};
