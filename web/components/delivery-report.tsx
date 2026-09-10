'use client';

import { useEffect, useId, useMemo, useRef, useState } from 'react';
import {
  ChevronDown,
  Download,
  LoaderCircle,
  RefreshCw,
  Sparkles,
} from 'lucide-react';
import {
  CartesianGrid,
  Bar,
  ComposedChart,
  Line,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts';
import { api, type Campaign } from '@/lib/api';
import {
  type DeliveryFilter,
  type DeliveryReport,
  reportPreset,
  formatReportBucket,
  deliveryChartData,
  deliveryDetailRows,
} from '@/lib/delivery-report';
import { Button } from '@/components/ui/button';
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  CardDescription,
} from '@/components/ui/card';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { ChartContainer } from '@/components/ui/chart';
import { FormSelect } from '@/components/form-select';
import { PageHeading } from '@/components/page-heading';
import { useAccess } from '@/components/auth-gate';
import { Checkbox } from '@/components/ui/checkbox';
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible';

import {
  ReportFilters,
  initialReportSelection,
  selectedReportFilter,
  type ReportSelection,
} from '@/components/report-filters';

const money = (fen: number) => `¥${(fen / 100).toFixed(2)}`;
const percent = (ratio: number) => `${(ratio * 100).toFixed(2)}%`;
const chartMetrics = [
  { value: 'impressions', label: '曝光量' },
  { value: 'clicks', label: '点击量' },
  { value: 'conversions', label: '转化量' },
  { value: 'spend', label: '消耗（元）' },
  { value: 'ctr', label: 'CTR（%）' },
  { value: 'cvr', label: 'CVR（%）' },
];

