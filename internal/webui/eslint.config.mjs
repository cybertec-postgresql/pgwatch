import js from "@eslint/js";
import tseslint from "typescript-eslint";
import reactPlugin from "eslint-plugin-react";
import reactHooksPlugin from "eslint-plugin-react-hooks";
import prettierConfig from "eslint-config-prettier";

export default [
    js.configs.recommended,
    ...tseslint.configs.recommended,
    {
        files: ["**/*.{ts,tsx,jsx}"],
        plugins: {
            react: reactPlugin,
            "react-hooks": reactHooksPlugin,
        },
        languageOptions: {
            parserOptions: {
                ecmaFeatures: {
                    jsx: true,
                },
            },
        },
        settings: {
            react: {
                version: "detect",
            },
        },
        rules: {
            ...reactPlugin.configs.recommended.rules,
            ...reactHooksPlugin.configs.recommended.rules,
            "react/react-in-jsx-scope": "off",
            "react-hooks/set-state-in-effect": "off",
            "react-hooks/immutability": "off",
        },
    },
    prettierConfig,
    {
        ignores: [
            "dist/**",
            "node_modules/**",
            "build/**",
            "internal/serviceWorker.js",
            "internal/setupTests.ts",
            "internal/**/__tests__/**",
            "internal/**/test/**",
        ],
    },
];
