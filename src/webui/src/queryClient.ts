import { MutationCache, QueryCache, QueryClient } from "@tanstack/react-query";
import axios from "axios";
import { isUnathorized } from "axiosInstance";
import { removeToken } from "services/Token";

export const queryClient = new QueryClient({
    queryCache: new QueryCache({
        onError: (error: any) => {
            if (axios.isAxiosError(error)) {
                if (isUnathorized(error)) {
                    removeToken();
                }
            }
        }
    }),
    mutationCache: new MutationCache({
        onError: (error: any) => {
            if (axios.isAxiosError(error)) {
                if (isUnathorized(error)) {
                    removeToken();
                }
            }
        }
    })
});
