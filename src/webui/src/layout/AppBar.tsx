import LogoutIcon from '@mui/icons-material/Logout';
import { Box, Button, AppBar as MuiAppBar, Toolbar, Typography } from "@mui/material";
import { NavLink, useNavigate } from "react-router-dom";
import { getToken, removeToken } from "services/Token";
import { privateRoutes } from "./Routes";


//import { routes } from "./Routes";

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

  const imitateLogout = () => {
    removeToken();
    navigate("/");
  };

  return (
    <MuiAppBar component="header">
      <Toolbar>
        <Box sx={{ display: "flex", width: "100%", height: 40 }}>
          <Box sx={{ display: "flex", flexGrow: 1, gap: 1.5 }}>
            <a href="https://www.cybertec-postgresql.com/en/">
              <Box sx={{ display: "flex", width: 230, height: 40 }}>
                <img src="/logo.png" />
              </Box>
            </a>
            <Typography variant="h6" component="div">
              pgwatch3
            </Typography>
          </Box>
          <Box sx={{ display: "flex", height: "100%", alignItems: "center" }}>
            {menuLinks}
            {
              token &&
              <Button sx={{ color: "white", border: 0, }} title="Logout" onClick={imitateLogout} >
                <LogoutIcon />
              </Button>
            }
          </Box>
        </Box>
      </Toolbar>
    </MuiAppBar>
  );
};
