import { ComponentProps, forwardRef, useState } from "react";
import VisibilityIcon from "@mui/icons-material/Visibility";
import VisibilityOffIcon from "@mui/icons-material/VisibilityOff";
import { IconButton, OutlinedInput } from "@mui/material";

export const PasswordInput = forwardRef<HTMLInputElement, ComponentProps<typeof OutlinedInput>>((props, ref) => {
  const [isVisible, setisVisible] = useState(false);

  const handleVisibilityChange = () => {
    setisVisible((prev) => !prev);
  };

  return (
    <OutlinedInput
      {...props}
      ref={ref}
      type={isVisible ? "text" : "password"}
      endAdornment={
        <IconButton onClick={handleVisibilityChange}>
          {isVisible ? <VisibilityOffIcon /> : <VisibilityIcon />}
        </IconButton>
      }
    />
  );
});

