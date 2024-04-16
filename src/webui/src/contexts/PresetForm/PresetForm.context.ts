import { createContext, useContext } from "react";
import { PresetFormContextDefaults } from "./PresetForm.consts";
import { PresetFormContextType } from "./PresetForm.types";

export const PresetFormContext = createContext<PresetFormContextType>(PresetFormContextDefaults);

export const usePresetFormContext = () => useContext(PresetFormContext);
