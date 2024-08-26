import { useEffect, useState } from "react";
import { Switch } from "@mui/material";
import { Source } from "types/Source/Source";
import { useEditSourceEnable } from "queries/Source";


type EnabledSourceSwitchProps = {
  source: Source;
};

export const EnabledSourceSwitch = ({ source }: EnabledSourceSwitchProps) => {
  const [checked, setChecked] = useState(source.IsEnabled);

  const editEnabled = useEditSourceEnable();
  const { status } = editEnabled;

  const handleChange = (_e: any, value: boolean) => {
    setChecked(value);
    editEnabled.mutate({
      ...source,
      IsEnabled: value,
    });
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
