import { Box } from "@mui/material";

import { DbsTable } from "./DbsTable";

export const Dashboard = () => {
  return (
    <Box sx={{ flex: "1 1 auto" }}>
      <DbsTable />
    </Box>
  );
};
