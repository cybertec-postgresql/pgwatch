import { Switch } from "@mui/material";
import { Source } from "types/Source/Source";
import { useEditSourceEnable } from "queries/Source";


type EnabledSourceSwitchProps = {
  source: Source;
};

export const EnabledSourceSwitch = ({ source }: EnabledSourceSwitchProps) => {
  const editEnabled = useEditSourceEnable();

  const handleChange = (_e: any, value: boolean) => {
    editEnabled.mutate({
      ...source,
      IsEnabled: value,
    });
  };

  const checked = editEnabled.isPending ? !source.IsEnabled : source.IsEnabled;

  return (
    <Switch
      checked={checked}
      onChange={handleChange}
    />
  );
};
