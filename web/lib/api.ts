const API_BASE =
  process.env.NEXT_PUBLIC_ADFLOW_API_URL ?? 'http://127.0.0.1:18080';

export type Status = 'DRAFT' | 'ACTIVE' | 'PAUSED' | 'ENDED';

export interface Condition {
  tag?: string;
  field?: string;
  op?: 'eq' | 'in' | 'gte' | 'lte';
  value?: string;
}

export interface Campaign {
  id: string;
  name: string;
  slotId: string;
  startAt: string;
  endAt: string;
  status: Status;
  revision: number;
  activeVersion?: {
    number: number;
    targeting: { all?: Condition[]; any?: Condition[]; none?: Condition[] };
    dailyBudgetFen: number;
    impressionCostFen: number;
    frequencyLimit: number;
    publishedAt: string;
  };
}

export interface Creative {
  id: string;
  campaignId: string;
  title: string;
  description: string;
  imageUrl: string;
  landingUrl: string;
  status: 'ACTIVE' | 'DISABLED';
  revision: number;
}

export interface Metrics {
  campaignId: string;
  impressions: number;
  clicks: number;
  conversions: number;
  valueFen: number;
}

export interface Decision {
  requestId: string;
  matched: boolean;
  campaignId: string;
  creativeId: string;
  reservationToken: string;
  expiresAt?: string;
  reason: string;
}

export interface RuleDraft {
  targeting: { all?: Condition[]; any?: Condition[]; none?: Condition[] };
  dailyBudgetFen: number;
  impressionCostFen: number;
  frequencyLimit: number;
  explanation: string;
  warnings: string[];
  provider: string;
  model: string;
  promptVersion: string;
  generatedAt: string;
}

export interface OutboxStats {
  pending: number;
  processing: number;
  published: number;
  deadLettered: number;
}

export interface OutboxRecord {
  event: {
    eventId: string;
    requestId: string;
    campaignId: string;
    creativeId: string;
    type: 'impression' | 'click' | 'conversion';
  };
  status: 'PENDING' | 'PROCESSING' | 'PUBLISHED' | 'DEAD_LETTERED';
  attempts: number;
  nextAttemptAt: string;
  lastError?: string;
  createdAt: string;
}

export interface KafkaPartitionLag {
  topic: string;
  partition: number;
  lag: number;
}

type ApiErrorBody = { error?: { message?: string; code?: string } };

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const headers = new Headers(init?.headers);
  headers.set('Content-Type', 'application/json');
  const response = await fetch(`${API_BASE}${path}`, {
    ...init,
    headers,
  });
  if (!response.ok) {
    let body: ApiErrorBody = {};
    try {
      body = (await response.json()) as ApiErrorBody;
    } catch {}
    throw new Error(body.error?.message ?? `请求失败 (${response.status})`);
  }
  if (response.status === 204) return undefined as T;
  return response.json() as Promise<T>;
}

export const api = {
  listCampaigns: () =>
    request<{ items: Campaign[] }>('/v1/campaigns?limit=100'),
  createCampaign: (input: {
    name: string;
    slotId: string;
    startAt: string;
    endAt: string;
  }) =>
    request<Campaign>('/v1/campaigns', {
      method: 'POST',
      body: JSON.stringify(input),
    }),
  publishCampaign: (
    id: string,
    input: {
      targeting: { all?: Condition[]; any?: Condition[]; none?: Condition[] };
      dailyBudgetFen: number;
      impressionCostFen: number;
      frequencyLimit: number;
    },
  ) =>
    request<Campaign>(`/v1/campaigns/${id}/publish`, {
      method: 'POST',
      body: JSON.stringify(input),
    }),
  pauseCampaign: (id: string) =>
    request<Campaign>(`/v1/campaigns/${id}/pause`, { method: 'POST' }),
  resumeCampaign: (campaignID: string) =>
    request<Campaign>(`/v1/campaigns/${campaignID}/resume`, { method: 'POST' }),
  updateCampaign: (
    campaignID: string,
    input: { name: string; slotId: string; startAt: string; endAt: string },
  ) =>
    request<Campaign>(`/v1/campaigns/${campaignID}`, {
      method: 'PUT',
      body: JSON.stringify(input),
    }),
  listCreatives: (campaignID: string) =>
    request<{ items: Creative[] }>(`/v1/campaigns/${campaignID}/creatives`),
  createCreative: (
    campaignID: string,
    input: {
      title: string;
      description: string;
      imageUrl: string;
      landingUrl: string;
    },
  ) =>
    request<Creative>(`/v1/campaigns/${campaignID}/creatives`, {
      method: 'POST',
      body: JSON.stringify(input),
    }),
  disableCreative: (campaignID: string, creativeID: string) =>
    request<Creative>(
      `/v1/campaigns/${campaignID}/creatives/${creativeID}/disable`,
      { method: 'POST' },
    ),
  putProfile: (
    userID: string,
    input: { tags: string[]; fields: Record<string, string> },
  ) =>
    request<void>(`/v1/profiles/${userID}`, {
      method: 'PUT',
      body: JSON.stringify(input),
    }),
  decide: (input: { requestId: string; userId: string; slotId: string }) =>
    request<Decision>('/v1/decisions', {
      method: 'POST',
      body: JSON.stringify(input),
    }),
  recordEvent: (input: {
    eventId: string;
    requestId: string;
    campaignId: string;
    creativeId: string;
    type: 'impression' | 'click' | 'conversion';
    valueFen?: number;
  }) =>
    request<{ eventId: string; recorded: boolean }>('/v1/events', {
      method: 'POST',
      body: JSON.stringify(input),
    }),
  metrics: (campaignID: string) =>
    request<Metrics>(`/v1/campaigns/${campaignID}/metrics`),
  generateRuleDraft: (prompt: string) =>
    request<RuleDraft>('/v1/agent/rule-drafts', {
      method: 'POST',
      body: JSON.stringify({ prompt }),
    }),
  operationsOutbox: (status = '') =>
    request<{ items: OutboxRecord[]; stats: OutboxStats }>(
      `/v1/operations/outbox?limit=50${status ? `&status=${status}` : ''}`,
    ),
  operationsKafkaLag: () =>
    request<{ items: KafkaPartitionLag[] }>('/v1/operations/kafka-lag'),
  replayDeadLetter: (eventID: string) =>
    request<{ eventId: string; status: string }>(
      `/v1/operations/dead-letters/${eventID}/replay`,
      { method: 'POST' },
    ),
};

export function newClientID(prefix: string) {
  return `${prefix}-${Date.now()}-${Math.random().toString(16).slice(2, 8)}`;
}
