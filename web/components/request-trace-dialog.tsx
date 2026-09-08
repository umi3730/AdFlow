'use client';

import { useEffect, useState } from 'react';
import { LoaderCircle, RefreshCw } from 'lucide-react';
import { api, type RequestTrace, type TraceTarget } from '@/lib/api';
import {
  eventCompletion,
  settlementSummary,
  traceAdvice,
  traceReasonLabels,
  traceStatusLabels,
  traceTime,
} from '@/lib/request-trace';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';

export function RequestTraceDialog({
  target,
  onClose,
}: {
  target: TraceTarget | null;
  onClose: () => void;
}) {
  const requestId = target?.requestId ?? '';
  const run = target?.simulationRunId ?? '',
    user = target?.simulationUserId ?? '';
  const key = JSON.stringify([requestId, run, user]);
  const [refresh, setRefresh] = useState(0);
  const [result, setResult] = useState<{
    key: string;
    loading: boolean;
    data?: RequestTrace;
    error?: string;
  }>({ key: '', loading: false });
  useEffect(() => {
    if (!requestId) return;
    let live = true;
    const controller = new AbortController();
    void Promise.resolve().then(async () => {
      if (!live) return;
      setResult({ key, loading: true });
      try {
        const data = await api.requestTrace(
          {
            requestId,
            simulationRunId: run || undefined,
            simulationUserId: user || undefined,
          },
          controller.signal,
        );
        if (live) setResult({ key, loading: false, data });
      } catch (error) {
        if (live)
          setResult({
            key,
            loading: false,
            error: error instanceof Error ? error.message : '无法读取请求详情',
          });
      }
    });
    return () => {
      live = false;
      controller.abort();
    };
  }, [key, requestId, run, user, refresh]);
  const current = result.key === key ? result : null;
  const loading = Boolean(target) && (!current || current.loading);
  const trace = current?.data;
  return (
    <Dialog
      open={Boolean(target)}
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <DialogContent className="max-h-[88dvh] overflow-y-auto sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle>请求处理过程</DialogTitle>
          <DialogDescription>
            查看已保存的决策和回执；刷新只读取状态，不会重新投放或重放事件。
          </DialogDescription>
        </DialogHeader>
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            <p className="text-xs text-muted-foreground">请求 ID</p>
            <p className="mt-1 break-all font-mono text-xs">
              {trace?.requestId ?? requestId}
            </p>
          </div>
          <Button
            variant="outline"
            size="sm"
            disabled={loading}
            onClick={() => setRefresh((value) => value + 1)}
          >
            <RefreshCw className={loading ? 'animate-spin' : ''} />
            刷新
          </Button>
        </div>
        {loading && (
          <output className="flex items-center gap-2 py-8 text-sm text-muted-foreground">
            <LoaderCircle className="size-4 animate-spin" />
            正在读取处理记录…
          </output>
        )}
        {target?.clientOutcome &&
          target.clientOutcome !== '命中' &&
          !target.clientOutcome.startsWith('未命中：') && (
            <p className="rounded-md bg-muted/60 p-3 text-sm">
              页面记录的结果：{target.clientOutcome}。
              {/timeout|network|已取消/.test(target.clientOutcome) &&
                '浏览器超时或取消不代表后台没有处理。'}
            </p>
          )}
        {current?.error && (
          <p
            role="alert"
            className="rounded-lg bg-destructive/10 p-4 text-sm text-destructive"
          >
            {current.error}
          </p>
        )}
        {trace && (
          <div className="space-y-5">
            <p className="text-xs text-muted-foreground">
              {trace.eventTransport === 'kafka'
                ? '持久化异步记录'
                : '本进程同步记录，重启后不保留'}{' '}
              · 查询时间 {traceTime(trace.observedAt)}（北京时间）
            </p>
            <section className="rounded-lg border p-4" aria-label="决策记录">
              <div className="flex items-center justify-between gap-3">
                <h3 className="font-semibold">1. 决策</h3>
                <Badge variant="outline">
                  {trace.decision
                    ? trace.decision.matched
                      ? '已命中'
                      : '未命中'
                    : (traceStatusLabels[trace.execution?.status ?? ''] ??
                      '尚无已保存结果')}
                </Badge>
              </div>
              {trace.decision ? (
                <>
                  <p className="mt-2 text-sm">
                    {traceReasonLabels[trace.decision.reason] ??
                      trace.decision.reason}
                  </p>
                  {trace.decision.matched && !trace.decision.pricing && (
                    <p className="mt-2 text-sm text-muted-foreground">
                      本条决策未保存成交价，无法从当前配置补算。
                    </p>
                  )}
                  <dl className="mt-3 grid gap-3 text-sm sm:grid-cols-2">
                    <TraceValue label="用户 ID" value={trace.decision.userId} />
                    <TraceValue label="广告位" value={trace.decision.slotId} />
                    {trace.decision.campaignId && (
                      <TraceValue
                        label="计划 ID"
                        value={trace.decision.campaignId}
                      />
                    )}
                    {trace.decision.creativeId && (
                      <TraceValue
                        label="素材 ID"
                        value={trace.decision.creativeId}
                      />
                    )}
                    {trace.decision.pricing && (
                      <TraceValue
                        label="已保存的单次曝光成交价"
                        value={`¥${(trace.decision.pricing.priceFen / 100).toFixed(2)}${trace.decision.pricing.advertiserName ? ' · ' + trace.decision.pricing.advertiserName : ''}`}
                      />
                    )}
                    {trace.decision.pricing?.version ? (
                      <TraceValue
                        label="成交规则版本"
                        value={`v${trace.decision.pricing.version}`}
                      />
                    ) : null}
                    {trace.decision.pricing && (
                      <TraceValue
                        label="已保存的投放方式"
                        value={
                          trace.decision.pricing.mode === 'first_price'
                            ? '一价竞价'
                            : '固定成本（兜底）'
                        }
                      />
                    )}
                    {trace.decision.expiresAt && (
                      <TraceValue
                        label="曝光回传截止"
                        value={traceTime(trace.decision.expiresAt)}
                      />
                    )}
                  </dl>
                  {trace.decision.pricing &&
                    trace.decision.pricing.rank > 0 && (
                      <details className="mt-3 text-sm">
                        <summary className="cursor-pointer text-muted-foreground">
                          已保存的筛选摘要
                        </summary>
                        <p className="mt-2 leading-6">
                          参与竞价广告主 {trace.decision.pricing.advertisers}{' '}
                          个，成交尝试顺位 {trace.decision.pricing.rank}
                          ；此前预算拒绝 {
                            trace.decision.pricing.budgetRejected
                          }{' '}
                          次，频控拒绝{' '}
                          {trace.decision.pricing.frequencyRejected} 次。
                        </p>
                        <p className="mt-1 text-xs text-muted-foreground">
                          未保存逐个候选的历史判定明细。
                        </p>
                      </details>
                    )}
                </>
              ) : (
                <p className="mt-2 text-sm text-muted-foreground">
                  {trace.execution?.status === 'RUNNING'
                    ? `执行租约有效至 ${traceTime(trace.execution.leaseUntil)}，可稍后刷新。`
                    : '尚无决策结果记录。无法从当前规则反推出这次请求的历史选择。'}
                </p>
              )}
            </section>
            <section
              className="rounded-lg border p-4"
              aria-label="曝光结算记录"
            >
              <h3 className="font-semibold">2. 曝光结算</h3>
              <p className="mt-2 text-sm">{settlementSummary(trace)}</p>
              {trace.settlement && (
                <>
                  <dl className="mt-3 grid gap-3 text-sm sm:grid-cols-2">
                    <TraceValue
                      label="曝光受理"
                      value={traceTime(trace.settlement.acceptedAt)}
                    />
                    {trace.settlement.settledAt && (
                      <TraceValue
                        label="结算确认"
                        value={traceTime(trace.settlement.settledAt)}
                      />
                    )}
                    {trace.settlement.failedAttempts > 0 && (
                      <TraceValue
                        label="结算失败次数"
                        value={String(trace.settlement.failedAttempts)}
                      />
                    )}
                    {trace.settlement.nextAttemptAt && (
                      <TraceValue
                        label="下次尝试"
                        value={traceTime(trace.settlement.nextAttemptAt)}
                      />
                    )}
                  </dl>
                  {trace.settlement.lastError && (
                    <p className="mt-3 break-all rounded-md bg-destructive/10 p-3 text-sm text-destructive">
                      {trace.settlement.lastError}
                    </p>
                  )}
                  {traceAdvice(
                    trace.settlement.status,
                    trace.eventTransport,
                  ) && (
                    <p className="mt-3 text-sm text-muted-foreground">
                      {traceAdvice(
                        trace.settlement.status,
                        trace.eventTransport,
                      )}
                    </p>
                  )}
                </>
              )}
            </section>
            <section aria-label="事件与计量记录">
              <h3 className="font-semibold">3. 事件与计量</h3>
              {!trace.events.length && (
                <p className="mt-2 text-sm text-muted-foreground">
                  {trace.decision && !trace.decision.matched
                    ? '本次未返回广告，没有需要回传的投放事件。'
                    : '尚无事件回执。命中广告后还需要回传曝光，才会进入结算与统计。'}
                </p>
              )}
              <div className="mt-3 space-y-3">
                {trace.events.map((event) => (
                  <article
                    key={event.eventId}
                    className="rounded-lg border p-4"
                  >
                    <div className="flex flex-wrap items-center justify-between gap-2">
                      <h4 className="text-sm font-semibold">
                        {{
                          impression: '曝光',
                          click: '点击',
                          conversion: '转化',
                        }[event.type] ?? event.type}
                      </h4>
                      <Badge variant="outline">{eventCompletion(event)}</Badge>
                    </div>
                    <p className="mt-2 break-all font-mono text-xs text-muted-foreground">
                      {event.eventId}
                    </p>
                    <dl className="mt-3 grid gap-3 text-sm sm:grid-cols-3">
                      <TraceValue
                        label="回执保存"
                        value={traceTime(event.acceptedAt)}
                      />
                      <TraceValue
                        label="发布确认"
                        value={
                          trace.eventTransport === 'sync'
                            ? '同步处理，不经过 Kafka'
                            : event.publishedAt
                              ? traceTime(event.publishedAt)
                              : (traceStatusLabels[event.status] ??
                                event.status)
                        }
                      />
                      <TraceValue
                        label="计入统计"
                        value={
                          event.processedAt
                            ? traceTime(event.processedAt)
                            : '尚无计量记录'
                        }
                      />
                    </dl>
                    {event.type === 'conversion' && (
                      <p className="mt-3 text-sm">
                        模拟转化价值 ¥{((event.valueFen ?? 0) / 100).toFixed(2)}
                      </p>
                    )}
                    {event.lastError && (
                      <p className="mt-3 break-all rounded-md bg-destructive/10 p-3 text-sm text-destructive">
                        {event.lastError}
                      </p>
                    )}
                    {event.failedAttempts > 0 && (
                      <p className="mt-2 text-xs text-muted-foreground">
                        当前阶段失败 {event.failedAttempts} 次
                        {event.nextAttemptAt
                          ? `；下次尝试 ${traceTime(event.nextAttemptAt)}`
                          : ''}
                      </p>
                    )}
                    {!event.processedAt &&
                      traceAdvice(event.status, trace.eventTransport) && (
                        <p className="mt-2 text-sm text-muted-foreground">
                          {traceAdvice(event.status, trace.eventTransport)}
                        </p>
                      )}
                    {event.processedAt &&
                      trace.eventTransport === 'kafka' &&
                      event.status !== 'PUBLISHED' && (
                        <p className="mt-2 text-xs text-muted-foreground">
                          计量记录已存在，投递确认状态尚未更新。
                        </p>
                      )}
                  </article>
                ))}
              </div>
              {trace.truncated && (
                <p className="mt-3 text-sm text-muted-foreground">
                  仅展示前 100 条事件，该请求还有更多记录。
                </p>
              )}
            </section>
          </div>
        )}
        <p className="text-xs leading-5 text-muted-foreground">
          仅展示已保存记录。未持久化的校验拒绝或消费错误，需要结合接口响应和服务日志核对；这里不重算历史候选。
        </p>
      </DialogContent>
    </Dialog>
  );
}
function TraceValue({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0">
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="mt-1 break-all">{value}</dd>
    </div>
  );
}
