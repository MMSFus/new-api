# Moments Auth Login and Registration Guidance Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make Moments Auth the clearly recommended New API entry point and prevent users from completing a password-registration form when password registration is disabled.

**Architecture:** Add a focused, presentational `MomentsAuthEntry` component for context-specific sign-in/sign-up copy while reusing the existing OIDC handler. Teach the generic OAuth provider list to omit providers promoted elsewhere, and make the sign-up form render one of four configuration-driven states without changing the backend OAuth or registration contracts.

**Tech Stack:** React 19, TypeScript, React Hook Form, TanStack Router, react-i18next, Tailwind CSS, Vitest, React Testing Library.

**Spec:** Conversation-approved New API-only design: Moments Auth is the primary login/registration entry; password login remains a clearly labeled on-site account option; disabled password registration never renders an actionable password form; Casdoor and backend OIDC behavior remain unchanged.

## Global Constraints

- Do not modify Casdoor, OIDC endpoints, OAuth state/callback behavior, or backend registration enforcement.
- Read the provider display name from `oidc_display_name`, with `OIDC` as a fallback; production will later set it to `Moments Auth`.
- All user-facing copy must use `useTranslation()` and literal `t()` calls.
- Reuse existing `Button`, `Badge`, `Skeleton`, form controls, `OAuthProviders`, and `useOAuthLogin`.
- Keep password registration available when `password_register_enabled=true`.
- Avoid rendering the OIDC provider twice after it receives a dedicated primary entry.
- Add behavior-focused component tests and run affected tests, typecheck, lint, format check, and production build before completion.

---

### Task 1: Add the dedicated Moments Auth entry

**Files:**
- Create: `web/src/features/auth/components/moments-auth-entry.tsx`
- Create: `web/src/features/auth/components/__tests__/moments-auth-entry.test.tsx`

**Interfaces:**
- Produces: `MomentsAuthEntry(props)` with `variant: 'sign-in' | 'sign-up'`, `displayName?: string`, `onContinue: () => void`, `disabled?: boolean`, and `loading?: boolean`.
- Behavior: renders a recommended identity card, variant-specific explanatory copy, and an accessible primary button.

- [ ] **Step 1: Write failing component tests**

Cover display-name fallback, sign-in wording containing “sign in or register”, sign-up wording explaining automatic account creation, and disabled/loading button behavior.

- [ ] **Step 2: Run the test and confirm failure**

Run: `cd web && bunx vitest run src/features/auth/components/__tests__/moments-auth-entry.test.tsx`

Expected: FAIL because `moments-auth-entry.tsx` does not exist.

- [ ] **Step 3: Implement the component**

Compose existing `Badge` and `Button` with a `ShieldCheck` icon. Keep the component presentational so OAuth state and navigation remain owned by `useOAuthLogin`.

- [ ] **Step 4: Run the component test**

Run: `cd web && bunx vitest run src/features/auth/components/__tests__/moments-auth-entry.test.tsx`

Expected: PASS.

### Task 2: Promote Moments Auth on sign-in without removing password compatibility

**Files:**
- Modify: `web/src/features/auth/sign-in/components/user-auth-form.tsx`
- Modify: `web/src/features/auth/components/oauth-providers.tsx`
- Create: `web/src/features/auth/sign-in/__tests__/login-method-priority.test.tsx`

**Interfaces:**
- `OAuthProviders` gains `excludedProviders?: string[]`; excluded provider keys are not rendered.
- `UserAuthForm` invokes the existing `handleOIDCLogin` from `useOAuthLogin` and renders `MomentsAuthEntry` whenever OIDC is enabled.

- [ ] **Step 1: Write failing sign-in tests**

Mock status and OAuth handlers. Assert that OIDC produces one recommended “sign in or register” button, does not appear again in the generic provider list, and that password login is labeled as an on-site account option when enabled.

- [ ] **Step 2: Run the test and confirm failure**

Run: `cd web && bunx vitest run src/features/auth/sign-in/__tests__/login-method-priority.test.tsx`

Expected: FAIL because the current page treats OIDC as a generic provider and has no on-site account heading.

- [ ] **Step 3: Implement sign-in hierarchy**

Render Moments Auth first, retain Passkey and non-OIDC OAuth methods as secondary choices, and add the translated “On-site account sign-in” heading plus compatibility help text around the existing password form.

- [ ] **Step 4: Run sign-in tests**

Run: `cd web && bunx vitest run src/features/auth/sign-in/__tests__/login-method-priority.test.tsx src/features/auth/components/__tests__/moments-auth-entry.test.tsx`

Expected: PASS.

### Task 3: Replace disabled password registration with configuration-aware guidance

**Files:**
- Modify: `web/src/features/auth/sign-up/components/sign-up-form.tsx`
- Create: `web/src/features/auth/sign-up/__tests__/registration-modes.test.tsx`

**Interfaces:**
- Registration modes are selected from `register_enabled`, `password_register_enabled`, `oidc_enabled`, and status loading state.
- The existing password form and submission behavior remain unchanged when password registration is enabled.

- [ ] **Step 1: Write failing registration-mode tests**

Assert:
- disabled password registration plus OIDC shows only the Moments Auth entry;
- username, password, verification-code, and password-submit controls are absent in that mode;
- enabled password registration preserves the form;
- globally disabled registration shows a closed message;
- no password and no OIDC shows an unavailable message;
- a first load with no status renders a skeleton rather than flashing the password form.

- [ ] **Step 2: Run the test and confirm failure**

Run: `cd web && bunx vitest run src/features/auth/sign-up/__tests__/registration-modes.test.tsx`

Expected: FAIL because the current form always renders password registration controls.

- [ ] **Step 3: Implement registration states**

Use `useStatus().loading`, show an auth-form skeleton only when status is absent and loading, and return explanatory cards for closed/unavailable states. In OIDC-only mode render `MomentsAuthEntry` and no password or email-verification controls. In password-enabled mode retain the existing form while promoting Moments Auth and excluding OIDC from the generic list.

- [ ] **Step 4: Run registration tests**

Run: `cd web && bunx vitest run src/features/auth/sign-up/__tests__/registration-modes.test.tsx src/features/auth/components/__tests__/moments-auth-entry.test.tsx`

Expected: PASS.

### Task 4: Verify the complete frontend change

**Files:**
- Verify all files changed in Tasks 1–3.

- [ ] **Step 1: Run all affected auth tests**

Run: `cd web && bunx vitest run src/features/auth/components/__tests__/moments-auth-entry.test.tsx src/features/auth/sign-in/__tests__/login-method-priority.test.tsx src/features/auth/sign-up/__tests__/registration-modes.test.tsx src/features/auth/api.test.ts src/features/auth/lib/__tests__/oauth-callback-mode.test.ts`

Expected: PASS.

- [ ] **Step 2: Run TypeScript typecheck**

Run: `cd web && bun run typecheck`

Expected: exit 0.

- [ ] **Step 3: Run lint on the frontend**

Run: `cd web && bun run lint`

Expected: exit 0 with no lint errors in changed files.

- [ ] **Step 4: Run format check**

Run: `cd web && bun run format:check`

Expected: exit 0. If changed files are unformatted, run `bun run format`, then repeat the check.

- [ ] **Step 5: Run production build**

Run: `cd web && bun run build`

Expected: exit 0 and Rsbuild emits the production bundle.

- [ ] **Step 6: Inspect the final diff and repository status**

Run: `git diff --check && git status --short && git diff --stat`

Expected: no whitespace errors; only the plan, auth components, and tests are modified.
