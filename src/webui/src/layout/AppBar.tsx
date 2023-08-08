import LogoutIcon from '@mui/icons-material/Logout';
import { Box, Button, AppBar as MuiAppBar, Toolbar, Typography } from "@mui/material";
import { NavLink, useNavigate } from "react-router-dom";
import { logout } from 'queries/Auth';
import { getToken } from "services/Token";
import { privateRoutes } from "./Routes";

export const AppBar = () => {
  const navigate = useNavigate();
  const token = getToken();

  const menuLinks = privateRoutes.map((item) => (
    <NavLink
      key={item.link}
      to={item.link}
    >
      {({ isActive }) => (
        <Button variant={isActive ? "contained" : "outlined"} sx={{ color: "white" }} style={{ border: 0 }} fullWidth>{item.title}</Button>
      )}
    </NavLink>
  ));

  const handleLogout = () => {
    logout(navigate);
  };

  return (
    <MuiAppBar component="header">
      <Toolbar>
        <Box sx={{ display: "flex", width: "100%", height: 40, alignItems: "center" }}>
          <Box sx={{ display: "flex", gap: 0.75, flexGrow: 1, height: "100%" }}>
            <a href="https://www.cybertec-postgresql.com/en/">
              <Box sx={{ display: "flex", width: 230, height: 40 }}>
                <img src="/logo.png" />
              </Box>
            </a>
            <Typography sx={{ height: "100%", alignItems: "center", display: "flex" }}>
              PGWATCH3
            </Typography>
          </Box>
          {
            token &&
            <Box sx={{ display: "flex", height: "100%", alignItems: "center" }}>
              {menuLinks}
              <Button sx={{ color: "white", border: 0, }} title="Logout" onClick={handleLogout} >
                <LogoutIcon />
              </Button>
            </Box>
          }
        </Box>
      </Toolbar>
    </MuiAppBar>
  );
};
