import { QueryClientProvider as ClientProvider, MutationCache, QueryCache, QueryClient } from "@tanstack/react-query";
import { isUnauthorized } from "api";
import axios from "axios";
import { useNavigate } from "react-router-dom";
import { logout } from "queries/Auth";
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
            callAlert("error", `${error.response?.data}`);
            logout(navigate);
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
            logout(navigate);
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
