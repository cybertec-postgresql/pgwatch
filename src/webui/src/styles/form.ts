export const formDialog = {
    "& .MuiPaper-root": {
      maxWidth: "750px",
    },
};
  
export const formContent = {
    maxWidth: "550px",
    width: "550px",
    maxHeight: "650px",
};
  
export const form = {
    paddingTop: "15px",
    width: "100%",
    display: "flex",
    flexFlow: "column",
    gap: "10px",
};

export const formControlInput = {
    display: "flex",
    "&$formControlBlock": {
      display: "block",
    },
    "& .MuiFormHelperText-root": {
      margin: "0px",
      paddingLeft: "5px",
    },
};

export const formControlCheckbox = {
    "&.MuiFormControlLabel-root": {
      flexDirection: "unset",
      marginLeft: "0px",
      width: "fit-content",
    },
};

export const formButtons = {
    "&.MuiDialogActions-root": {
      padding: "0px 8px 8px",
    },
};

export const widthDefault = {
    maxWidth: "240px",
    width: "100%",
};

export const widthFull = {
    maxWidth: "100%",
    width: "100%",
};