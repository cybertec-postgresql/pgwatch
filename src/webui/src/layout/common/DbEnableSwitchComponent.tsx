import { useState } from "react";

import { Switch } from "@mui/material";
import { useNavigate } from "react-router-dom";
import { useEditEnableDb } from "queries/Dashboard";
import { useAlert } from "utils/AlertContext";


type Props = {
  id: string,
  value: boolean
};

export const DbEnableSwitchComponent = ({ id, value }: Props) => {
  const [checked, setChecked] = useState(value);
  const { callAlert } = useAlert();
  const navigate = useNavigate();

  const editEnable = useEditEnableDb(callAlert, navigate);

  const handleChange = (_event: React.ChangeEvent<HTMLInputElement>, changedValue: boolean) => {
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
