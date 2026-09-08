import {
  sessionToken,
  sessionRevision,
  expireSessionIfCurrent,
  type AuthInfo,
  type IssuedToken,
} from './auth-session.ts';
import { collectNumberingPages } from './available-number.ts';

const API_BASE =
  process.env.NEXT_PUBLIC_ADFLOW_API_URL ?? 'http://127.0.0.1:18080';
export const simulationAPIBase = API_BASE;

export interface TraceTarget {
  requestId: string;
  simulationRunId?: string;
  simulationUserId?: string;
  clientOutcome?: string;
}
export interface RequestTrace {
  requestId: string;
  eventTransport: 'sync' | 'kafka';
  observedAt: string;
  decision?: {
    userId: string;
    slotId: string;
    matched: boolean;
    campaignId?: string;
    creativeId?: string;
    reason: string;
    pricing?: Decision['pricing'];
    expiresAt?: string;
  };
  execution?: { status: string; leaseUntil?: string };
  settlement?: {
    status: string;
    failedAttempts: number;
    acceptedAt: string;
    settledAt?: string;
    nextAttemptAt?: string;
    lockedUntil?: string;
    lastError?: string;
  };
  events: {
    eventId: string;
    type: 'impression' | 'click' | 'conversion';
    valueFen?: number;
    occurredAt: string;
    acceptedAt: string;
    status: string;
    failedAttempts: number;
    lastError?: string;
    nextAttemptAt?: string;
    publishedAt?: string;
    processedAt?: string;
  }[];
  truncated: boolean;
}

export type Status = 'DRAFT' | 'ACTIVE' | 'PAUSED' | 'ENDED';

export interface Condition {
  tag?: string;
  field?: string;
  op?: 'eq' | 'in' | 'gte' | 'lte';
  value?: string;
}

export interface Campaign {
  auctionSupported?: boolean;
  id: string;
  name: string;
  slotId: string;
  startAt: string;
  endAt: string;
  status: Status;
  revision: number;
  activeVersion?: {
    auction?: AuctionTerms;
    number: number;
    targeting: { all?: Condition[]; any?: Condition[]; none?: Condition[] };
    dailyBudgetFen: number;
    impressionCostFen: number;
    frequencyLimit: number;
    publishedAt: string;
  };
}

export interface AuctionTerms {
  advertiserId: string;
  advertiserName: string;
  bidFen: number;
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
  pricing?: {
    mode: 'fixed' | 'first_price';
    advertiserId?: string;
    advertiserName?: string;
    bidFen?: number;
    priceFen: number;
    version: number;
    advertisers: number;
    rank: number;
    budgetRejected: number;
    frequencyRejected: number;
  } | null;
  requestId: string;
  matched: boolean;
  campaignId: string;
  creativeId: string;
  reservationToken: string;
  expiresAt?: string | null;
  reason: string;
}

export interface Profile {
  userId: string;
  tags: string[];
  fields: Record<string, string>;
}

export interface ProfilePage {
  items: Profile[];
  total: number;
  limit: number;
  offset: number;
}

export interface TargetingExplanation {
  profile: Profile;
  checkedAt: string;
  scope: 'current_targeting_only';
  candidates: {
    campaignId: string;
    targetingMatched: boolean;
    hasCreative: boolean;
    failures: {
      group: 'all' | 'any' | 'none';
      code:
        | 'invalid_condition'
        | 'missing_tag'
        | 'missing_field'
        | 'value_mismatch'
        | 'excluded_condition';
      condition: Condition;
      actual?: string;
      present: boolean;
    }[];
  }[];
}

