import { createContext, useContext, useState } from "react";
import { AlertColor } from "@mui/material";

export type AlertContextType = {
  open: boolean;
  severity: AlertColor;
  message: string;
  callAlert: (alertSeverity: AlertColor, alertMessage: string) => void;
  closeAlert: () => void;
};

type AlertProviderProps = {
  children: JSX.Element;
};

export const AlertContext = createContext<AlertContextType | null>(null);

export const AlertProvider = ({ children }: AlertProviderProps) => {
  const [open, setOpen] = useState(false);
  const [severity, setSeverity] = useState<AlertColor>("success");
  const [message, setMessage] = useState("");

  const callAlert = (alertSeverity: AlertColor, alertMessage: string) => {
    setSeverity(alertSeverity);
    setMessage(alertMessage);
    setOpen(true);
  };

  const closeAlert = () => {
    setOpen(false);
  };

  return (
    <AlertContext.Provider value={{ open, severity, message, callAlert, closeAlert }}>
      {children}
    </AlertContext.Provider>
  );
};

export const useAlert = () => useContext(AlertContext) as AlertContextType;
