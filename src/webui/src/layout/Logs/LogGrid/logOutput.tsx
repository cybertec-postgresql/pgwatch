interface LogEventColor {
  [key: string]: string;
}

const logEventColor: LogEventColor = {
  "[INFO]": "green",
  "[WARNING]": "orange",
  "[DEBUG]": "purple",
  "[ERROR]": "red"
};

export const logOutput = (log: MessageEvent<any>, index: number) => {
  const splittedLog = String(log.data).split(" ");

  return (
    <p>
      {
        splittedLog.map((logElement, elementIndex) => {
          if (logEventColor[logElement]) {
            return (<span key={index} style={{ color: logEventColor[logElement] }}> {logElement}</span>);
          } else {
            if (elementIndex === 0) {
              return logElement;
            }
            return " " + logElement;
          }
        })
      }
    </p>
  );
};
