import type { Decision, Profile } from './api';
import {
  simulationBehaviorForRequest,
  validateSimulationBehavior,
  type SimulationBehavior,
} from './simulation-behavior.ts';

export const simulationLimits = {
  users: 100,
  concurrency: 100,
  seconds: 120,
  rounds: 3000,
};
export interface SimulationConfig {
  mode?: 'concurrency';
  maxRounds?: number;
  runId: string;
  userIds: string[];
  slotId: string;
  concurrency: number;
  seconds: number;
  timeoutMs: number;
  impressions: boolean;
  behavior?: SimulationBehavior;
}
export interface SimulationSnapshot {
  runId: string;
  status: 'running' | 'stopping' | 'completed' | 'stopped';
  stopReason: string;
  elapsedMs: number;
  started: number;
  successful: number;
  failed: number;
  canceled: number;
  inFlight: number;
  peakInFlight: number;
  matched: number;
  noAd: number;
  httpRequests: number;
  impressionsAccepted: number;
  clicksAccepted: number;
  conversionsAccepted: number;
  valueFenAccepted: number;
  timeouts: number;
  reasons: Record<string, number>;
  errors: Record<string, number>;
  p50Ms: number;
  p95Ms: number;
  p99Ms: number;
  recent: {
    acceptedEventIds: string[];
    campaignId?: string;
    advertiserName?: string;
    priceFen?: number;
    requestId: string;
    userId: string;
    outcome: string;
    durationMs: number;
  }[];
}
export interface SimulationTransport {
  click?: (
    decision: Decision,
    eventId: string,
    signal: AbortSignal,
  ) => Promise<unknown>;
  conversion?: (
    decision: Decision,
    eventId: string,
    valueFen: number,
    signal: AbortSignal,
  ) => Promise<unknown>;
  decide: (
    input: { requestId: string; userId: string; slotId: string },
    signal: AbortSignal,
  ) => Promise<Decision>;
  impression: (
    decision: Decision,
    eventId: string,
    signal: AbortSignal,
  ) => Promise<unknown>;
}
export interface SimulationClock {
  now: () => number;
  later: (callback: () => void, delay: number) => unknown;
  cancel: (handle: unknown) => void;
}
const realClock: SimulationClock = {
  now: () => performance.now(),
  later: (callback, delay) => setTimeout(callback, delay),
  cancel: (handle) => clearTimeout(handle as ReturnType<typeof setTimeout>),
};

export function validateSimulationConfig(
  config: SimulationConfig,
): SimulationConfig {
  const integer = (value: number, min: number, max: number, label: string) => {
    if (!Number.isInteger(value) || value < min || value > max)
      throw new Error(label + '需要在 ' + min + '～' + max + ' 之间');
  };
  const mode = config.mode ?? 'concurrency';
  if (mode !== 'concurrency') throw new Error('仅支持固定并发模拟');
  const maxRounds = config.maxRounds ?? simulationLimits.rounds;
  integer(maxRounds, 1, simulationLimits.rounds, '最大轮数');
  integer(config.concurrency, 1, simulationLimits.concurrency, '并发上限');
  integer(config.seconds, 1, simulationLimits.seconds, '运行秒数');
  integer(config.timeoutMs, 100, 10000, '单轮超时毫秒');
  const userIds = [...new Set(config.userIds.map((id) => id.trim()))];
  if (
    !userIds.length ||
    userIds.length > simulationLimits.users ||
    userIds.some((id) => !id || id.length > 128)
  )
    throw new Error('请选择 1～100 个有效用户');
  const slotId = config.slotId.trim();
  if (!slotId || slotId.length > 64) throw new Error('请填写有效广告位');
  if (!/^[\w-]{1,80}$/.test(config.runId)) throw new Error('模拟运行 ID 无效');
  if (config.behavior && !config.impressions)
    throw new Error('模拟点击与转化需要先开启曝光回传');
  const behavior = config.behavior
    ? validateSimulationBehavior(config.behavior)
    : undefined;
  return {
    mode,
    maxRounds,
    runId: config.runId,
    userIds,
    slotId,
    concurrency: config.concurrency,
    seconds: config.seconds,
    timeoutMs: config.timeoutMs,
    impressions: config.impressions,
    behavior,
  };
}

export function percentile(samples: number[], percent: number) {
  if (!samples.length) return 0;
  const sorted = [...samples].sort((a, b) => a - b);
  return sorted[Math.max(0, Math.ceil((sorted.length * percent) / 100) - 1)];
}

export function localSimulationTarget(url: string) {
  try {
    const target = new URL(url);
    return (
      ['http:', 'https:'].includes(target.protocol) &&
      ['localhost', '127.0.0.1', '[::1]'].includes(target.hostname) &&
      !target.username &&
      !target.password
    );
  } catch {
    return false;
  }
}

