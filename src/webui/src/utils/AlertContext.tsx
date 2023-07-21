import { createContext, useContext, useState } from "react";
import { AlertColor, SnackbarCloseReason } from "@mui/material";

export type AlertContextType = {
  open: boolean;
  severity: AlertColor;
  message: string;
  callAlert: (alertSeverity: AlertColor, alertMessage: string) => void;
  closeAlert: (event: Event | React.SyntheticEvent<any, Event>, reason: SnackbarCloseReason) => void;
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

  const closeAlert = (_event: Event | React.SyntheticEvent<any, Event>, reason: SnackbarCloseReason) => {
    if (reason === "clickaway" || reason === "escapeKeyDown") {
      return;
    }
    setOpen(false);
  };

  return (
    <AlertContext.Provider value={{ open, severity, message, callAlert, closeAlert }}>
      {children}
    </AlertContext.Provider>
  );
};

export const useAlert = () => useContext(AlertContext) as AlertContextType;
