export interface DeliveryFilter {
  from: string;
  to: string;
  granularity: 'hour' | 'day';
  campaignId?: string;
}
export interface DeliveryMetrics {
  impressions: number;
  clicks: number;
  conversions: number;
  spendFen: number;
  valueFen: number;
  unpricedImpressions: number;
  ctr: number;
  cvr: number;
  cpcFen: number;
  cpaFen: number;
}
export interface DeliveryReport extends DeliveryFilter {
  timezone: string;
  generatedAt: string;
  summary: DeliveryMetrics;
  series: (DeliveryMetrics & { bucket: string })[];
}
export interface DeliveryDiagnosis {
  summary: string;
  recommendations: { title: string; action: string; evidenceIds: string[] }[];
  evidence: { id: string; title: string; detail: string; suggestion: string }[];
  provider: string;
  model: string;
  promptVersion: string;
  generatedAt: string;
  fallback: boolean;
}
export function beijingDate(now = new Date()): string {
  return new Date(now.getTime() + 8 * 3600000).toISOString().slice(0, 10);
}
export function reportDateRange(
  start: string,
  end: string,
  granularity: 'hour' | 'day',
  campaignId = '',
): DeliveryFilter {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(start) || !/^\d{4}-\d{2}-\d{2}$/.test(end))
    throw new Error('请选择开始与结束日期');
  const from = new Date(`${start}T00:00:00+08:00`);
  const last = new Date(`${end}T00:00:00+08:00`);
  if (
    !Number.isFinite(from.getTime()) ||
    !Number.isFinite(last.getTime()) ||
    beijingDate(from) !== start ||
    beijingDate(last) !== end
  )
    throw new Error('日期无效');
  const to = new Date(last.getTime() + 86400000);
  if (to <= from || to.getTime() - from.getTime() > 31 * 86400000)
    throw new Error('请选择不超过 31 天的有效日期范围');
  return {
    from: from.toISOString(),
    to: to.toISOString(),
    granularity,
    ...(campaignId ? { campaignId } : {}),
  };
}
export function reportQuery(filter: DeliveryFilter): string {
  const query = new URLSearchParams({
    from: filter.from,
    to: filter.to,
    granularity: filter.granularity,
  });
  if (filter.campaignId) query.set('campaignId', filter.campaignId);
  return query.toString();
}
export function reportPreset(
  days: number,
  now = new Date(),
): { start: string; end: string } {
  return {
    start: beijingDate(new Date(now.getTime() - (days - 1) * 86400000)),
    end: beijingDate(now),
  };
}
export function formatReportBucket(
  value: string,
  granularity: 'hour' | 'day',
): string {
  const date = new Date(new Date(value).getTime() + 8 * 3600000).toISOString();
  return granularity === 'hour'
    ? `${date.slice(5, 10)} ${date.slice(11, 16)}`
    : date.slice(0, 10);
}

// Keep real zero buckets, but do not draw future hours as a drop to zero.
export function deliveryChartData(report: DeliveryReport) {
  const observedAt = Date.parse(report.generatedAt);
  return report.series
    .filter((p) => Date.parse(p.bucket) <= observedAt)
    .map((p) => ({
      ...p,
      label: formatReportBucket(p.bucket, report.granularity),
      spend: p.spendFen / 100,
      ctr: p.impressions > 0 ? p.ctr * 100 : null,
      cvr: p.clicks > 0 ? p.cvr * 100 : null,
    }));
}

export function deliveryDetailRows(
  series: DeliveryReport['series'],
  showEmpty = false,
) {
  return series
    .filter(
      (row) =>
        showEmpty ||
        [
          row.impressions,
          row.clicks,
          row.conversions,
          row.spendFen,
          row.valueFen,
          row.unpricedImpressions,
        ].some((value) => value !== 0),
    )
    .sort((a, b) => Date.parse(b.bucket) - Date.parse(a.bucket));
}
