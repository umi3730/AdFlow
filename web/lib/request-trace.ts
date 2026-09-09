import type { RequestTrace, TraceTarget } from './api';

export const traceReasonLabels: Record<string, string> = {
  matched: '已命中广告',
  targeting_miss: '定向未命中',
  no_creative: '缺少可用素材',
  profile_not_found: '用户画像不存在',
  no_candidate: '没有有效候选',
  frequency_capped: '达到每日频控',
  budget_exhausted: '预算不足或被预占',
  dependency_unavailable: '依赖暂不可用',
};
export const traceStatusLabels: Record<string, string> = {
  PENDING: '等待处理',
  PROCESSING: '处理中',
  SETTLING: '等待结算',
  SETTLED: '已结算',
  RECONCILE: '待核对',
  DEAD_LETTERED: '死信',
  PUBLISHED: '已确认发布',
  PROCESSED: '已计入统计',
  ACCEPTED: '已受理',
  UNCONFIRMED: '结算未确认',
  RUNNING: '执行中',
  LEASE_EXPIRED: '执行租约已过期',
  RELEASED: '执行权已释放',
};
export function traceAdvice(status: string, mode: 'sync' | 'kafka') {
  if (status === 'RECONCILE') return '需要核对结算证明；不要直接重新扣费。';
  if (status === 'DEAD_LETTERED')
    return '排除投递故障后，可由管理员在事件处理页重放原事件。';
  if (status === 'UNCONFIRMED')
    return '可用相同事件 ID 和参数重试；无法证明结算时应先核对。';
  if (status === 'PUBLISHED')
    return '已确认发布；是否计入统计，以计量记录为准。';
  if (
    mode === 'kafka' &&
    ['PENDING', 'PROCESSING', 'SETTLING'].includes(status)
  )
    return '后台会继续处理，稍后刷新即可。';
  return '';
}
export function settlementSummary(trace: RequestTrace) {
  if (trace.settlement)
    return (
      traceStatusLabels[trace.settlement.status] ?? trace.settlement.status
    );
  if (trace.decision && !trace.decision.matched)
    return '本次未返回广告，无需曝光结算';
  if (trace.events.some((event) => event.type === 'impression'))
    return '曝光记录存在，但结算状态未保存';
  if (
    trace.decision?.matched &&
    trace.decision.expiresAt &&
    Date.parse(trace.observedAt) >= Date.parse(trace.decision.expiresAt)
  )
    return '尚无曝光回执，曝光有效期已过';
  return trace.decision?.matched ? '尚未收到曝光回执' : '尚无可展示的结算记录';
}
export function eventCompletion(event: RequestTrace['events'][number]) {
  return event.processedAt ? '已计入统计' : '尚无计量记录';
}
export function simulationTraceTarget(
  requestId: string,
  userId: string,
  runId: string,
  temporary: boolean,
): TraceTarget {
  return temporary && !requestId.startsWith('tmp:')
    ? { requestId, simulationRunId: runId, simulationUserId: userId }
    : { requestId };
}
export function traceTime(value?: string) {
  if (!value) return '—';
  const date = new Date(value);
  return Number.isFinite(date.getTime())
    ? date.toLocaleString('zh-CN', { timeZone: 'Asia/Shanghai', hour12: false })
    : '时间未记录';
}
