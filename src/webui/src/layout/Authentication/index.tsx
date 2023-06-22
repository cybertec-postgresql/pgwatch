import { Box, Button, Stack, TextField, Typography } from "@mui/material";

type Props = {
  action: "SIGN_UP" | "SIGN_IN"
};

export const Authentication = ({ action }: Props) => {
  return (
    <Box sx={{ flex: "1 1 auto", justifyContent: "center", alignItems: "center", display: "flex" }}>
      <Box
        sx={{
          width: "30%",
          height: "40%",
          display: "flex",
          alignItems: "center",
          justifyContent: "space-around",
          flexDirection: "column"
        }}
      >
        <Typography variant="h3" sx={{ fontWeight: "bold" }}>{action === "SIGN_IN" ? "Login" : "Set password"}</Typography>
        <TextField
          label="Password"
          type="password"
          fullWidth
        />
        <Button fullWidth variant="contained">
          {action === "SIGN_IN" ? "sign in" : "set password"}
        </Button>
      </Box>
    </Box>
  );
};
