import { useState } from "react";
import Visibility from "@mui/icons-material/Visibility";
import VisibilityOff from "@mui/icons-material/VisibilityOff";
import { IconButton } from "@mui/material";
import { Source } from "types/Source/Source";

type MaskedTextProps = {
  source: Source;
};

const mask = (connStr: string) => {
  if (connStr.includes("://")) {
    return connStr.replace(
      /(postgresql:\/\/[^:]+:)([^@]+)(@.*)/,
      (_, start, pass, end) => `${start}${"•".repeat(8)}${end}`
    );
  } else {
    return connStr.replace(
      /(Password=)[^;]+(;|$)/i, (_, start, end) => `${start}${"•".repeat(8)}${end}`
    );
  }
};

export const MaskConnectionString = ({ source }: MaskedTextProps) => {
  const [unmask, setUnmask] = useState(false);

  const handleUnmask = () => setUnmask(!unmask);

  return (
    <>
      <span>{unmask ? source.ConnStr : mask(source.ConnStr)}</span>
      <IconButton title={unmask ? "Hide password" : "Show password"} onClick={handleUnmask}>
        {
          unmask ? <VisibilityOff /> : <Visibility />
        }
      </IconButton>
    </>
  );
};
