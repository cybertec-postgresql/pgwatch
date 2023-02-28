import { Box, Typography } from "@mui/material";

type Props = {
  errorMessage: string
}

export const ErrorComponent = ({ errorMessage }: Props) => {
  return (
    <Box sx={{ width: "100%", height: "100%", display: "flex", flexDirection: "column", justifyContent: "center", alignItems: "center", gap: 3 }}>
      <Typography variant="h3" fontWeight="bold">Oops!</Typography>
      <Typography variant="h5">Something went wrong...</Typography>
      <Typography>{errorMessage}</Typography>
    </Box>
  );
};
