import * as Yup from "yup";

export const loginFormValuesValidationSchema = Yup.object({
  user: Yup.string().required("User is required"),
  password: Yup.string().required("Password is required"),
});
