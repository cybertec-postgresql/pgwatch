import React from 'react';
import RestartAltIcon from '@mui/icons-material/RestartAlt';
import { Divider, ListItemIcon, ListItemText, MenuItem } from '@mui/material';
import {
  GridColumnMenu,
  GridColumnMenuColumnsItem,
  GridColumnMenuFilterItem,
  GridColumnMenuHideItem,
  GridColumnMenuProps,
  GridColumnMenuSortItem,
} from '@mui/x-data-grid';

interface CustomColumnMenuProps extends GridColumnMenuProps {
  onResetAllColumnWidths?: () => void;
}

export const CustomColumnMenu: React.FC<CustomColumnMenuProps> = (props) => {
  const { hideMenu, onResetAllColumnWidths, colDef, ...other } = props;

  const handleResetAllWidths = (event: React.MouseEvent) => {
    event.preventDefault();
    onResetAllColumnWidths?.();
    hideMenu?.(event);
  };

  return (
    <GridColumnMenu
      hideMenu={hideMenu}
      colDef={colDef}
      {...other}
    >
      <GridColumnMenuSortItem onClick={hideMenu} colDef={colDef} />
      <GridColumnMenuFilterItem onClick={hideMenu} colDef={colDef} />
      <Divider />
      <GridColumnMenuHideItem onClick={hideMenu} colDef={colDef} />
      <GridColumnMenuColumnsItem onClick={hideMenu} colDef={colDef} />
      <Divider />
      <MenuItem onClick={handleResetAllWidths}>
        <ListItemIcon>
          <RestartAltIcon fontSize="small" />
        </ListItemIcon>
        <ListItemText primary="Reset All Column Widths" />
      </MenuItem>
    </GridColumnMenu>
  );
};