export function makeSimulationProfiles(
  count: number,
  seed = 'adflow',
): Profile[] {
  if (!Number.isInteger(count) || count < 1 || count > simulationLimits.users)
    throw new Error('临时用户池需要 1～100 人');
  if (!seed.trim() || seed.length > 80)
    throw new Error('样本种子需要 1～80 个字符');
  // Each attribute has its own keyed stream, avoiding index-modulo correlations.
  const sample = (index: number, field: string) => {
    let hash = 2166136261;
    for (const char of seed + ':' + index + ':' + field)
      hash = Math.imul(hash ^ char.charCodeAt(0), 16777619) >>> 0;
    hash ^= hash << 13;
    hash ^= hash >>> 17;
    hash ^= hash << 5;
    return (hash >>> 0) / 4294967296;
  };
  const pick = (i: number, field: string, values: string[]) =>
    values[Math.floor(sample(i, field) * values.length)];
  return Array.from({ length: count }, (_, i) => ({
    userId: 'user-' + String(i + 1).padStart(4, '0'),
    tags: [
      pick(i, 'interest', ['tech_interest', 'gaming_interest']),
      ...(sample(i, 'active') < 0.75 ? ['active_7d'] : []),
      ...(sample(i, 'new') < 0.2 ? ['new_user'] : []),
      ...(sample(i, 'paying') < 0.5 ? ['paying_user'] : []),
    ],
    fields: {
      device: pick(i, 'device', ['android', 'ios', 'android', 'web']),
      age: String(18 + Math.floor(sample(i, 'age') * 53)),
      score: String(40 + Math.floor(sample(i, 'score') * 61)),
      member_level: pick(i, 'member', ['basic', 'silver', 'gold', 'diamond']),
      channel: pick(i, 'channel', ['organic', 'paid', 'referral']),
    },
  }));
}