export function DeliveryReportWorkspace({
  campaigns,
  onDiagnose,
  initialFilter,
  refreshKey = 0,
  onFilterApplied,
}: {
  campaigns: Campaign[];
  onDiagnose: (filter: DeliveryFilter) => void;
  initialFilter?: DeliveryFilter;
  refreshKey?: number;
  onFilterApplied?: (filter: DeliveryFilter) => void;
}) {
  const { canOperate } = useAccess();
  const [selection, setSelection] = useState<ReportSelection>(() =>
    initialReportSelection(initialFilter),
  );
  const [filter, setFilter] = useState<DeliveryFilter>(() =>
    selectedReportFilter(initialReportSelection(initialFilter)),
  );
  const [report, setReport] = useState<DeliveryReport | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [exporting, setExporting] = useState(false);
  const [chartMetric, setChartMetric] = useState('impressions');
  const [page, setPage] = useState(0);
  const [detailsOpen, setDetailsOpen] = useState(false);
  const [showEmpty, setShowEmpty] = useState(false);
  const detailCheckboxId = useId();
  const exportController = useRef<AbortController | null>(null);
  useEffect(() => () => exportController.current?.abort(), []);
  useEffect(() => {
    const controller = new AbortController();
    api
      .deliveryReport(filter, controller.signal)
      .then((value) => {
        if (!controller.signal.aborted) {
          setReport(value);
          setError('');
        }
      })
      .catch((cause) => {
        if (!controller.signal.aborted)
          setError(cause instanceof Error ? cause.message : '报表加载失败');
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [filter, refreshKey]);
  let dirty = true;
  try {
    dirty =
      JSON.stringify(selectedReportFilter(selection)) !==
      JSON.stringify(filter);
  } catch {}
  const chartData = useMemo(
    () => (report ? deliveryChartData(report) : []),
    [report],
  );
  const rateChart = chartMetric === 'ctr' || chartMetric === 'cvr';
  const shownRange = report ? initialReportSelection(report) : null;
  function applySelection(next: ReportSelection) {
    const nextFilter = selectedReportFilter(next);
    setSelection(next);
    setLoading(true);
    setReport(null);
    setPage(0);
    setFilter(nextFilter);
    setError('');
    onFilterApplied?.(nextFilter);
  }
  function submit(e: { preventDefault: () => void }) {
    e.preventDefault();
    try {
      applySelection(selection);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : '筛选条件无效');
    }
  }
  async function download() {
    if (!report || exporting || dirty) return;
    exportController.current?.abort();
    const controller = new AbortController();
    exportController.current = controller;
    setExporting(true);
    setError('');
    try {
      const blob = await api.exportDeliveryReport(filter, controller.signal);
      if (controller.signal.aborted) return;
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `AdFlow-投放报表-${selection.start}-${selection.end}.csv`;
      a.click();
      setTimeout(() => URL.revokeObjectURL(url), 1000);
    } catch (cause) {
      if (!controller.signal.aborted)
        setError(cause instanceof Error ? cause.message : '导出失败');
    } finally {
      if (!controller.signal.aborted) setExporting(false);
    }
  }
  const metric = report?.summary;
  const detailRows = useMemo(
    () => deliveryDetailRows(report?.series ?? [], showEmpty),
    [report, showEmpty],
  );
  const pageSize = 10;
  const totalPages = Math.max(1, Math.ceil(detailRows.length / pageSize));
  const currentPage = Math.min(page, totalPages - 1);
  return (
    <>
      <PageHeading
        title="投放效果报表"
        description="查看投放趋势、消耗与转化效果。"
        action={
          <div className="flex flex-wrap gap-2">
            <Button
              type="button"
              variant="outline"
              disabled={!report || loading || dirty || exporting}
              onClick={download}
            >
              {exporting ? (
                <LoaderCircle className="animate-spin" />
              ) : (
                <Download />
              )}
              导出 CSV
            </Button>
            {canOperate && (
              <Button
                type="button"
                disabled={!report || loading || dirty}
                onClick={() => onDiagnose(filter)}
              >
                <Sparkles />
                Agent 分析
              </Button>
            )}
          </div>
        }
      />
      <Card className="mt-5">
        <CardContent className="pt-5">
          <form onSubmit={submit} className="space-y-4">
            <ReportFilters
              value={selection}
              onChange={setSelection}
              campaigns={campaigns}
            />
            <div className="flex flex-wrap items-center justify-between gap-3">
              <div className="flex flex-wrap items-center gap-2">
                {[
                  { days: 1, label: '今天' },
                  { days: 7, label: '近7天' },
                  { days: 30, label: '近30天' },
                ].map((p) => (
                  <Button
                    type="button"
                    key={p.days}
                    size="sm"
                    variant="outline"
                    onClick={() =>
                      applySelection({
                        ...selection,
                        ...reportPreset(p.days),
                        granularity: p.days === 1 ? 'hour' : 'day',
                      })
                    }
                  >
                    {p.label}
                  </Button>
                ))}
                <span className="text-xs text-muted-foreground">
                  北京时间 · 最多31天
                </span>
              </div>
              <Button type="submit" disabled={loading}>
                {loading ? (
                  <LoaderCircle className="animate-spin" />
                ) : (
                  <RefreshCw />
                )}
                查询报表
              </Button>
            </div>
          </form>
        </CardContent>
      </Card>
      {error && (
        <p
          role="alert"
          className="mt-4 rounded-lg border border-destructive/20 bg-destructive/5 p-4 text-sm text-destructive"
        >
          {error}
        </p>
      )}
      {dirty && (
        <output className="mt-5 flex min-h-40 items-center justify-center rounded-lg border border-dashed text-sm text-muted-foreground">
          点击「查询报表」查看所选范围。
        </output>
      )}
      {loading && (
        <output className="flex min-h-64 items-center justify-center gap-2 text-muted-foreground">
          <LoaderCircle className="size-5 animate-spin" />
          正在汇总投放数据…
        </output>
      )}
      {report && metric && !dirty && (
        <div className="mt-5 space-y-5">
          <p
            className="text-sm text-muted-foreground"
            aria-label="当前报表范围"
          >
            {shownRange?.start}
            {shownRange?.start !== shownRange?.end
              ? ` 至 ${shownRange?.end}`
              : ''}
            {' · '}
            {campaigns.find((c) => c.id === report.campaignId)?.name ??
              (report.campaignId ? report.campaignId : '全部计划')}
            {' · '}
            {report.granularity === 'hour' ? '按小时' : '按天'}（北京时间）
          </p>
          <div className="grid grid-cols-2 gap-3 lg:grid-cols-3 xl:grid-cols-6">
            {[
              {
                label: '曝光',
                value: metric.impressions.toLocaleString(),
                detail: '已计入统计',
              },
              {
                label: '点击',
                value: metric.clicks.toLocaleString(),
                detail: `CTR ${percent(metric.ctr)}`,
              },
              {
                label: '转化',
                value: metric.conversions.toLocaleString(),
                detail: `CVR ${percent(metric.cvr)}`,
              },
              {
                label: '实际消耗',
                value: money(metric.spendFen),
                detail: '已完成曝光结算',
              },
              {
                label: '转化价值',
                value: money(metric.valueFen),
                detail: `平均点击成本 ${money(metric.cpcFen)}`,
              },
              {
                label: '平均转化成本',
                value: money(metric.cpaFen),
                detail: '消耗 ÷ 转化次数',
              },
            ].map((item) => (
              <Card key={item.label} className="gap-2 py-4">
                <CardContent className="px-4">
                  <p className="text-sm text-muted-foreground">{item.label}</p>
                  <p className="mt-2 break-words text-2xl font-semibold tabular-nums">
                    {item.value}
                  </p>
                  <p className="mt-2 text-xs text-muted-foreground">
                    {item.detail}
                  </p>
                </CardContent>
              </Card>
            ))}
          </div>
          {metric.unpricedImpressions > 0 && (
            <output className="rounded-lg border border-amber-300/50 bg-amber-50 p-3 text-sm text-amber-900">
              有 {metric.unpricedImpressions}{' '}
              次历史曝光缺少结算凭据，消耗仅汇总可确认部分。
            </output>
          )}
          <Card>
            <CardHeader>
              <div className="flex flex-wrap items-center justify-between gap-3">
                <div>
                  <CardTitle>投放趋势</CardTitle>
                  <CardDescription className="mt-1">
                    {chartMetrics.find((m) => m.value === chartMetric)?.label}
                    {' · '}截至{' '}
                    {new Date(report.generatedAt).toLocaleString('zh-CN', {
                      timeZone: 'Asia/Shanghai',
                      month: '2-digit',
                      day: '2-digit',
                      hour: '2-digit',
                      minute: '2-digit',
                      hourCycle: 'h23',
                    })}
                  </CardDescription>
                </div>
                <div className="w-40">
                  <FormSelect
                    label="趋势指标"
                    value={chartMetric}
                    onChange={setChartMetric}
                    options={chartMetrics}
                  />
                </div>
              </div>
            </CardHeader>
            <CardContent>
              {report.granularity === 'hour' && chartData.length > 72 && (
                <div className="mb-3 flex flex-wrap items-center justify-between gap-2 text-sm text-muted-foreground">
                  <span>时段较多，按天查看更清楚。</span>
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() =>
                      applySelection({ ...selection, granularity: 'day' })
                    }
                  >
                    切换按天
                  </Button>
                </div>
              )}
              {!metric.impressions && !metric.clicks && !metric.conversions ? (
                <div className="flex min-h-60 flex-col items-center justify-center gap-2 text-sm text-muted-foreground">
                  <p>所选时间内暂无投放数据</p>
                  <p>完成一次投放及事件回传后，点击查询即可查看。</p>
                </div>
              ) : (
                <ChartContainer
                  config={{
                    [chartMetric]: {
                      label: chartMetrics.find((m) => m.value === chartMetric)
                        ?.label,
                      color: 'var(--primary)',
                    },
                  }}
                  className="h-[280px] w-full aspect-auto"
                >
                  <ComposedChart
                    data={chartData}
                    margin={{ top: 12, right: 16, bottom: 0, left: 0 }}
                    accessibilityLayer
                  >
                    <CartesianGrid vertical={false} />
                    <XAxis
                      dataKey="label"
                      tickLine={false}
                      axisLine={false}
                      minTickGap={36}
                      tickFormatter={(label: string) =>
                        shownRange?.start === shownRange?.end &&
                        report.granularity === 'hour'
                          ? label.slice(-5)
                          : label
                      }
                    />
                    <YAxis
                      tickLine={false}
                      axisLine={false}
                      width={55}
                      allowDecimals={
                        !['impressions', 'clicks', 'conversions'].includes(
                          chartMetric,
                        )
                      }
                    />
                    <Tooltip
                      formatter={(v) => [
                        Number(v).toLocaleString('zh-CN', {
                          maximumFractionDigits: 2,
                        }),
                        chartMetrics.find((m) => m.value === chartMetric)
                          ?.label,
                      ]}
                      contentStyle={{
                        background: 'var(--card)',
                        borderColor: 'var(--border)',
                        borderRadius: 8,
                      }}
                    />
                    {rateChart ? (
                      <Line
                        dataKey={chartMetric}
                        type="linear"
                        stroke="var(--primary)"
                        strokeWidth={2}
                        dot={chartData.length <= 31}
                        connectNulls={false}
                        isAnimationActive={false}
                      />
                    ) : (
                      <Bar
                        dataKey={chartMetric}
                        fill="var(--primary)"
                        maxBarSize={32}
                        radius={[3, 3, 0, 0]}
                        isAnimationActive={false}
                      />
                    )}
                  </ComposedChart>
                </ChartContainer>
              )}
            </CardContent>
          </Card>
          <Collapsible open={detailsOpen} onOpenChange={setDetailsOpen}>
            <Card className="min-w-0">
              <CardHeader>
                <div className="flex items-center justify-between gap-3">
                  <CardTitle>时段明细</CardTitle>
                  <CollapsibleTrigger
                    render={<Button variant="ghost" size="sm" />}
                  >
                    {detailsOpen ? '收起明细' : '查看明细'}
                    <ChevronDown className={detailsOpen ? 'rotate-180' : ''} />
                  </CollapsibleTrigger>
                </div>
              </CardHeader>
              <CollapsibleContent>
                <CardContent className="px-0">
                  <div className="flex flex-wrap items-center justify-between gap-3 px-4 pb-4 text-sm">
                    <span className="text-muted-foreground">
                      最近时段在前 · 每页 10 条
                    </span>
                    <label
                      htmlFor={detailCheckboxId}
                      className="flex cursor-pointer items-center gap-2"
                    >
                      <Checkbox
                        id={detailCheckboxId}
                        checked={showEmpty}
                        onCheckedChange={(checked) => {
                          setShowEmpty(checked);
                          setPage(0);
                        }}
                      />
                      显示空时段
                    </label>
                  </div>
                  <Table className="min-w-[860px]">
                    <TableHeader>
                      <TableRow>
                        {[
                          '时段（北京时间）',
                          '曝光',
                          '点击',
                          '转化',
                          '消耗',
                          '转化价值',
                          'CTR',
                          'CVR',
                        ].map((h) => (
                          <TableHead key={h} className="px-4">
                            {h}
                          </TableHead>
                        ))}
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {detailRows.length === 0 && (
                        <TableRow>
                          <TableCell
                            colSpan={8}
                            className="py-8 text-center text-muted-foreground"
                          >
                            此范围没有投放记录
                          </TableCell>
                        </TableRow>
                      )}
                      {detailRows
                        .slice(
                          currentPage * pageSize,
                          (currentPage + 1) * pageSize,
                        )
                        .map((p) => (
                          <TableRow key={p.bucket}>
                            <TableCell className="px-4 whitespace-nowrap">
                              {formatReportBucket(p.bucket, report.granularity)}
                            </TableCell>
                            {[
                              p.impressions.toLocaleString(),
                              p.clicks.toLocaleString(),
                              p.conversions.toLocaleString(),
                              money(p.spendFen),
                              money(p.valueFen),
                              percent(p.ctr),
                              percent(p.cvr),
                            ].map((v, i) => (
                              <TableCell key={i} className="px-4 tabular-nums">
                                {v}
                              </TableCell>
                            ))}
                          </TableRow>
                        ))}
                    </TableBody>
                  </Table>
                  <div className="flex items-center justify-between gap-2 border-t px-4 pt-4 text-sm">
                    <span className="text-muted-foreground">
                      共 {detailRows.length} 个时段 · {currentPage + 1}/
                      {totalPages} 页
                    </span>
                    <div className="flex gap-2">
                      <Button
                        type="button"
                        size="sm"
                        variant="outline"
                        disabled={currentPage === 0}
                        onClick={() => setPage(currentPage - 1)}
                      >
                        上一页
                      </Button>
                      <Button
                        type="button"
                        size="sm"
                        variant="outline"
                        disabled={currentPage >= totalPages - 1}
                        onClick={() => setPage(currentPage + 1)}
                      >
                        下一页
                      </Button>
                    </div>
                  </div>
                </CardContent>
              </CollapsibleContent>
            </Card>
          </Collapsible>
        </div>
      )}
    </>
  );
}
