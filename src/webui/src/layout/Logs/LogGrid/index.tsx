import { Box } from "@mui/material";

import style from "../style.module.css";

export default () => {
  return (
    <Box className={style.LogGrid}>
      <pre>
        {
          `[15/Dec/2022:10:31:03] ENGINE Listening for SIGTERM.
2022-12-15 10:31:03,216 INFO 84 [15/Dec/2022:10:31:03] ENGINE Listening for SIGTERM.
[15/Dec/2022:10:31:03] ENGINE Listening for SIGHUP.
2022-12-15 10:31:03,216 INFO 84 [15/Dec/2022:10:31:03] ENGINE Listening for SIGHUP.
[15/Dec/2022:10:31:03] ENGINE Listening for SIGUSR1.
2022-12-15 10:31:03,216 INFO 84 [15/Dec/2022:10:31:03] ENGINE Listening for SIGUSR1.
[15/Dec/2022:10:31:03] ENGINE Bus STARTING
2022-12-15 10:31:03,216 INFO 84 [15/Dec/2022:10:31:03] ENGINE Bus STARTING
[15/Dec/2022:10:31:03] ENGINE Started monitor thread 'Autoreloader'.
2022-12-15 10:31:03,218 INFO 84 [15/Dec/2022:10:31:03] ENGINE Started monitor thread 'Autoreloader'.
[15/Dec/2022:10:31:03] ENGINE Serving on http://0.0.0.0:8080
2022-12-15 10:31:03,320 INFO 84 [15/Dec/2022:10:31:03] ENGINE Serving on http://0.0.0.0:8080
[15/Dec/2022:10:31:03] ENGINE Bus STARTED
2022-12-15 10:31:03,321 INFO 84 [15/Dec/2022:10:31:03] ENGINE Bus STARTED
2022-12-15 10:31:22,462 INFO 84 172.17.0.1 - - [15/Dec/2022:10:31:22] "GET /dbs HTTP/1.1" 200 55912 "http://localhost:8080/metrics" "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/108.0.0.0 Safari/537.36"
2022-12-15 10:31:22,513 INFO 84 172.17.0.1 - - [15/Dec/2022:10:31:22] "GET /static/custom.css HTTP/1.1" 200 31 "http://localhost:8080/dbs" "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/108.0.0.0 Safari/537.36"
2022-12-15 10:31:22,517 INFO 84 172.17.0.1 - - [15/Dec/2022:10:31:22] "GET /static/bootstrap-4.3.1-dist/css/bootstrap.min.css HTTP/1.1" 200 155758 "http://localhost:8080/dbs" "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/108.0.0.0 Safari/537.36"
2022-12-15 10:31:22,588 INFO 84 172.17.0.1 - - [15/Dec/2022:10:31:22] "GET /static/bootstrap-4.3.1-dist/js/bootstrap.min.js HTTP/1.1" 200 58072 "http://localhost:8080/dbs" "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/108.0.0.0 Safari/537.36"
2022-12-15 10:31:22,588 INFO 84 172.17.0.1 - - [15/Dec/2022:10:31:22] "GET /static/jquery-3.5.1.min.js HTTP/1.1" 200 89476 "http://localhost:8080/dbs" "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/108.0.0.0 Safari/537.36"
2022-12-15 10:31:22,591 INFO 84 172.17.0.1 - - [15/Dec/2022:10:31:22] "GET /static/logo.png HTTP/1.1" 200 5325 "http://localhost:8080/dbs" "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/108.0.0.0 Safari/537.36"
2022-12-15 10:31:22,739 INFO 84 172.17.0.1 - - [15/Dec/2022:10:31:22] "GET /favicon.ico HTTP/1.1" 200 1406 "http://localhost:8080/dbs" "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/108.0.0.0 Safari/537.36"
2022-12-15 10:31:27,457 INFO 84 172.17.0.1 - - [15/Dec/2022:10:31:27] "GET /logs/pgwatch3/200 HTTP/1.1" 200 - "http://localhost:8080/dbs" "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/108.0.0.0 Safari/537.36"`
        }
      </pre>
    </Box>
  );
};
