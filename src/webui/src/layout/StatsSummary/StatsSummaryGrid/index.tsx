import { Box, Grid, Typography } from "@mui/material";
import { ErrorComponent } from "layout/common/ErrorComponent";
import { LoadingComponent } from "layout/common/LoadingComponent";
import { useStatsSummary } from "queries/StatsSummary";


export const StatsSummaryGrid = () => {
  const { status, data, error } = useStatsSummary();

  if (status === "loading") {
    return (
      <LoadingComponent />
    );
  }

  if (status === "error") {
    return (
      <ErrorComponent errorMessage={String(error)} />
    );
  }

  const prettifyString = (value: string) => {
    const newValue = value.replace(/[A-Z]+|\d/g, (element) => ` ${element.toLowerCase()}`);
    return newValue.charAt(0).toUpperCase() + newValue.slice(1);
  };

  return (
    <Box
      sx={{
        display: "flex",
        flexDirection: "column",
        gap: 1,
        height: "100%",
        alignItems: "center"
      }}
    >
      <Box
        sx={{
          height: 80,
          width: 1000,
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          padding: 1,
          gap: 2,
          borderBottom: 2,
        }}
      >
        {
          Object.entries(data.main).map(([key, value], index) => (
            <Box
              key={index}
              sx={{
                borderLeft: 5,
                borderLeftColor: "#1976d2",
                height: 50,
                width: 150,
                paddingLeft: 2
              }}
            >
              <Typography
                sx={{
                  fontSize: 14,
                  color: "#6c6b6b"
                }}
              >
                {prettifyString(key)}
              </Typography>
              <Typography
                sx={{
                  fontWeight: "bold"
                }}
                variant="h6"
              >
                {value}
              </Typography>
            </Box>
          ))
        }
      </Box>
      {
        Object.entries(data).map(([key, dataSet], dataSetIndex) => (
          key !== "main" && (
            <Box
              key={dataSetIndex}
              sx={{
                width: 1000,
                height: "100%",
                padding: 2,
                display: "flex",
                alignItems: "center",
                flexFlow: "column",
                gap: 1
              }}
            >
              <Typography variant="h5" fontWeight="bold" sx={{width: "100%"}}>{prettifyString(key)}</Typography>
              <Grid
                container
                sx={{
                  height: "100%",
                  width: "100%",
                  display: "flex",
                  paddingRight: 3,
                  paddingLeft: 3,
                  margin: 0,
                  justifyContent: "space-between"
                }}
                rowGap={1}
              >
                {
                  Object.entries(dataSet).map(([dataSetKey, dataSetValue], index) => (
                    <Grid
                      key={index}
                      item
                      xs={5.7}
                      sx={{
                        height: 50,
                        maxHeight: 50,
                        display: "flex",
                        borderBottom: 1,
                        borderColor: "lightgray"
                      }}
                    >
                      <Box sx={{ display: "flex", height: "100%", width: "75%", alignItems: "center" }}>{prettifyString(dataSetKey)}</Box>
                      <Box sx={{ display: "flex", height: "100%", width: "25%", alignItems: "center", justifyContent: "right" }}>{dataSetValue}</Box>
                    </Grid>
                  ))
                }
              </Grid>
            </Box>
          )
        ))
      }
    </Box>
  );
};
