import {
  test as base,
  expect,
  type APIRequestContext,
  type Page,
} from '@playwright/test';
import { randomUUID } from 'node:crypto';
import { apiOrigin, appOrigin } from './runtime.mjs';
import { adSlotLabel } from '../lib/ad-slots';

export class Scenario {
  readonly name = '自动回归-' + randomUUID().slice(0, 8);
  readonly userId = 'e2e-' + randomUUID();
  slotId = 'activity-popup';
  readonly campaigns: string[] = [];
  readonly users: string[] = [];

  constructor(readonly api: APIRequestContext) {}

  async createUser() {
    this.users.push(this.userId);
    const response = await this.api.put('/v1/profiles/' + this.userId, {
      data: {
        tags: ['tech_interest'],
        fields: { device: 'android', score: '88', age: '28' },
      },
    });
    expect(response.ok(), await response.text()).toBeTruthy();
  }

  async createPlan(options: { name?: string; tag?: string } = {}) {
    const now = Date.now();
    const response = await this.api.post('/v1/campaigns', {
      data: {
        name: options.name ?? this.name,
        slotId: this.slotId,
        startAt: new Date(now - 3600000).toISOString(),
        endAt: new Date(now + 86400000).toISOString(),
      },
    });
    expect(response.ok(), await response.text()).toBeTruthy();
    const campaign = (await response.json()) as { id: string };
    this.campaigns.push(campaign.id);
    const creative = await this.api.post(
      '/v1/campaigns/' + campaign.id + '/creatives',
      {
        data: {
          title: this.name,
          description: '自动化回归',
          imageUrl: appOrigin + '/demo-assets/bangdream/tomori.webp',
          landingUrl: appOrigin,
        },
      },
    );
    expect(creative.ok(), await creative.text()).toBeTruthy();
    const published = await this.api.post(
      '/v1/campaigns/' + campaign.id + '/publish',
      {
        data: {
          targeting: { all: [{ tag: options.tag ?? 'tech_interest' }] },
          dailyBudgetFen: 10000,
          impressionCostFen: 5,
          frequencyLimit: 100,
        },
      },
    );
    expect(published.ok(), await published.text()).toBeTruthy();
    return campaign;
  }

  async summary(campaignId: string) {
    const now = Date.now();
    const query = new URLSearchParams({
      campaignId,
      from: new Date(now - 3600000).toISOString(),
      to: new Date(now + 3600000).toISOString(),
      granularity: 'hour',
    });
    const response = await this.api.get('/v1/reports/delivery?' + query);
    expect(response.ok(), await response.text()).toBeTruthy();
    return (await response.json()).summary as {
      impressions: number;
      clicks: number;
      conversions: number;
      spendFen: number;
      valueFen: number;
    };
  }

  async cleanup() {
    for (const id of this.campaigns) {
      const response = await this.api.get('/v1/campaigns/' + id);
      if (response.status() === 404) continue;
      const current = (await response.json()) as { status: string };
      if (current.status === 'ACTIVE')
        await this.api.post('/v1/campaigns/' + id + '/pause');
      expect((await this.api.delete('/v1/campaigns/' + id)).ok()).toBeTruthy();
    }
    for (const id of this.users) {
      expect((await this.api.delete('/v1/profiles/' + id)).ok()).toBeTruthy();
    }
  }
}

export const test = base.extend<{ scenario: Scenario }>({
  scenario: async ({ playwright }, runTest) => {
    const api = await playwright.request.newContext({ baseURL: apiOrigin });
    const ready = await api.get('/readyz');
    expect(await ready.json()).toEqual({ ready: true, dependencies: {} });
    const scenario = new Scenario(api);
    try {
      await runTest(scenario);
    } finally {
      await scenario.cleanup();
      await api.dispose();
    }
  },
  page: async ({ page }, runTest) => {
    const errors: string[] = [];
    const forbidden: string[] = [];
    page.on('pageerror', (error) => errors.push(error.message));
    await page.route('**/*', async (route) => {
      const url = new URL(route.request().url());
      if (url.origin === appOrigin) return route.continue();
      forbidden.push(url.origin + url.pathname);
      await route.abort('blockedbyclient');
    });
    await runTest(page);
    expect(
      forbidden,
      'Browser requests must remain inside the isolated app',
    ).toEqual([]);
    expect(errors, 'Unexpected browser runtime errors').toEqual([]);
  },
});

export { expect };

export async function navigate(page: Page, name: string, heading = name) {
  await page
    .getByRole('navigation')
    .getByRole('button', { name, exact: true })
    .click();
  await expect(
    page.getByRole('heading', { name: heading, exact: true }),
  ).toBeVisible();
}

export async function select(page: Page, label: string, option: string) {
  await page.getByRole('combobox', { name: label, exact: true }).click();
  await page.getByRole('option', { name: option, exact: true }).click();
}

export async function openDecision(page: Page, scenario: Scenario) {
  await navigate(page, '单次投放测试');
  await page
    .getByRole('combobox', { name: '选择已保存用户', exact: true })
    .selectOption(scenario.userId);
  await select(page, '广告位', adSlotLabel(scenario.slotId));
}

export async function runDecision(page: Page) {
  const response = page.waitForResponse(
    (response) =>
      new URL(response.url()).pathname === '/v1/decisions' &&
      response.request().method() === 'POST',
  );
  await page.getByRole('button', { name: '运行决策', exact: true }).click();
  const decision = (await (await response).json()) as {
    requestId: string;
    matched: boolean;
    campaignId: string;
    expiresAt?: string;
    reason: string;
  };
  await expect(
    page.getByText(decision.requestId, { exact: true }),
  ).toBeVisible();
  return decision;
}

export async function openBatch(page: Page, scenario: Scenario) {
  await navigate(page, '批量投放测试');
  await page.getByRole('button', { name: '自定义用户池', exact: true }).click();
  await select(page, '用户池来源', '已保存用户');
  await page.getByText(/查看用户与调整选择/).click();
  await page
    .getByRole('checkbox', { name: new RegExp('^' + scenario.userId + ' ') })
    .check();
  await select(page, '广告位', adSlotLabel(scenario.slotId));
  await page
    .getByRole('spinbutton', { name: '最大轮数', exact: true })
    .fill('3');
  await page
    .getByRole('spinbutton', { name: '目标并发轮次', exact: true })
    .fill('1');
}
