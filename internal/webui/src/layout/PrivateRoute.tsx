import { Navigate } from "react-router-dom";
import { getToken } from "services/Token";

type Props = {
  children: JSX.Element
};

export const PrivateRoute = ({ children }: Props) => {
  const token = getToken();

  if (!token) {
    return <Navigate to="/" />;
  }

  return children;
};
