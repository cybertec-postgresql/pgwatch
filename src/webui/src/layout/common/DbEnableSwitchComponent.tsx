import { useState } from "react";

import { Switch } from "@mui/material";
import { UseMutationResult } from "@tanstack/react-query";
import { AxiosResponse } from "axios";
import { updateEnabledDbForm } from "queries/types/DbTypes";


type Props = {
  id: string,
  value: boolean,
  editEnable: UseMutationResult<AxiosResponse<any, any>, any, updateEnabledDbForm, unknown>
};

export const DbEnableSwitchComponent = ({ id, value, editEnable }: Props) => {
  const [checked, setChecked] = useState(value);

  const handleChange = (event: React.ChangeEvent<HTMLInputElement>, changedValue: boolean) => {
    setChecked(changedValue);
    editEnable.mutate({ md_unique_name: id, data: { md_is_enabled: changedValue } });
  };

  return (
    <Switch
      checked={checked}
      onChange={handleChange}
    />
  );
};
