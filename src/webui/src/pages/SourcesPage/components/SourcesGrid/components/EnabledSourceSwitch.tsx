import { useEffect, useState } from "react";
import { Switch } from "@mui/material";
import { useEditSourceEnable } from "queries/Source";


type EnabledSourceSwitchProps = {
  DBUniqueName: string;
  IsEnabled: boolean;
};

export const EnabledSourceSwitch = (props: EnabledSourceSwitchProps) => {
  const { DBUniqueName, IsEnabled } = props;

  const [checked, setChecked] = useState(IsEnabled);

  const editEnabled = useEditSourceEnable();
  const { status } = editEnabled;

  const handleChange = (_e: any, value: boolean) => {
    setChecked(value);
    editEnabled.mutate(
      {
        DBUniqueName,
        data: { IsEnabled: value },
      }
    );
  };

  useEffect(() => {
    if (status === "error") {
      setChecked((prev) => !prev);
    }
  }, [status]);

  return (
    <Switch
      checked={checked}
      onChange={handleChange}
    />
  );
};
