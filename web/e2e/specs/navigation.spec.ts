import { test, expect, navigate, select } from '../fixtures';

test('workspace and campaign filters survive refresh and browser history', async ({
  page,
}) => {
  await page.goto('/');
  await navigate(page, '广告计划');
  await page
    .getByRole('textbox', { name: '搜索计划', exact: true })
    .fill('不存在的计划');
  await select(page, '筛选广告位', '活动弹窗');
  await select(page, '筛选计划状态', '草稿');
  await expect(page).toHaveURL(/view=campaigns/);
  await page.reload();
  await expect(
    page.getByRole('textbox', { name: '搜索计划', exact: true }),
  ).toHaveValue('不存在的计划');
  await expect(
    page.getByRole('combobox', { name: '筛选广告位' }),
  ).toContainText('活动弹窗');
  await expect(
    page.getByRole('combobox', { name: '筛选计划状态' }),
  ).toContainText('草稿');
  await navigate(page, '批量投放测试');
  await page.goBack();
  await expect(
    page.getByRole('heading', { name: '广告计划', exact: true }),
  ).toBeVisible();
  await page.goForward();
  await expect(
    page.getByRole('heading', { name: '批量投放测试', exact: true }),
  ).toBeVisible();
  await page.reload();
  await expect(
    page.getByRole('heading', { name: '批量投放测试', exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole('button', { name: '开始模拟', exact: true }),
  ).toBeDisabled();
  await expect(page.getByRole('combobox', { name: '用户池来源' })).toBeHidden();
  await expect(
    page.getByRole('spinbutton', { name: '单轮超时（毫秒）' }),
  ).toBeHidden();
  await page.getByRole('button', { name: '高级运行参数', exact: true }).click();
  await expect(
    page.getByRole('spinbutton', { name: '单轮超时（毫秒）' }),
  ).toHaveValue('5000');
});

test('only applied report filters enter the URL and history restores the query', async ({
  page,
}) => {
  await page.goto(
    '/?view=reports&reportStart=2026-09-09&reportEnd=2026-09-09&reportGrain=hour',
  );
  await expect(page.getByRole('textbox', { name: '开始日期' })).toHaveValue(
    '2026-09-09',
  );
  const before = page.url();
  await page.getByRole('textbox', { name: '开始日期' }).fill('2026-09-08');
  expect(page.url()).toBe(before);
  await page.getByRole('button', { name: '查询报表', exact: true }).click();
  await expect(page).toHaveURL(/reportStart=2026-09-08/);
  await page.reload();
  await expect(page.getByRole('textbox', { name: '开始日期' })).toHaveValue(
    '2026-09-08',
  );
  await page.goBack();
  await expect(page.getByRole('textbox', { name: '开始日期' })).toHaveValue(
    '2026-09-09',
  );
});