export interface RuleDraft {
  fallback: boolean;
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
  settling?: number;
  reconcile?: number;
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
  status:
    | 'PENDING'
    | 'PROCESSING'
    | 'PUBLISHED'
    | 'DEAD_LETTERED'
    | 'SETTLING'
    | 'RECONCILE';
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

export class ApiError extends Error {
  status: number;
  code?: string;
  constructor(message: string, status: number, code?: string) {
    super(message);
    this.status = status;
    this.code = code;
  }
}

async function request<T>(
  path: string,
  init?: RequestInit,
  authenticated = true,
): Promise<T> {
  const headers = new Headers(init?.headers);
  headers.set('Content-Type', 'application/json');
  const token = authenticated ? sessionToken() : '';
  const revision = sessionRevision();
  if (token) headers.set('Authorization', 'Bearer ' + token);
  const response = await fetch(`${API_BASE}${path}`, {
    ...init,
    headers,
  });
  if (!response.ok) {
    if (response.status === 401 && authenticated)
      expireSessionIfCurrent(revision);
    let body: ApiErrorBody = {};
    try {
      body = (await response.json()) as ApiErrorBody;
    } catch {}
    throw new ApiError(
      response.status === 401
        ? '登录已失效，请重新登录'
        : response.status === 403
          ? '当前账号没有此操作权限'
          : (body.error?.message ?? `请求失败 (${response.status})`),
      response.status,
      body.error?.code,
    );
  }
  if (response.status === 204) return undefined as T;
  return response.json() as Promise<T>;
}

export const api = {
  authOptions: () =>
    request<{
      registrationEnabled: boolean;
      registrationRole: 'admin';
      demoLoginPrefill: boolean;
    }>('/v1/auth/options', {}, false),
  register: (username: string, password: string) =>
    request<IssuedToken>(
      '/v1/auth/register',
      { method: 'POST', body: JSON.stringify({ username, password }) },
      false,
    ),
  authMe: () => request<AuthInfo>('/v1/auth/me'),
  login: (username: string, password: string) =>
    request<IssuedToken>(
      '/v1/auth/login',
      {
        method: 'POST',
        body: JSON.stringify({ username, password }),
      },
      false,
    ),
  operationsMode: () =>
    request<{ eventTransport: 'sync' | 'kafka'; outboxEnabled: boolean }>(
      '/v1/operations/mode',
    ),
  requestTrace: (target: TraceTarget, signal?: AbortSignal) => {
    const query = new URLSearchParams({ requestId: target.requestId });
    if (target.simulationRunId)
      query.set('simulationRunId', target.simulationRunId);
    if (target.simulationUserId)
      query.set('simulationUserId', target.simulationUserId);
    return request<RequestTrace>(`/v1/operations/request-trace?${query}`, {
      signal,
    });
  },
  listProfiles: (
    filter: {
      q?: string;
      tag?: string;
      device?: string;
      limit?: number;
      offset?: number;
    } = {},
    signal?: AbortSignal,
  ) => {
    const params = new URLSearchParams();
    for (const [key, value] of Object.entries(filter))
      if (value !== undefined && value !== '') params.set(key, String(value));
    return request<ProfilePage>(`/v1/profiles?${params}`, { signal });
  },
  getProfile: (userID: string, signal?: AbortSignal) =>
    request<Profile>(`/v1/profiles/${encodeURIComponent(userID)}`, { signal }),
  explainDecision: (input: { userId: string; slotId: string }) =>
    request<TargetingExplanation>('/v1/decisions/explain', {
      method: 'POST',
      body: JSON.stringify(input),
    }),
  listProfileCatalog: () =>
    collectNumberingPages<Profile>((offset, limit) =>
      request<ProfilePage>(`/v1/profiles?limit=${limit}&offset=${offset}`),
    ),
  listCampaigns: async () => ({
    items: await collectNumberingPages<Campaign>((offset, limit) =>
      request<{ items: Campaign[] }>(
        `/v1/campaigns?limit=${limit}&offset=${offset}`,
      ),
    ),
  }),
  getCampaign: (id: string) =>
    request<Campaign>('/v1/campaigns/' + encodeURIComponent(id)),
  deleteCampaign: (id: string) =>
    request<void>('/v1/campaigns/' + encodeURIComponent(id), {
      method: 'DELETE',
    }),
  deleteCreative: (campaignID: string, creativeID: string) =>
    request<void>(
      '/v1/campaigns/' +
        encodeURIComponent(campaignID) +
        '/creatives/' +
        encodeURIComponent(creativeID),
      { method: 'DELETE' },
    ),
  deleteProfile: (userID: string) =>
    request<void>('/v1/profiles/' + encodeURIComponent(userID), {
      method: 'DELETE',
    }),
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
      auction?: AuctionTerms;
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
    signal?: AbortSignal,
  ) =>
    request<void>(`/v1/profiles/${encodeURIComponent(userID)}`, {
      method: 'PUT',
      body: JSON.stringify(input),
      signal,
    }),
  enableCreative: (campaignID: string, creativeID: string) =>
    request<Creative>(
      `/v1/campaigns/${encodeURIComponent(campaignID)}/creatives/${encodeURIComponent(creativeID)}/enable`,
      { method: 'POST' },
    ),
  decide: (
    input: { requestId: string; userId: string; slotId: string },
    signal?: AbortSignal,
  ) =>
    request<Decision>('/v1/decisions', {
      method: 'POST',
      body: JSON.stringify(input),
      signal,
    }),
  simulateDecision: (
    input: {
      runId: string;
      requestId: string;
      slotId: string;
      profile: Profile;
    },
    signal?: AbortSignal,
  ) =>
    request<Decision>('/v1/simulations/decisions', {
      method: 'POST',
      body: JSON.stringify(input),
      signal,
    }),
  recordEvent: (
    input: {
      eventId: string;
      requestId: string;
      campaignId: string;
      creativeId: string;
      type: 'impression' | 'click' | 'conversion';
      valueFen?: number;
    },
    signal?: AbortSignal,
  ) =>
    request<{ eventId: string; recorded: boolean }>('/v1/events', {
      method: 'POST',
      body: JSON.stringify(input),
      signal,
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
