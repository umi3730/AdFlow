import { readFile } from 'node:fs/promises';
import { appOrigin } from '../runtime.mjs';
import {
  test,
  expect,
  navigate,
  select,
  openDecision,
  runDecision,
  openBatch,
} from '../fixtures';

test('create, publish, retain selections, deliver events and export a report', async ({
  page,
  scenario,
}, testInfo) => {
  await page.goto('/');
  await page.getByRole('button', { name: '新建广告计划', exact: true }).click();
  const createDialog = page.getByRole('dialog', {
    name: '新建广告计划',
    exact: true,
  });
  await createDialog
    .getByRole('textbox', { name: '计划名称', exact: true })
    .fill(scenario.name);
  await select(page, '广告位', '活动弹窗');
  const createdResponse = page.waitForResponse(
    (response) =>
      new URL(response.url()).pathname === '/v1/campaigns' &&
      response.request().method() === 'POST',
  );
  await createDialog
    .getByRole('button', { name: '创建草稿', exact: true })
    .click();
  const campaign = (await (await createdResponse).json()) as { id: string };
  scenario.campaigns.push(campaign.id);
  await expect(createDialog).toBeHidden();
  const rules = page.getByRole('dialog', { name: '编辑投放规则', exact: true });
  await rules
    .getByRole('textbox', { name: '日预算（元）', exact: true })
    .fill('10');
  await rules
    .getByRole('textbox', { name: '单次曝光成本（元）', exact: true })
    .fill('0.05');
  await rules
    .getByRole('spinbutton', { name: '每人每日最多曝光次数', exact: true })
    .fill('10');
  await rules.getByRole('button', { name: '预览并确认', exact: true }).click();
  await expect(
    rules.getByText('发布规则后还需添加素材，计划才会参与投放。'),
  ).toBeVisible();
  await rules.getByRole('button', { name: '确认发布 v1', exact: true }).click();
  const published = page.getByRole('dialog', {
    name: '查看投放规则',
    exact: true,
  });
  await expect(
    published.getByText('还缺一份素材，添加后即可试投。'),
  ).toBeVisible();
  await published
    .getByRole('button', { name: '添加素材', exact: true })
    .click();
  await expect(
    page.getByRole('heading', { name: '素材管理', exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole('combobox', { name: '选择广告计划', exact: true }),
  ).toContainText(scenario.name);
  await page
    .getByRole('textbox', { name: '素材标题', exact: true })
    .fill('回归素材');
  await page
    .getByRole('button', { name: '选择高松灯素材', exact: true })
    .click();

  await navigate(page, '用户画像');
  scenario.users.push(scenario.userId);
  await page
    .getByRole('textbox', { name: '用户 ID', exact: true })
    .fill(scenario.userId);
  await page.getByRole('button', { name: '保存画像', exact: true }).click();
  const user = page.getByRole('row').filter({ hasText: scenario.userId });
  await user.getByRole('button', { name: '发起决策', exact: true }).click();
  await expect(
    page.getByRole('textbox', { name: '用户 ID', exact: true }),
  ).toHaveValue(scenario.userId);
  await select(page, '广告位', '活动弹窗');
  const missing = await runDecision(page);
  expect(missing.reason).toBe('no_creative');
  await expect(
    page.getByText('定向已满足，但计划缺少可用素材', { exact: true }),
  ).toBeVisible();

  await navigate(page, '素材管理');
  await expect(
    page.getByRole('textbox', { name: '素材标题', exact: true }),
  ).toHaveValue('回归素材');
  await expect(
    page.getByRole('button', { name: '左侧已选素材接收区', exact: true }),
  ).toContainText('高松灯');
  await page.getByRole('button', { name: '确认添加素材', exact: true }).click();
  await expect(
    page.getByRole('button', { name: '确认添加素材', exact: true }),
  ).toBeDisabled();
  await expect(
    page.getByRole('img', { name: '回归素材', exact: true }),
  ).toBeVisible();
  await navigate(page, '单次投放测试');
  await expect(
    page.getByText(missing.requestId, { exact: true }),
  ).toBeVisible();
  await navigate(page, '素材管理');
  await page.getByRole('button', { name: '测试此计划', exact: true }).click();
  await expect(page.getByText('填写参数并运行一次决策')).toBeVisible();
  await expect(
    page.getByRole('textbox', { name: '用户 ID', exact: true }),
  ).toHaveValue(scenario.userId);
  await expect(
    page.getByRole('combobox', { name: '广告位', exact: true }),
  ).toContainText('活动弹窗');
  await expect
    .poll(async () => (await runDecision(page)).matched, {
      message:
        'The candidate cache should observe the new creative within its TTL',
      timeout: 12_000,
      intervals: [250, 500, 1000],
    })
    .toBe(true);
  await expect(
    page.getByRole('button', { name: '记录点击', exact: true }),
  ).toBeDisabled();
  await page.getByRole('button', { name: '记录曝光', exact: true }).click();
  await page.getByRole('button', { name: '记录点击', exact: true }).click();
  await page.getByRole('button', { name: '记录转化 ¥5', exact: true }).click();
  await expect(
    page.getByRole('button', { name: '转化已受理 ¥5', exact: true }),
  ).toBeDisabled();
  await expect
    .poll(() => scenario.summary(campaign.id))
    .toMatchObject({
      impressions: 1,
      clicks: 1,
      conversions: 1,
      spendFen: 5,
      valueFen: 500,
    });

  await page.getByRole('button', { name: '查看处理过程', exact: true }).click();
  await expect(
    page.getByRole('dialog').getByText('已计入统计', { exact: true }),
  ).toHaveCount(3);
  await page
    .getByRole('dialog')
    .getByRole('button', { name: '关闭', exact: true })
    .click();
  await expect(page.getByRole('dialog')).toBeHidden();
  await page
    .getByRole('button', { name: '查看此计划报表', exact: true })
    .click();
  await expect(
    page.getByRole('combobox', { name: '报表广告计划', exact: true }),
  ).toContainText(scenario.name);
  await page.getByRole('button', { name: '查看明细', exact: true }).click();
  await expect(page.getByRole('table')).toContainText('¥0.05');
  const downloadEvent = page.waitForEvent('download');
  await page.getByRole('button', { name: '导出 CSV', exact: true }).click();
  const download = await downloadEvent;
  expect(download.suggestedFilename()).toMatch(/\.csv$/);
  const output = testInfo.outputPath('delivery.csv');
  await download.saveAs(output);
  const csv = await readFile(output, 'utf8');
  expect(csv).toContain('CTR（%）');
  expect(csv).toContain('0.0500,5.0000,100.0000,100.0000');

  await page
    .getByRole('textbox', { name: '开始日期', exact: true })
    .fill('2000-01-01');
  await expect(page.getByText('点击「查询报表」查看所选范围。')).toBeVisible();
  await expect(
    page.getByRole('button', { name: '导出 CSV', exact: true }),
  ).toBeDisabled();
  await page.getByRole('button', { name: '查询报表', exact: true }).click();
  await expect(page.getByRole('alert')).toContainText('31');
});

test('unexposed decisions expire while their workspace is hidden', async ({
  page,
  scenario,
}) => {
  scenario.slotId = 'sidebar-recommendation';
  await scenario.createUser();
  const campaign = await scenario.createPlan();
  await page.clock.install();
  await page.goto('/');
  await openDecision(page, scenario);
  const decision = await runDecision(page);
  expect(decision.matched).toBe(true);
  await expect(
    page.getByRole('button', { name: '记录曝光', exact: true }),
  ).toBeEnabled();
  await navigate(page, '总览', '投放总览');
  await page.clock.fastForward(31_000);
  await navigate(page, '单次投放测试');
  await expect(
    page.getByText(decision.requestId, { exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole('button', { name: '已过期，请重新决策', exact: true }),
  ).toBeDisabled();
  expect((await scenario.summary(campaign.id)).impressions).toBe(0);
});

test('bounded batch completes and its result survives navigation', async ({
  page,
  scenario,
}) => {
  scenario.slotId = 'content-detail-bottom';
  await scenario.createUser();
  const campaign = await scenario.createPlan();
  await page.goto('/');
  await openBatch(page, scenario);
  await page.getByRole('button', { name: '开始模拟', exact: true }).click();
  await expect(page.getByText('已发 3 / 3 轮', { exact: true })).toBeVisible();
  await expect(
    page.getByRole('status').filter({ hasText: /^请求已结束$/ }),
  ).toBeVisible();
  await navigate(page, '总览', '投放总览');
  await navigate(page, '批量投放测试');
  await expect(page.getByText('已发 3 / 3 轮', { exact: true })).toBeVisible();
  await expect(page.getByText('0 / 0', { exact: true })).toHaveCount(2);
  await expect(
    page.getByText('回传已接收，最终是否计入统计尚未核对。'),
  ).toBeVisible();
  await page.getByRole('button', { name: '核对统计', exact: true }).click();
  await expect(page.getByRole('region', { name: '统计确认' })).toContainText(
    '已计入统计 3/3 条',
  );
  await expect(page.getByRole('region', { name: '统计确认' })).toContainText(
    '尚未确认 0 条 · 需处理 0 条 · 查询失败 0 条',
  );
  await expect
    .poll(() => scenario.summary(campaign.id))
    .toMatchObject({ impressions: 3, spendFen: 15 });
  await page.getByRole('button', { name: '再次运行', exact: true }).click();
  await expect(
    page.getByRole('status').filter({ hasText: /^请求已结束$/ }),
  ).toBeVisible();
  await expect(
    page.getByText('回传已接收，最终是否计入统计尚未核对。'),
  ).toBeVisible();
  await expect(
    page.getByRole('region', { name: '统计确认' }),
  ).not.toContainText('已计入统计 3/3 条');
});

test('global stop cancels a pending round after switching workspaces', async ({
  page,
  scenario,
}) => {
  scenario.slotId = 'feed-recommendation';
  await scenario.createUser();
  await scenario.createPlan();
  await page.goto('/');
  await openBatch(page, scenario);
  let release!: () => void;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  let observed!: () => void;
  const pending = new Promise<void>((resolve) => {
    observed = resolve;
  });
  let decisions = 0;
  let stopped = false;
  // Hold a real API response, making the in-flight boundary deterministic.
  await page.route('**/v1/decisions', async (route) => {
    expect(new URL(route.request().url()).origin).toBe(appOrigin);
    decisions++;
    const response = await route.fetch();
    observed();
    await gate;
    try {
      await route.fulfill({ response });
    } catch (error) {
      if (!stopped) throw error;
    }
  });
  try {
    await page.getByRole('button', { name: '开始模拟', exact: true }).click();
    await pending;
    await navigate(page, '总览', '投放总览');
    await expect(
      page.getByText('批量投放测试运行中', { exact: true }),
    ).toBeVisible();
    await page.getByRole('button', { name: '停止模拟', exact: true }).click();
    stopped = true;
    await navigate(page, '批量投放测试');
    await expect(
      page.getByRole('status').filter({ hasText: /^已停止$/ }),
    ).toBeVisible();
    await expect(page.getByText('1 / 0', { exact: true })).toBeVisible();
    expect(decisions).toBe(1);
  } finally {
    release();
  }
});
