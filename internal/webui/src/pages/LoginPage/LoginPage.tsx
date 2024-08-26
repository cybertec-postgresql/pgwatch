import { usePageStyles } from "styles/page";
import { useLoginPageStyles } from "./LoginPage.styles";
import { LoginForm } from "./components/LoginForm/LoginForm";

export const LoginPage = () => {
  const { classes: pageClasses } = usePageStyles();
  const { classes: signLoginClasses, cx } = useLoginPageStyles();

  return (
    <div className={cx(pageClasses.root, signLoginClasses.root)}>
      <LoginForm />
    </div>
  );
};
