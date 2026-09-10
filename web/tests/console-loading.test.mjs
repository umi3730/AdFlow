import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync, existsSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import ts from 'typescript';

const root = fileURLToPath(new URL('../', import.meta.url));

// Follow static local imports only: dynamic workspace imports must not
// become eager dependencies of the dashboard or the Agent's filter form.
function eagerModules(entry, seen = new Set()) {
  const file = resolve(root, entry);
  if (seen.has(file)) return seen;
  seen.add(file);
  const source = ts.createSourceFile(
    file,
    readFileSync(file, 'utf8'),
    ts.ScriptTarget.Latest,
    true,
    ts.ScriptKind.TSX,
  );
  for (const item of source.statements) {
    if (
      !ts.isImportDeclaration(item) ||
      item.importClause?.phaseModifier === ts.SyntaxKind.TypeKeyword
    )
      continue;
    const name = item.moduleSpecifier.text;
    const base = name.startsWith('@/')
      ? resolve(root, name.slice(2))
      : name.startsWith('.')
        ? resolve(dirname(file), name)
        : null;
    if (!base) continue;
    const dependency = [base, base + '.ts', base + '.tsx'].find((candidate) =>
      existsSync(candidate),
    );
    if (dependency) eagerModules(dependency, seen);
  }
  return seen;
}

test('Dashboard does not eagerly import other workspaces or the report chart', () => {
  const loaded = eagerModules('components/adflow-console.tsx');
  for (const file of [
    'console/campaigns-view',
    'console/creatives-view',
    'console/decision-view',
    'console/operations-view',
    'user-pool-simulation',
    'profile-workspace',
    'delivery-report',
    'delivery-diagnosis',
  ]) {
    assert.equal(
      loaded.has(resolve(root, 'components/' + file + '.tsx')),
      false,
      file,
    );
  }
  assert.equal(
    loaded.has(resolve(root, 'components/console/dashboard.tsx')),
    true,
  );
});

test('Agent reuses report filters without importing report charts', () => {
  const loaded = eagerModules('components/delivery-diagnosis.tsx');
  assert.equal(
    loaded.has(resolve(root, 'components/report-filters.tsx')),
    true,
  );
  assert.equal(
    loaded.has(resolve(root, 'components/delivery-report.tsx')),
    false,
  );
});
