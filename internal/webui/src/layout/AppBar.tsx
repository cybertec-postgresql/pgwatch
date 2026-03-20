import LogoutIcon from '@mui/icons-material/Logout';
import { Box, Button, AppBar as MuiAppBar, Toolbar, Typography, IconButton } from "@mui/material";
import { NavLink, useNavigate } from "react-router-dom";
import { logout } from 'queries/Auth';
import { getToken } from "services/Token";
import { privateRoutes } from "./Routes";
import { AutoAwesome } from '@mui/icons-material';

export const AppBar = ({ onToggleCopilot }: { onToggleCopilot: () => void }) => {
  const navigate = useNavigate();
  const token = getToken();

  const menuLinks = privateRoutes.map((item) => (
    <NavLink key={item.link} to={item.link} style={{ textDecoration: 'none' }}>
      {({ isActive }) => (
        <Button variant="text" sx={{ color: "white", mx: 0.5, fontweight: isActive ? 'bold' : 'normal', borderBottom: isActive ? '3px solid #ffffffff' : '3px solid transparent', borderRadius: 0, '&:hover': { background: 'rgba(255,255,255, 0.1)', borderBottom: '3px solid #21CBF3' } }}>{item.title}</Button>
      )}
    </NavLink>
  ));

  const handleLogout = () => {
    logout(navigate);
  };

  return (
    <MuiAppBar component="header">
      <Toolbar>
        {/*Use flexGrow to push the menu links to the right*/}
        <Box sx={{ display: "flex", width: "100%", height: 40, alignItems: "center" }}>
          {/*Logo and Title*/}
          <Box sx={{ display: "flex", gap: 0.75, flexGrow: 1, height: "100%" }}>
            <a href="https://www.cybertec-postgresql.com/en/">
              <Box sx={{ display: "flex", width: 230, height: 40 }}>
                <img src="./logo.png" />
              </Box>
            </a>
            <Typography sx={{ height: "100%", alignItems: "center", display: "flex" }}>
              PGWATCH
            </Typography>
          </Box>
          {/*Group all interactive items inside the 'token'*/}
          {token &&
            <Box sx={{ display: "flex", height: "100%", alignItems: "center" }}>
              {/*Render navigation links*/}
              {menuLinks}
              {/*AI copilot */}
              <IconButton color="inherit" onClick={onToggleCopilot} title="AI Copilot" sx={{ ml: 1, '&:hover': { color: '#21CBF3' } }}><AutoAwesome /></IconButton>
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