export function startSimulation(
  input: SimulationConfig,
  transport: SimulationTransport,
  onUpdate: (snapshot: SimulationSnapshot) => void = () => {},
  clock = realClock,
) {
  const config = validateSimulationConfig(input);
  if (config.behavior && (!transport.click || !transport.conversion))
    throw new Error('点击与转化回传未配置');
  const startedAt = clock.now();
  const deadline = startedAt + config.seconds * 1000;
  let issuing = true;
  let stopped = false;
  let finishedAt: number | undefined;
  let schedule: unknown;
  let refresh: unknown;
  let refillPending = false;
  let final = false;
  const controllers = new Set<AbortController>();
  const durations: number[] = [];
  const state: SimulationSnapshot = {
    runId: config.runId,
    status: 'running',
    stopReason: '',
    elapsedMs: 0,
    started: 0,
    successful: 0,
    failed: 0,
    canceled: 0,
    inFlight: 0,
    peakInFlight: 0,
    matched: 0,
    noAd: 0,
    httpRequests: 0,
    impressionsAccepted: 0,
    clicksAccepted: 0,
    conversionsAccepted: 0,
    valueFenAccepted: 0,
    timeouts: 0,
    reasons: {},
    errors: {},
    p50Ms: 0,
    p95Ms: 0,
    p99Ms: 0,
    recent: [],
  };
  let sortedCount = -1;
  let sortedDurations: number[] = [];
  const snapshot = (): SimulationSnapshot => {
    if (sortedCount !== durations.length) {
      sortedDurations = [...durations].sort((a, b) => a - b);
      sortedCount = durations.length;
    }
    const at = (percent: number) =>
      sortedDurations.length
        ? sortedDurations[
            Math.ceil((sortedDurations.length * percent) / 100) - 1
          ]
        : 0;
    return {
      ...state,
      elapsedMs: Math.max(0, (finishedAt ?? clock.now()) - startedAt),
      reasons: { ...state.reasons },
      errors: { ...state.errors },
      recent: state.recent.map((row) => ({
        ...row,
        acceptedEventIds: [...row.acceptedEventIds],
      })),
      p50Ms: at(50),
      p95Ms: at(95),
      p99Ms: at(99),
    };
  };
  let resolveDone!: (value: SimulationSnapshot) => void;
  const done = new Promise<SimulationSnapshot>((resolve) => {
    resolveDone = resolve;
  });
  function emit() {
    onUpdate(snapshot());
  }
  function finishIfDrained() {
    if (issuing || state.inFlight || final) return;
    final = true;
    finishedAt = clock.now();
    state.status = stopped ? 'stopped' : 'completed';
    clock.cancel(schedule);
    clock.cancel(refresh);
    clock.cancel(deadlineTimer);
    emit();
    resolveDone(snapshot());
  }
  function recordError(code: string) {
    state.errors[code] = (state.errors[code] ?? 0) + 1;
  }
  async function round() {
    const index = state.started++;
    const userId = config.userIds[index % config.userIds.length];
    const requestId = config.runId + '-' + String(index + 1).padStart(6, '0');
    const controller = new AbortController();
    controllers.add(controller);
    state.inFlight++;
    state.peakInFlight = Math.max(state.peakInFlight, state.inFlight);
    const began = clock.now();
    let timedOut = false;
    let phase = 'decision';
    let serverRequestId = requestId;
    let outcome = '';
    let campaignId: string | undefined;
    let advertiserName: string | undefined;
    let priceFen: number | undefined;
    const acceptedEventIds: string[] = [];
    const timeout = clock.later(() => {
      timedOut = true;
      controller.abort();
    }, config.timeoutMs);
    try {
      state.httpRequests++;
      const decision = await transport.decide(
        { requestId, userId, slotId: config.slotId },
        controller.signal,
      );
      serverRequestId = decision.requestId || requestId;
      if (controller.signal.aborted) throw new Error('aborted');
      if (decision.matched) {
        state.matched++;
        campaignId = decision.campaignId;
        advertiserName = decision.pricing?.advertiserName;
        priceFen = decision.pricing?.priceFen;
      } else {
        state.noAd++;
        state.reasons[decision.reason || 'unknown'] =
          (state.reasons[decision.reason || 'unknown'] ?? 0) + 1;
      }
      outcome = decision.matched ? '命中' : '未命中：' + decision.reason;
      if (decision.matched && config.impressions) {
        phase = 'impression';
        state.httpRequests++;
        await transport.impression(
          decision,
          requestId + '-impression',
          controller.signal,
        );
        if (controller.signal.aborted) throw new Error('aborted');
        state.impressionsAccepted++;
        acceptedEventIds.push(requestId + '-impression');
        if (config.behavior) {
          const behavior = simulationBehaviorForRequest(
            config.behavior,
            requestId,
          );
          if (behavior.click) {
            phase = 'click';
            state.httpRequests++;
            await transport.click!(
              decision,
              requestId + '-click',
              controller.signal,
            );
            if (controller.signal.aborted) throw new Error('aborted');
            state.clicksAccepted++;
            acceptedEventIds.push(requestId + '-click');
            if (behavior.valueFen !== null) {
              phase = 'conversion';
              state.httpRequests++;
              await transport.conversion!(
                decision,
                requestId + '-conversion',
                behavior.valueFen,
                controller.signal,
              );
              if (controller.signal.aborted) throw new Error('aborted');
              state.conversionsAccepted++;
              acceptedEventIds.push(requestId + '-conversion');
              state.valueFenAccepted += behavior.valueFen;
            }
          }
        }
      }
      state.successful++;
    } catch (cause) {
      if (controller.signal.aborted && !timedOut) {
        state.canceled++;
        outcome = '已取消';
      } else {
        state.failed++;
        const status =
          typeof cause === 'object' && cause !== null && 'status' in cause
            ? String(cause.status)
            : 'network';
        const code = phase + ':' + (timedOut ? 'timeout' : status);
        if (timedOut) state.timeouts++;
        recordError(code);
        outcome = code;
      }
    } finally {
      clock.cancel(timeout);
      controllers.delete(controller);
      state.inFlight--;
      const durationMs = Math.max(0, clock.now() - began);
      if (outcome !== '已取消') durations.push(durationMs);
      state.recent.unshift({
        acceptedEventIds,
        campaignId,
        advertiserName,
        priceFen,
        requestId: serverRequestId,
        userId,
        outcome,
        durationMs,
      });
      state.recent.length = Math.min(20, state.recent.length);
      if (issuing && !refillPending) {
        // Yield to the event loop even for immediate/synchronous failures.
        refillPending = true;
        schedule = clock.later(fillConcurrency, 1);
      }
      finishIfDrained();
    }
  }
  function endConcurrency(reason: string) {
    if (!issuing) return;
    issuing = false;
    state.stopReason = reason;
    state.status = 'stopping';
    clock.cancel(schedule);
    clock.cancel(deadlineTimer);
    finishIfDrained();
  }
  function fillConcurrency() {
    refillPending = false;
    if (!issuing) return;
    if (clock.now() >= deadline) {
      endConcurrency('duration');
      return;
    }
    const remaining = config.maxRounds! - state.started;
    const slots = Math.min(config.concurrency - state.inFlight, remaining);
    for (let i = 0; i < slots && issuing; i++) {
      if (clock.now() >= deadline) break;
      void round();
    }
    if (state.started >= config.maxRounds!) endConcurrency('round_limit');
    else if (clock.now() >= deadline) endConcurrency('duration');
  }
  function tick() {
    if (final) return;
    emit();
    refresh = clock.later(tick, 250);
  }
  function stop(reason = 'manual') {
    if (final || (stopped && state.stopReason !== 'manual_drain')) return;
    stopped = true;
    issuing = false;
    state.status = 'stopping';
    state.stopReason = reason;
    clock.cancel(schedule);
    clock.cancel(deadlineTimer);
    for (const controller of controllers) controller.abort();
    emit();
    finishIfDrained();
  }
  function drain() {
    if (!issuing || final) return;
    stopped = true;
    endConcurrency('manual_drain');
    if (!final) emit();
  }
  emit();
  const deadlineTimer = clock.later(
    () => endConcurrency('duration'),
    config.seconds * 1000,
  );
  fillConcurrency();
  if (!final) refresh = clock.later(tick, 250);
  return { stop, drain, done, snapshot };
}
