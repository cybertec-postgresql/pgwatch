import { makeStyles } from "@mui/styles";

export const useFormStyles = makeStyles(() => ({
  formDialog: {
    "& .MuiPaper-root": {
      maxWidth: "750px",
    }
  },
  formContent: {
    maxWidth: "550px",
    width: "550px",
    maxHeight: "650px",
  },
  form: {
    paddingTop: "15px",
    width: "100%",
    display: "flex",
    flexFlow: "column",
    gap: "10px",
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
  formButtons: {
    "&.MuiDialogActions-root": {
      padding: "0px 8px 8px",
    },
  },
  widthDefault: {
    maxWidth: "240px",
    width: "100%",
  },
  widthFull: {
    maxWidth: "100%",
    width: "100%",
  },
}));
