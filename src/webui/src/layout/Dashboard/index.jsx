import { Box } from "@mui/material";

import { DbsTable } from "./DbsTable";

export default function Dashboard() {
  return (
    <Box sx={{ flex: "1 1 auto" }}>
      <DbsTable />
    </Box>
  );
}
