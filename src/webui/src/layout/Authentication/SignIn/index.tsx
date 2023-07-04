import { Box, Button, TextField, Typography } from "@mui/material";
import { NavLink, useNavigate } from "react-router-dom";
import { setToken } from "services/Token";


export const SignIn = () => {
  const navigate = useNavigate();

  const imitateSignIn = () => {
    setToken("AccessToken");
    navigate("/dashboard");
  };

  return (
    <Box sx={{ flex: "1 1 auto", display: "flex", justifyContent: "center", alignItems: "center" }}>
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
        <Typography variant="h3" sx={{ fontWeight: "bold" }}>Login</Typography>
        <TextField
          label="Password"
          type="password"
          fullWidth
        />
        <Button fullWidth variant="contained" onClick={imitateSignIn}>
          sign in
        </Button>
        <NavLink
          key="/sign_up"
          to="/sign_up"
          style={{
            width: "100%"
          }}
        >
          <Button variant="outlined" fullWidth>
            {"don't set password yet?"}
          </Button>
        </NavLink>
      </Box>
    </Box>
  );
};
