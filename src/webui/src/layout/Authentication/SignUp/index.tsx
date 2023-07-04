import { Box, Button, TextField, Typography } from "@mui/material";
import { NavLink, useNavigate } from "react-router-dom";
import { setToken } from "services/Token";


export const SignUp = () => {
  const navigate = useNavigate();

  const imitateSignUp = () => {
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
        <Typography variant="h3" sx={{ fontWeight: "bold" }}>Set password</Typography>
        <TextField
          label="Password"
          type="password"
          fullWidth
        />
        <Button fullWidth variant="contained" onClick={imitateSignUp}>
          set password
        </Button>
        <NavLink
          key="/sign_in"
          to="/"
          style={{
            width: "100%"
          }}
        >
          <Button variant="outlined" fullWidth>
            {"already set password?"}
          </Button>
        </NavLink>
      </Box>
    </Box>
  );
};
