import { ArrayFromRecord } from "./toArrayFromRecord";

export const toRecordFromArray = <T extends string | number>(arr?: ArrayFromRecord<T>[] | null) => {
  if (arr) {
    const record: Record<string, T> = {};
    arr.map((val) => record[val.Name] = val.Value);
    return record;
  }
  return {};
};
