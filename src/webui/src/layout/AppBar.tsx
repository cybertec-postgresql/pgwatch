import { Box, Typography } from "@mui/material";
import { AppBar as MuiAppBar } from "@mui/material";
import Button from "@mui/material/Button";
import Toolbar from "@mui/material/Toolbar";
import { Link } from "react-router-dom";


import { routes } from "./Routes";

export const AppBar = () => {
  const menuLinks = routes.map((item) => (
    <Button
      key={item.link}
      to={item.link}
      component={Link}
      sx={{ color: "#fff" }}
    >
      {item.title}
    </Button>
  ));

  return (
    <MuiAppBar component="header">
      <Toolbar>
        <Box sx={{ display: "flex", width: "100%" }}>
          <Box sx={{ display: "flex", flexGrow: 1 }}>
            <a href="https://www.cybertec-postgresql.com/en/">
              <Box sx={{ display: "flex", width: 150, height: 40, marginRight: 2 }}>
                <img src="/logo.png" />
              </Box>
            </a>
            <Typography variant="h6" component="div">
              pgwatch3
            </Typography>
          </Box>
          <Box>
            {menuLinks}
          </Box>
        </Box>
      </Toolbar>
    </MuiAppBar>
  );
};
