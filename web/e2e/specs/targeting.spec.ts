import { test, expect, openDecision, runDecision } from '../fixtures';

test('A nonmatching plan is collapsed separately from the saved winner', async ({
  page,
  scenario,
}) => {
  scenario.slotId = 'game-home-banner';
  await scenario.createUser();
  const winner = await scenario.createPlan();
  const otherName = '未匹配计划-' + scenario.name;
  await scenario.createPlan({ name: otherName, tag: 'unmatched-e2e-tag' });
  await page.goto('/');
  await openDecision(page, scenario);
  const decision = await runDecision(page);
  expect(decision.campaignId).toBe(winner.id);

  const selected = page.getByRole('region', {
    name: '本次成交计划的定向检查',
    exact: true,
  });
  await expect(selected.getByText('本次成交', { exact: true })).toBeVisible();
  await expect(selected).toContainText(scenario.name);
  await expect(
    selected.getByText('定向条件满足', { exact: true }),
  ).toBeVisible();
  await expect(page.getByText(otherName, { exact: true })).toBeHidden();

  await page.getByText('查看其他计划（1）', { exact: true }).click();
  await expect(page.getByText(otherName, { exact: true })).toBeVisible();
  await expect(page.getByText('定向条件不满足', { exact: true })).toBeVisible();
  await expect(page.getByText(/以下计划均未在本次成交。/)).toBeVisible();
  await expect(
    selected.getByText('定向条件不满足', { exact: true }),
  ).toHaveCount(0);
});
