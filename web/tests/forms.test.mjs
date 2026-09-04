import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import ts from 'typescript';

test('all console forms explicitly declare a submit button', () => {
  const source = ts.createSourceFile(
    'adflow-console.tsx',
    readFileSync(
      new URL('../components/adflow-console.tsx', import.meta.url),
      'utf8',
    ),
    ts.ScriptTarget.Latest,
    true,
    ts.ScriptKind.TSX,
  );
  const forms = [];
  function visit(node) {
    if (
      ts.isJsxElement(node) &&
      node.openingElement.tagName.getText(source) === 'form'
    ) {
      forms.push(node);
    }
    ts.forEachChild(node, visit);
  }
  visit(source);
  assert.ok(forms.length >= 5, 'expected the five existing console forms');
  for (const form of forms) {
    let submitButtons = 0;
    function findSubmit(node) {
      const element = ts.isJsxElement(node) ? node.openingElement : node;
      if (
        ts.isJsxOpeningElement(element) ||
        ts.isJsxSelfClosingElement(element)
      ) {
        if (['Button', 'button'].includes(element.tagName.getText(source))) {
          const type = element.attributes.properties.find(
            (prop) =>
              ts.isJsxAttribute(prop) && prop.name.getText(source) === 'type',
          );
          if (
            type?.initializer &&
            ts.isStringLiteral(type.initializer) &&
            type.initializer.text === 'submit'
          ) {
            submitButtons += 1;
          }
        }
      }
      // Visit children, not the opening element again, to avoid double counting.
      if (ts.isJsxElement(node)) node.children.forEach(findSubmit);
      else ts.forEachChild(node, findSubmit);
    }
    form.children.forEach(findSubmit);
    const line = source.getLineAndCharacterOfPosition(form.pos).line + 1;
    assert.equal(
      submitButtons,
      1,
      `form near line ${line} needs one explicit submit button (Base UI defaults to type=button)`,
    );
  }
});
