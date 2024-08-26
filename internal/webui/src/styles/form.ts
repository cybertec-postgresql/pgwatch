import { makeStyles } from "tss-react/mui";

export const useFormStyles = makeStyles()(
  () => ({
    formDialog: {
      "& .MuiPaper-root": {
        maxWidth: "750px",
      },
    },
    formContent: {
      maxWidth: "550px",
      width: "550px",
    },
    form: {
      paddingTop: "15px",
      width: "100%",
      display: "flex",
      flexFlow: "column",
      gap: "10px",
    },
    row: {
      display: "flex",
      width: "100%",
      alignItems: "flex-start",
      justifyContent: "space-between",
    },
    formControlInput: {
      display: "flex",
      "&$formControlBlock": {
        display: "block",
      },
      "& .MuiFormHelperText-root": {
        margin: "0px",
        paddingLeft: "5px",
      },
    },
    formControlCheckbox: {
      "&.MuiFormControlLabel-root": {
        flexDirection: "unset",
        marginLeft: "0px",
        width: "fit-content",
      },
    },
    addButton: {
      justifyContent: "end",
    },
    formButtons: {
      "&.MuiDialogActions-root": {
        padding: "0px 8px 8px",
      },
    },
    widthDefault: {
      maxWidth: "270px",
      width: "100%",
    },
    widthFull: {
      maxWidth: "100%",
      width: "100%",
    },
    hidden: {
      display: "none",
    }
  }),
);
