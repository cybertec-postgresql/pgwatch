import { Box, Typography } from "@mui/material";
import { AppBar as MuiAppBar } from "@mui/material";
import Button from "@mui/material/Button";
import Toolbar from "@mui/material/Toolbar";
import { NavLink } from "react-router-dom";


import { routes } from "./Routes";

export const AppBar = () => {
  const menuLinks = routes.map((item) => (
    <NavLink
      key={item.link}
      to={item.link}
    >
      {({ isActive }) => (
        <Button variant={isActive ? "contained" : "text"} sx={{ color: "white" }} fullWidth>{item.title}</Button>
      )}
    </NavLink>
  ));

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
          </Box>
        </Box>
      </Toolbar>
    </MuiAppBar>
  );
};
