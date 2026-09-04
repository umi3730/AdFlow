# Frontend form submission regression — 2026-09-04

## Symptom and cause

The enabled Create Draft button did nothing when clicked. Forms used React
`onSubmit`, but their Base UI `Button` components omitted `type="submit"`.
Unlike a bare HTML button, the installed Base UI primitive defaults to
`type="button"`. The click therefore never submitted the form or sent a POST.

The same omission affected campaign creation, creative creation, profile saving,
decision execution, and Agent draft generation. Each submit control now declares
its type explicitly; the shared Button primitive and ordinary action buttons are
unchanged.

## Verification

- `npm run test:forms` parses the JSX and requires an explicit submit button in
  each console form. This structural guard runs in frontend CI.
- Frontend lint, formatting, and production build passed.
- A real browser click created the local test campaign
  `表单提交回归验证0904`: the page displayed `广告计划已创建`, showed its DRAFT
  row, and cleared the name input. No API shortcut or WebMCP creation was used.

Earlier navigation checks and successful builds did not test form submission.
Future form verification must include filling inputs and clicking the actual
submit control, not merely navigating to the form or probing an API endpoint.
