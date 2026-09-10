import { test, expect, navigate, select } from '../fixtures';

test('Existing demo tags are Chinese in rule view and edit, while saving the same ID', async ({
  page,
  scenario,
}) => {
  const campaign = await scenario.createPlan({ tag: 'auction_demo' });
  await page.goto('/');
  await navigate(page, '广告计划');
  await page
    .getByRole('row')
    .filter({ hasText: scenario.name })
    .getByRole('button', { name: '查看规则', exact: true })
    .click();
  const tag = page.getByRole('combobox', {
    name: '必须全部满足第1条标签',
    exact: true,
  });
  await expect(tag).toBeDisabled();
  await expect(tag).toContainText('竞价人群');
  await expect(tag).not.toContainText('auction_demo');

  await page.getByRole('button', { name: '暂停后编辑', exact: true }).click();
  await expect(tag).toBeEnabled();
  await expect(tag).toContainText('竞价人群');
  await tag.click();
  await page.getByRole('option', { name: '竞价人群', exact: true }).click();
  await page.getByRole('button', { name: '预览并确认', exact: true }).click();
  await page.getByRole('button', { name: '确认发布 v2', exact: true }).click();
  await expect(
    page.getByText('版本 v2 已发布。', { exact: true }),
  ).toBeVisible();
  const saved = await scenario.api.get('/v1/campaigns/' + campaign.id);
  expect((await saved.json()).activeVersion.targeting.all).toEqual([
    { tag: 'auction_demo' },
  ]);
});

test('Chinese demo names and tag labels preserve actual user IDs across workspaces', async ({
  page,
  scenario,
}) => {
  const id = 'demo-auction-user';
  scenario.users.push(id);
  const saved = await scenario.api.put('/v1/profiles/' + id, {
    data: {
      tags: [
        'auction_demo',
        'tech_interest',
        'demo_excluded',
        'custom_audience',
      ],
      fields: { device: 'android', score: '80' },
    },
  });
  expect(saved.ok()).toBeTruthy();
  await scenario.createUser();
  await page.goto('/');
  await navigate(page, '用户画像');
  const row = page.getByRole('row').filter({ hasText: id });
  await expect(row).toContainText('竞价演示用户');
  await expect(row).toContainText(id);
  await expect(row.getByText('竞价人群', { exact: true })).toBeVisible();

  await navigate(page, '单次投放测试');
  const users = page.getByRole('combobox', {
    name: '选择已保存用户',
    exact: true,
  });
  await users.selectOption({ label: '竞价演示用户' });
  await expect(users).toHaveValue(id);
  await expect(
    page.getByRole('textbox', { name: '用户 ID', exact: true }),
  ).toHaveValue(id);
  await expect(
    page.getByText('已保存画像 · 竞价演示用户', { exact: true }),
  ).toBeVisible();
  await expect(page.getByText('竞价人群', { exact: true })).toHaveAttribute(
    'title',
    /auction_demo/,
  );
  await expect(page.getByText('排除演示人群', { exact: true })).toHaveAttribute(
    'title',
    /仅在计划设置/,
  );
  await expect(
    page.getByText('custom_audience', { exact: true }),
  ).toBeVisible();
  await users.selectOption(scenario.userId);
  await expect(users.locator('option:checked')).toHaveText(scenario.userId);
  await expect(
    page.getByRole('textbox', { name: '用户 ID', exact: true }),
  ).toHaveValue(scenario.userId);

  await navigate(page, '批量投放测试');
  await page.getByRole('button', { name: '自定义用户池', exact: true }).click();
  await select(page, '用户池来源', '已保存用户');
  await page.getByText(/查看用户与调整选择/).click();
  await expect(
    page.getByRole('checkbox', { name: /^竞价演示用户 demo-auction-user/ }),
  ).toBeVisible();
  await expect(
    page.getByRole('checkbox', {
      name: new RegExp('^' + scenario.userId + ' '),
    }),
  ).toBeVisible();
});
