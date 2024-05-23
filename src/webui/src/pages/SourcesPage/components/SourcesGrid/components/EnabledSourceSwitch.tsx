import { useEffect, useState } from "react";
import { Switch } from "@mui/material";
import { useEditSourceEnable } from "queries/Source";


type EnabledSourceSwitchProps = {
  uniqueName: string;
  enabled: boolean;
};

export const EnabledSourceSwitch = (props: EnabledSourceSwitchProps) => {
  const { uniqueName, enabled } = props;

  const [checked, setChecked] = useState(enabled);

  const editEnabled = useEditSourceEnable();
  const { status } = editEnabled;

  const handleChange = (_e: any, checked: boolean) => {
    setChecked(checked);
    editEnabled.mutate(
      {
        uniqueName,
        data: { IsEnabled: checked },
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
