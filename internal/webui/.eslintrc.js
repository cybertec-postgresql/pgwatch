module.exports = {
  env: {
    browser: true,
    es6: true,
    node: true,
  },
  extends: [
    "eslint:recommended",
    "plugin:@typescript-eslint/eslint-recommended",
    "plugin:import/recommended",
    "plugin:react/recommended",
    "plugin:@typescript-eslint/recommended",
    "plugin:prettier/recommended",
    "prettier",
    "plugin:react-hooks/recommended",
    "plugin:react/jsx-runtime",
  ],
  globals: {
    Atomics: "readonly",
    SharedArrayBuffer: "readonly",
  },
  parser: "@typescript-eslint/parser",
  parserOptions: {
    ecmaFeatures: {
      jsx: "react",
    },
    project: ["./tsconfig.json"],
    ecmaVersion: 2018,
    sourceType: "module",
  },
  plugins: ["react", "@typescript-eslint", "prettier", "react-hooks"],
  settings: {
    react: {
      version: require("./package.json").dependencies.react,
    },
  },
  rules: {
    "prettier/prettier": [
      0,
      {
        semi: true,
        trailingComma: "all",
        printWidth: 100,
        tabWidth: 2,
        jsxSingleQuote: false,
        jsxBracketSameLine: true,
      },
    ],
    semi: 1,
    eqeqeq: [1, "always"],
    quotes: "off",
    "jsx-quotes": ["warn", "prefer-double"],
    "no-undef": 0,
    "no-console": "warn",
    "no-unused-vars": 0,
    "no-mixed-operators": "off",
    "eol-last": ["warn", "always"],
    "no-confusing-arrow": 0,
    complexity: "off",
    "constructor-super": "warn",
    curly: "warn",
    "new-parens": "off",
    "newline-per-chained-call": "off",
    "no-bitwise": "warn",
    "no-caller": "warn",
    "no-cond-assign": "warn",
    "no-debugger": "warn",
    "no-empty-function": "off",
    "no-empty": ["warn", { allowEmptyCatch: true }],
    "no-eval": "warn",
    "no-extra-semi": "off",
    "no-shadow": "off",
    "@typescript-eslint/no-shadow": [
      "warn",
      {
        ignoreFunctionTypeParameterNameValueShadow: true,
        ignoreTypeValueShadow: true,
      },
    ],
    "no-fallthrough": "off",
    "no-invalid-this": "off",
    "no-irregular-whitespace": "off",
    "no-multiple-empty-lines": "off",
    "no-new-wrappers": "warn",
    "no-throw-literal": "warn",
    "no-trailing-spaces": "off",
    "no-undef-init": "warn",
    "no-unsafe-finally": "warn",
    "no-unused-labels": "warn",
    "object-shorthand": "warn",
    "prefer-arrow-callback": "warn",
    "prefer-const": "warn",
    "quote-props": "off",
    radix: "warn",
    "space-before-function-paren": "off",
    "use-isnan": "warn",
    "valid-typeof": "off",
    "sort-imports": ["warn", { ignoreDeclarationSort: true }],
    "no-param-reassign": 0,
    "prefer-template": 0,
    "prefer-promise-reject-errors": 0,
    "no-script-url": 0,
    "no-unused-expressions": 0,
    "dot-notation": 0,
    "import/prefer-default-export": 0,
    "import/no-useless-path-segments": 1,
    "import/no-unresolved": 0,
    "import/no-extraneous-dependencies": 0,
    "import/no-named-as-default": 0,
    "import/no-duplicates": 0,
    "import/order": [
      "warn",
      {
        groups: [
          "builtin",
          "external",
          "internal",
          "parent",
          "sibling",
          "index",
        ],
        pathGroups: [
          {
            pattern: "react",
            group: "external",
            position: "before",
          },
          {
            pattern: "layout/**",
            group: "internal",
          },
          {
            pattern: "queries/**",
            group: "internal",
          },
          {
            pattern: "services/**",
            group: "internal",
          },
          {
            pattern: "utils/**",
            group: "internal",
          },
        ],
        pathGroupsExcludedImportTypes: ["internal"],
        alphabetize: {
          order: "asc",
        },
      },
    ],
    "import/newline-after-import": 1,
    "import/no-named-as-default-member": 0,
    "import/namespace": 0,
    "import/named": 0,
    "jsx-a11y/anchor-is-valid": 0,
    "jsx-a11y/click-events-have-key-events": 0,
    "jsx-a11y/no-static-element-interactions": 0,
    "jsx-a11y/label-has-for": 0,
    "jsx-a11y/iframe-has-title": 0,
    "jsx-a11y/control-has-associated-label": 0,
    "react/jsx-wrap-multilines": [
      "warn",
      {
        declaration: "parens-new-line",
        assignment: "parens-new-line",
        return: "parens-new-line",
        arrow: "ignore",
        condition: "ignore",
        logical: "ignore",
        prop: "ignore",
      },
    ],
    "react/jsx-filename-extension": 0,
    "react/jsx-indent": 0,
    "react/jsx-boolean-value": 0,
    "react/jsx-closing-tag-location": 0,
    "react/jsx-closing-bracket-location": [1, "tag-aligned"],
    "react/button-has-type": 0,
    "react/prop-types": [2, { "ignore": ["sx"] }],
    "react/button-has-type": 0,
    "react/jsx-tag-spacing": ["warn", { beforeSelfClosing: "always" }],
    "react/jsx-one-expression-per-line": 0,
    "react/jsx-curly-spacing": 0,
    "react/no-access-state-in-setstate": 0,
    "react/no-unknown-property": ["error", { ignore: ["sx"] }],
    "react/destructuring-assignment": 0,
    "react/jsx-no-bind": 0,
    "react/react-in-jsx-scope": 0,
    "react/require-default-props": 0,
    "react/display-name": 0,
    "react/jsx-first-prop-new-line": 0,
    "react/jsx-props-no-spreading": 0,
    "react/static-property-placement": 0,
    "react/state-in-constructor": 0,
    "@typescript-eslint/indent": "off",
    "@typescript-eslint/interface-name-prefix": "off",
    "@typescript-eslint/member-delimiter-style": "off",
    "@typescript-eslint/member-ordering": "off",
    "@typescript-eslint/consistent-type-assertions": "warn",
    "@typescript-eslint/no-empty-interface": "warn",
    "@typescript-eslint/no-explicit-any": "off",
    "@typescript-eslint/no-misused-new": "warn",
    "@typescript-eslint/no-namespace": "warn",
    "@typescript-eslint/no-parameter-properties": "off",
    "@typescript-eslint/no-use-before-declare": "off",
    "@typescript-eslint/no-var-requires": "warn",
    "@typescript-eslint/prefer-for-of": "warn",
    "@typescript-eslint/prefer-function-type": "warn",
    "@typescript-eslint/prefer-interface": "off",
    "@typescript-eslint/prefer-namespace-keyword": "warn",
    "@typescript-eslint/quotes": "off",
    "@typescript-eslint/type-annotation-spacing": "off",
    "@typescript-eslint/unified-signatures": "warn",
    "@typescript-eslint/camelcase": 0,
    "@typescript-eslint/explicit-module-boundary-types": "off",
    "@typescript-eslint/no-non-null-assertion": 0,
    "@typescript-eslint/no-use-before-define": 0,
    "@typescript-eslint/no-unused-vars": [
      "warn",
      {
        argsIgnorePattern: "^_",
        varsIgnorePattern: "^_",
        ignoreRestSiblings: true,
      },
    ],
    "@typescript-eslint/no-use-before-define": 0,
    "@typescript-eslint/explicit-member-accessibility": 0,
    // how to be more granular? we only want to allow `{}` for now
    "@typescript-eslint/ban-types": "off",
  },
};
