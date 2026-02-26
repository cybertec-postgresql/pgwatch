import { QueryClientProvider as ClientProvider, MutationCache, QueryCache, QueryClient } from "@tanstack/react-query";
import { isUnauthorized } from "api";
import axios from "axios";
import { useEffect, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";
import { logout } from "queries/Auth";
import { useAlert } from "utils/AlertContext";

type Props = {
  children: JSX.Element
};

export const QueryClientProvider = ({ children }: Props) => {
  const { callAlert } = useAlert();
  const navigate = useNavigate();

  const callAlertRef = useRef(callAlert);

  useEffect(() => {
    callAlertRef.current = callAlert;
  }, [callAlert]);

  const [queryClient] = useState(() => {
    const client = new QueryClient({
      queryCache: new QueryCache({
        onError: (error) => {
          if (axios.isAxiosError(error)) {
            if (isUnauthorized(error)) {
              callAlertRef.current("error", `${error.response?.data}`);
              logout(navigate);
            }
          }
        }
      }),
      mutationCache: new MutationCache({
        onError: (error) => {
          if (axios.isAxiosError(error)) {
            callAlertRef.current("error", `${error.response?.data}`);
            if (isUnauthorized(error)) {
              logout(navigate);
            }
          }
        },
        onSuccess: (_data, _variables, _context, mutation) => {
          if (mutation.options.mutationKey) {
            client.invalidateQueries({ queryKey: mutation.options.mutationKey });
          }
        },
      })
    })
    return client;
  });


  return (
    <ClientProvider client={queryClient}>
      {children}
    </ClientProvider>
  );
};
