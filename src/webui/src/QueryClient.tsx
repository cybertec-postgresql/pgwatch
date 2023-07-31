import { MutationCache, QueryCache, QueryClient, QueryClientProvider as ClientProvider } from "@tanstack/react-query";
import axios from "axios";
import { isUnauthorized } from "axiosInstance";
import { useNavigate } from "react-router-dom";
import { removeToken } from "services/Token";
import { useAlert } from "utils/AlertContext";

type Props = {
  children: JSX.Element
};

export const QueryClientProvider = ({ children }: Props) => {
  const { callAlert } = useAlert();
  const navigate = useNavigate();

  const queryClient = new QueryClient({
    queryCache: new QueryCache({
      onError: (error) => {
        if (axios.isAxiosError(error)) {
          if (isUnauthorized(error)) {
            removeToken();
            callAlert("error", `${error.response?.data}`);
            navigate("/");
          }
        }
      }
    }),
    mutationCache: new MutationCache({
      onSuccess: (_data, _variables, _context, mutation) => {
        callAlert("success", "Success");
        if (mutation.options.mutationKey) {
          queryClient.invalidateQueries(mutation.options.mutationKey);
        }
      },
      onError: (error) => {
        if (axios.isAxiosError(error)) {
          callAlert("error", `${error.response?.data}`);
          if (isUnauthorized(error)) {
            removeToken();
            navigate("/");
          }
        }
      }
    })
  });

  return (
    <ClientProvider client={queryClient} contextSharing={true}>
      {children}
    </ClientProvider>
  );
};
