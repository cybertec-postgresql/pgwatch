import { createContext, useContext } from "react";
import { SourceFormContextDefaults } from "./SourceForm.consts";
import { SourceFormContextType } from "./SourceForm.types";

export const SourceFormContext = createContext<SourceFormContextType>(SourceFormContextDefaults);

export const useSourceFormContext = () => useContext(SourceFormContext);
