import { useState } from "react";

import { Switch } from "@mui/material";
import { useEditEnableDb } from "queries/Source";


type Props = {
  id: string,
  value: boolean
};

export const DbEnableSwitchComponent = ({ id, value }: Props) => {
  const [checked, setChecked] = useState(value);

  const editEnable = useEditEnableDb();

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
