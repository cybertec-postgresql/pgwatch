import { createContext, useContext } from "react";
import { MetricFormContextDefaults } from "./MetricForm.consts";
import { MetricFormContextType } from "./MetricForm.types";

export const MetricFormContext = createContext<MetricFormContextType>(MetricFormContextDefaults);

export const useMetricFormContext = () => useContext(MetricFormContext);
