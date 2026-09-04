# Campaign rule editor

The campaign workspace no longer publishes a hard-coded example when a user
clicks an action. Click the plan name or its rule button to open the editor.

- **DRAFT:** edit targeting, daily budget, impression cost, and frequency limit;
  preview the payload, then explicitly confirm publication as v1.
- **ACTIVE:** inspect the current version read-only. The explicit
  "暂停后编辑" action pauses the campaign before allowing edits.
- **PAUSED:** load the current rules, edit, preview, and publish the next version.
  The existing Resume button resumes the unchanged version instead.
- Unpublished edits are local to the open dialog, not saved server-side.
  Closing the dialog discards them.
- Existing conditions are copied without silent changes. Legacy `platform`
  conditions display a warning and can explicitly be converted to `device`, the
  field saved by the profile editor.
- Money is entered in yuan and converted to safe integer fen. Empty conditions,
  more than 50 conditions, invalid operators/numeric comparisons, non-positive
  amounts, cost above budget, and frequency outside 1–100 are rejected.

## Verification — 2026-09-04

Using the actual browser controls on the local application:

1. Created an isolated plan named `规则编辑验证0904` on `rule-editor-test`.
2. Edited budget to 19.99 yuan, impression cost to 0.01 yuan, frequency to 2,
   and added `device=android` plus an installed-player exclusion.
3. Preview left the API record in DRAFT with no active version.
4. Explicit confirmation published v1 and switched the dialog to read-only.
5. Paused through the dialog; a zero-budget preview was rejected.
6. Verified the explicit `platform` → `device` conversion, changed budget to
   25 yuan and frequency to 4, then confirmed v2.
7. Left the test plan paused; existing user plans were not changed.

Desktop and 390px viewport checks passed. The mobile dialog measured 343px
wide and scrolls internally. The operations column stays at the right edge of
the horizontally scrollable plan table. Rule compilation tests and the explicit
submit-button guard run in frontend CI.
