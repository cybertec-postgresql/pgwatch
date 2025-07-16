import * as Yup from "yup";

export const loginFormValuesValidationSchema = Yup.object({
  user: Yup.string().optional(),
  password: Yup.string().optional(),
});
