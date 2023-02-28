import { Box, CircularProgress } from "@mui/material";


export const LoadingComponent = () => {
  return (
    <Box sx={{ width: "100%", height: "100%", display: "flex", justifyContent: "center", alignItems: "center" }}>
      <CircularProgress size={60} />
    </Box>
  );
};
