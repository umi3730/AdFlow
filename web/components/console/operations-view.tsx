'use client';

import { useCallback, useEffect, useState } from 'react';
import {
  Activity,
  AlertTriangle,
  DatabaseZap,
  Inbox,
  RadioTower,
  RefreshCw,
  RotateCcw,
} from 'lucide-react';
import {
  api,
  KafkaPartitionLag,
  OutboxRecord,
  OutboxStats,
  type TraceTarget,
} from '@/lib/api';
import { useAccess } from '@/components/auth-gate';
import { Button } from '@/components/ui/button';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { PageHeading } from '@/components/page-heading';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { MetricCard, StatusBadge, Empty } from './shared';

export function OperationsView({
  busy,
  run,
  connected,
  onInspectRequest,
}: {
  busy: boolean;
  run: (action: () => Promise<void>, message: string) => Promise<void>;
  connected: boolean;
  onInspectRequest: (target: TraceTarget) => void;
}) {
  const { canAdmin } = useAccess();
  const [inspectId, setInspectId] = useState('');
  const traceLookup = (
    <form
      className="my-5 flex flex-wrap items-end gap-3 rounded-lg border bg-card p-4"
      onSubmit={(event) => {
        event.preventDefault();
        if (inspectId.trim()) onInspectRequest({ requestId: inspectId.trim() });
      }}
    >
      <label
        htmlFor="trace-request-id"
        className="min-w-0 flex-1 space-y-2 text-sm"
      >
        按请求查看处理过程
        <Input
          id="trace-request-id"
          aria-label="查询请求 ID"
          placeholder="粘贴测试结果中的 requestId"
          value={inspectId}
          maxLength={128}
          onChange={(event) => setInspectId(event.target.value)}
        />
      </label>
      <Button type="submit" variant="outline" disabled={!inspectId.trim()}>
        查询请求
      </Button>
    </form>
  );
  const [runtimeMode, setRuntimeMode] = useState<Awaited<
    ReturnType<typeof api.operationsMode>
  > | null>(null);
  const [status, setStatus] = useState('');
  const [records, setRecords] = useState<OutboxRecord[]>([]);
  const [stats, setStats] = useState<OutboxStats>({
    pending: 0,
    processing: 0,
    published: 0,
    deadLettered: 0,
  });
  const [lag, setLag] = useState<KafkaPartitionLag[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  const load = useCallback(async () => {
    setLoading(true);
    setError('');
    setRuntimeMode(null);
    try {
      const mode = await api.operationsMode();
      setRuntimeMode(mode);
      if (!mode.outboxEnabled) {
        setRecords([]);
        setLag([]);
        setStats({ pending: 0, processing: 0, published: 0, deadLettered: 0 });
        return;
      }
      const [outbox, kafka] = await Promise.all([
        api.operationsOutbox(status),
        api.operationsKafkaLag(),
      ]);
      setRecords(outbox.items);
      setStats(outbox.stats);
      setLag(kafka.items);
    } catch (loadError) {
      setError(
        loadError instanceof Error ? loadError.message : '运行态加载失败',
      );
    } finally {
      setLoading(false);
    }
  }, [status]);

  useEffect(() => {
    void Promise.resolve().then(load);
  }, [load]);

  const totalLag = lag.reduce((sum, item) => sum + item.lag, 0);
  const unhealthy =
    stats.pending +
    stats.processing +
    stats.deadLettered +
    (stats.settling ?? 0) +
    (stats.reconcile ?? 0);
  const available = runtimeMode?.outboxEnabled && !loading && !error;
  if (runtimeMode?.eventTransport === 'sync' && !loading)
    return (
      <>
        <PageHeading
          title="事件处理"
          description="当前事件处理模式由后端配置决定。"
        />
        {traceLookup}
        <Card className="mt-7">
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <DatabaseZap className="size-5 text-primary" />
              同步事件模式
            </CardTitle>
            <CardDescription>
              未启用 MySQL Outbox 与 Kafka 异步链路，不展示积压或死信指标。
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <p className="text-sm text-muted-foreground">
              曝光、点击和转化当前同步计入内存统计，可在总览查看。这里没有消息记录不等于
              Kafka 链路健康；切换为 Kafka 模式后才会显示投递和消费状态。
            </p>
            <Button type="button" variant="outline" onClick={() => void load()}>
              <RefreshCw />
              重新检查运行模式
            </Button>
          </CardContent>
        </Card>
      </>
    );
  return (
    <>
      <PageHeading
        title="事件处理"
        description={
          loading
            ? '正在读取运行模式…'
            : error
              ? '运行状态不可用，请重试。'
              : 'Kafka 异步模式 · 追踪 Outbox 投递、分区积压与死信重放。'
        }
        action={
          <Button
            variant="outline"
            onClick={() => void load()}
            disabled={loading}
          >
            <RefreshCw className={loading ? 'animate-spin' : ''} />
            刷新运行态
          </Button>
        }
      />
      {traceLookup}
      <section className="metric-strip">
        <MetricCard
          label="待发布"
          value={available ? String(stats.pending) : '—'}
          change="Outbox pending"
          icon={Inbox}
          tone="amber"
        />
        <MetricCard
          label="处理中"
          value={available ? String(stats.processing) : '—'}
          change="持有 Relay 租约"
          icon={Activity}
          tone="blue"
        />
        <MetricCard
          label="Kafka Lag"
          value={available && lag.length ? totalLag.toLocaleString() : '—'}
          change={
            available && lag.length
              ? `${lag.length} 个分区`
              : '尚无有效分区采样'
          }
          icon={RadioTower}
          tone="violet"
        />
        <MetricCard
          label="死信"
          value={available ? String(stats.deadLettered) : '—'}
          change={
            !available
              ? '尚未获得有效状态'
              : unhealthy === 0
                ? '当前无待处理或死信'
                : '需要运维关注'
          }
          icon={AlertTriangle}
          tone={stats.deadLettered > 0 ? 'rose' : 'mint'}
        />
      </section>

      <div className="mt-6 grid gap-5 xl:grid-cols-[minmax(0,1fr)_320px]">
        <Card className="min-w-0">
          <CardHeader className="border-b">
            <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
              <div>
                <CardTitle>Outbox 事件</CardTitle>
                {available &&
                  ((stats.settling ?? 0) > 0 || (stats.reconcile ?? 0) > 0) && (
                    <p className="text-sm text-muted-foreground">
                      等待结算 {stats.settling ?? 0} 条 · 待核对{' '}
                      {stats.reconcile ?? 0} 条。结算完成后才会发布并计入统计。
                    </p>
                  )}
                <CardDescription>
                  最近 50 条，死信可由管理员重新入队。
                </CardDescription>
              </div>
              <select
                value={status}
                disabled={!runtimeMode?.outboxEnabled || loading}
                onChange={(event) => setStatus(event.target.value)}
                className="h-9 rounded-lg border border-input bg-background px-3 text-sm"
              >
                <option value="">全部状态</option>
                <option value="PENDING">待发布</option>
                <option value="SETTLING">等待结算</option>
                <option value="RECONCILE">待核对</option>
                <option value="PROCESSING">处理中</option>
                <option value="PUBLISHED">已发布</option>
                <option value="DEAD_LETTERED">死信</option>
              </select>
            </div>
          </CardHeader>
          <CardContent className="overflow-x-auto p-0">
            {error ? (
              <Empty text={error} />
            ) : loading ? (
              <Empty text="正在读取投递状态…" />
            ) : records.length === 0 ? (
              <Empty
                text={
                  connected
                    ? '当前筛选下没有 Outbox 事件'
                    : '连接 API 后查看真实运行态'
                }
              />
            ) : (
              <Table className="min-w-[760px]">
                <TableHeader>
                  <TableRow>
                    <TableHead className="pl-5">事件</TableHead>
                    <TableHead>状态</TableHead>
                    <TableHead>重试</TableHead>
                    <TableHead>时间</TableHead>
                    <TableHead>最近错误</TableHead>
                    <TableHead className="pr-5 text-right">操作</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {records.map((record) => (
                    <TableRow key={record.event.eventId}>
                      <TableCell className="pl-5">
                        <p className="font-medium">{record.event.type}</p>
                        <p className="max-w-52 truncate font-mono text-[11px] text-muted-foreground">
                          {record.event.eventId}
                        </p>
                      </TableCell>
                      <TableCell>
                        <StatusBadge status={record.status} />
                      </TableCell>
                      <TableCell className="tabular-nums">
                        {record.attempts}
                      </TableCell>
                      <TableCell className="whitespace-nowrap text-xs text-muted-foreground">
                        {new Date(record.createdAt).toLocaleString('zh-CN')}
                      </TableCell>
                      <TableCell
                        className="max-w-64 truncate text-xs text-rose-700"
                        title={record.lastError}
                      >
                        {record.lastError || '—'}
                      </TableCell>
                      <TableCell className="pr-5 text-right">
                        <Button
                          size="xs"
                          variant="ghost"
                          onClick={() =>
                            onInspectRequest({
                              requestId: record.event.requestId,
                            })
                          }
                        >
                          查看处理过程
                        </Button>
                        {record.status === 'DEAD_LETTERED' && (
                          <Button
                            size="xs"
                            variant="outline"
                            disabled={busy || !canAdmin || !available}
                            onClick={() =>
                              void run(async () => {
                                await api.replayDeadLetter(
                                  record.event.eventId,
                                );
                                await load();
                              }, '死信已重新进入 Outbox 队列')
                            }
                          >
                            <RotateCcw />
                            重放
                          </Button>
                        )}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            )}
          </CardContent>
        </Card>

        <Card className="self-start bg-card text-card-foreground xl:sticky xl:top-24">
          <CardHeader>
            <CardTitle>Kafka 分区</CardTitle>
            <CardDescription>
              消费进度由运行中的 Consumer 实时上报。
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            {lag.length === 0 ? (
              <p className="rounded-lg border bg-card p-4 text-sm text-muted-foreground">
                暂无分区采样；Consumer 收到消息后会显示。
              </p>
            ) : (
              lag.map((item) => (
                <div
                  key={`${item.topic}-${item.partition}`}
                  className="rounded-lg border bg-card p-3"
                >
                  <div className="flex items-center justify-between">
                    <span className="font-mono text-xs text-muted-foreground">
                      P{item.partition}
                    </span>
                    <span
                      className={
                        item.lag > 0 ? 'text-amber-700' : 'text-emerald-700'
                      }
                    >
                      {item.lag.toLocaleString()} lag
                    </span>
                  </div>
                  <div className="mt-2 h-1.5 overflow-hidden rounded-full bg-muted">
                    <div
                      className={`h-full rounded-full ${item.lag > 0 ? 'bg-amber-300' : 'bg-emerald-300'}`}
                      style={{
                        width: `${item.lag > 0 ? Math.min(100, 12 + Math.log10(item.lag + 1) * 28) : 4}%`,
                      }}
                    />
                  </div>
                  <p className="mt-2 truncate text-xs text-muted-foreground">
                    {item.topic}
                  </p>
                </div>
              ))
            )}
          </CardContent>
        </Card>
      </div>
    </>
  );
}
