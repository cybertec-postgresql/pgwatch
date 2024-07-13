import { HostConfigFormValues } from "./HostConfigPopUp.types";

export const getHostConfigInitialValues = (data?: object): HostConfigFormValues => ({
  HostConfig: JSON.stringify(data ?? {}, undefined, 2),
});
