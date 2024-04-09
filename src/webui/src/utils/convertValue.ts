export const convertValue = (value: any): any => {
  if (typeof value === "object" && value !== null) {
    return JSON.stringify(value);
  }
  return value;
};
