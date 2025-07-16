import { useCallback, useEffect, useState } from "react";
import { Box, CircularProgress, Typography } from "@mui/material";
import { useNavigate } from "react-router-dom";
import { usePageStyles } from "styles/page";
import AuthService from "services/Auth";
import { setToken } from "services/Token";
import { useLoginPageStyles } from "./LoginPage.styles";
import { LoginForm } from "./components/LoginForm/LoginForm";

export const LoginPage = () => {
  const { classes: pageClasses } = usePageStyles();
  const { classes: signLoginClasses, cx } = useLoginPageStyles();
  
  const [isCheckingAuth, setIsCheckingAuth] = useState(true);
  const [showLoginForm, setShowLoginForm] = useState(false);
  const navigate = useNavigate();

  const attemptAutoLogin = useCallback(async () => {
    try {
      // Try to login with empty credentials using AuthService directly
      // This bypasses the mutation's onSuccess/onError handlers
      const authService = AuthService.getInstance();
      const data = await authService.login({ user: "", password: "" });
      
      // If successful, set token and navigate manually
      setToken(data);
      navigate("/sources", { replace: true });
    } catch (error: any) {
        setShowLoginForm(true);
    } finally {
      setIsCheckingAuth(false);
    }
  }, [navigate]);

  useEffect(() => {
    attemptAutoLogin();
  }, [attemptAutoLogin]);

  // Show loading spinner while checking authentication requirement
  if (isCheckingAuth) {
    return (
      <div className={cx(pageClasses.root, signLoginClasses.root)}>
        <Box display="flex" flexDirection="column" alignItems="center" gap={2}>
          <CircularProgress size={40} />
          <Typography variant="body1">Checking authentication...</Typography>
        </Box>
      </div>
    );
  }

  // Show login form only if authentication is required
  if (showLoginForm) {
    return (
      <div className={cx(pageClasses.root, signLoginClasses.root)}>
        <LoginForm />
      </div>
    );
  }

  // If we reach here, auto-login was successful and we're redirecting
  return null;
};
