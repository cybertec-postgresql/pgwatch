import { Box } from "@mui/material";

import LogGrid from "./LogGrid";
import TopPanel from "./TopPanel";

import style from "./style.module.css";

export const Logs = () => {
  return (
    <Box className={style.Logs}>
      <TopPanel />
      <LogGrid />
    </Box>
  );
};
