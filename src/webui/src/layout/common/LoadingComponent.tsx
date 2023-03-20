import { Box } from "@mui/material";

import style from "./style.module.css";


export const LoadingComponent = () => {
  return (
    <Box sx={{ width: "100%", height: "100%", display: "flex", flexFlow: "column", gap:2, justifyContent: "center", alignItems: "center" }}>
      <img src="/logo192.png" height={120} width={120} className={style.rotating} />
      <img src="/loadingLogo.png" width={200} height={45} />
    </Box>
  );
};
