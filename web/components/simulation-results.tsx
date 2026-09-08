'use client';
import { memo } from 'react';
import type {
  SimulationConfig,
  SimulationSnapshot,
} from '@/lib/user-pool-simulator';
import { simulationProgress } from '@/lib/simulation-presentation';
import { Button } from '@/components/ui/button';
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardContent,
} from '@/components/ui/card';
import {
  Table,
  TableHeader,
  TableHead,
  TableRow,
  TableBody,
  TableCell,
} from '@/components/ui/table';
const reasonLabels: Record<string, string> = {
  targeting_miss: '定向未命中',
  profile_not_found: '画像不存在',
  no_candidate: '无候选计划/素材',
  frequency_capped: '触达频控',
  budget_exhausted: '预算不足',
  dependency_unavailable: '依赖不可用',
};
const stopLabels: Record<string, string> = {
  duration: '达到设定时长，停止补发并等待在途结束',
  round_limit: '达到最大轮数，停止补发并等待在途结束',
  manual: '已立即停止，取消在途请求',
  manual_drain: '已停止补发，等待已发出的轮次完成',
  hidden: '页面进入后台',
  left_page: '离开模拟页面',
  unmounted: '页面关闭',
};

export const SimulationResults = memo(function SimulationResults({
  snapshot,
  runConfig,
  preview,
  startedAt,
  onInspectRequest,
}: {
  snapshot: SimulationSnapshot | null;
  runConfig: SimulationConfig | null;
  preview: { seconds: number; maxRounds: number; concurrency: number };
  startedAt: string;
  onInspectRequest?: (
    requestId: string,
    userId: string,
    outcome: string,
  ) => void;
}) {
  const settled = snapshot ? snapshot.successful + snapshot.failed : 0;
  const decisions = snapshot ? snapshot.matched + snapshot.noAd : 0;
  const actualRate =
    snapshot && snapshot.elapsedMs ? settled / (snapshot.elapsedMs / 1000) : 0;
  const phase = !snapshot
    ? '未开始'
    : snapshot.status === 'running'
      ? '运行中'
      : snapshot.status === 'stopping'
        ? '结束中'
        : snapshot.status === 'completed'
          ? '已完成'
          : '已停止';
  const progress = simulationProgress(
    runConfig ?? {
      seconds: preview.seconds,
      maxRounds: preview.maxRounds,
    },
    snapshot,
  );
  const mainReason =
    snapshot && Object.entries(snapshot.reasons).sort((a, b) => b[1] - a[1])[0];

  return (
    <div className="min-w-0 space-y-5 xl:col-span-2">
      <Card>
        <CardHeader>
          <div className="flex flex-wrap justify-between gap-2">
            <CardTitle>
              3. 运行结果 · <output>{phase}</output>
            </CardTitle>
            <div className="text-sm text-muted-foreground sm:text-right">
              <p className="font-medium text-foreground">{progress.primary}</p>
              <p>{progress.secondary}</p>
            </div>
          </div>
          <CardDescription>
            {snapshot?.stopReason
              ? stopLabels[snapshot.stopReason]
              : '轮数或时间上限先到即停止补发，等待在途结束。'}
          </CardDescription>
        </CardHeader>
        <CardContent>
          <p className="mb-4 text-sm text-muted-foreground">
            {snapshot
              ? '本次记录开始于 ' +
                new Date(startedAt).toLocaleTimeString('zh-CN') +
                '；修改上方配置只影响下一轮。'
              : '运行后查看请求、命中和事件回传。请求正常完成不等于广告命中。'}
          </p>
          {mainReason && (
            <p className="mb-4 border-l-2 border-amber-400 bg-amber-50 px-3 py-2 text-sm text-amber-900">
              {snapshot!.noAd} 轮未命中，主要原因：
              {reasonLabels[mainReason[0]] ?? mainReason[0]}（{mainReason[1]}{' '}
              轮）。
            </p>
          )}
          <div className="grid grid-cols-2 gap-3 lg:grid-cols-6">
            {[
              ['已发决策', snapshot?.started ?? 0],
              ['广告命中', snapshot?.matched ?? 0],
              ['未命中', snapshot?.noAd ?? 0],
              ['曝光已接收', snapshot?.impressionsAccepted ?? 0],
              ['点击已接收', snapshot?.clicksAccepted ?? 0],
              ['转化已接收', snapshot?.conversionsAccepted ?? 0],
            ].map(([label, value]) => (
              <div
                key={label}
                className="border-l-2 border-primary/20 pl-3 py-1"
              >
                <p className="text-xs text-muted-foreground">{label}</p>
                <p className="mt-2 text-xl font-semibold tabular-nums">
                  {value}
                </p>
              </div>
            ))}
          </div>
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>吞吐与耗时</CardTitle>
          <CardDescription>
            浏览器测量整轮耗时，含网络及所选事件回传；不代表纯后端耗时。
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="mb-5 grid grid-cols-2 gap-4 lg:grid-cols-4">
            {[
              ['完成轮次/秒', actualRate.toFixed(1)],
              ['流程正常完成', snapshot?.successful ?? 0],
              [
                '流程失败 / 超时',
                (snapshot?.failed ?? 0) + ' / ' + (snapshot?.timeouts ?? 0),
              ],
              [
                '取消 / 在途',
                (snapshot?.canceled ?? 0) + ' / ' + (snapshot?.inFlight ?? 0),
              ],
              ['HTTP 调用总数', snapshot?.httpRequests ?? 0],
              [
                '决策命中率',
                decisions
                  ? ((snapshot!.matched / decisions) * 100).toFixed(1) + '%'
                  : '—',
              ],
              [
                '流程正常完成率',
                settled
                  ? ((snapshot!.successful / settled) * 100).toFixed(1) + '%'
                  : '—',
              ],
              [
                '模拟转化价值',
                '¥' + ((snapshot?.valueFenAccepted ?? 0) / 100).toFixed(2),
              ],
            ].map(([label, value]) => (
              <div key={label}>
                <p className="text-xs text-muted-foreground">{label}</p>
                <p className="mt-1 text-lg font-semibold tabular-nums">
                  {value}
                </p>
              </div>
            ))}
          </div>
          <div className="grid grid-cols-3 gap-3">
            {[
              ['P50', snapshot?.p50Ms],
              ['P95', snapshot?.p95Ms],
              ['P99', snapshot?.p99Ms],
            ].map(([label, value]) => (
              <div
                key={label}
                className="border-l-2 border-primary/20 pl-3 py-1"
              >
                <p className="text-xs text-muted-foreground">{label}</p>
                <p className="mt-2 font-semibold">
                  {Number(value ?? 0).toFixed(1)} ms
                </p>
              </div>
            ))}
          </div>
          <p className="mt-3 text-sm text-muted-foreground">
            目标并发 {runConfig?.concurrency ?? preview.concurrency} 轮 ·
            实际峰值 {snapshot?.peakInFlight ?? 0}{' '}
            轮；完成后补位，不建立等待队列。
          </p>
        </CardContent>
      </Card>
      <div className="grid gap-5 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle>未命中原因</CardTitle>
            <CardDescription>
              No-Ad 单独统计，不计为 HTTP 失败。
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-2">
            {Object.entries(snapshot?.reasons ?? {}).length ? (
              Object.entries(snapshot!.reasons).map(([reason, count]) => (
                <p key={reason} className="flex justify-between gap-2 text-sm">
                  <span>{reasonLabels[reason] ?? reason}</span>
                  <span>{count}</span>
                </p>
              ))
            ) : (
              <p className="text-sm text-muted-foreground">暂无未命中记录</p>
            )}
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle>错误分布</CardTitle>
            <CardDescription>
              decision 为决策，impression 为曝光，click 为点击，conversion
              为转化；数字为 HTTP 状态。
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-2">
            {Object.entries(snapshot?.errors ?? {}).length ? (
              Object.entries(snapshot!.errors).map(([code, count]) => (
                <p key={code} className="flex justify-between gap-2 text-sm">
                  <span>{code}</span>
                  <span>{count}</span>
                </p>
              ))
            ) : (
              <p className="text-sm text-muted-foreground">暂无错误</p>
            )}
          </CardContent>
        </Card>
      </div>
      <Card className="min-w-0">
        <CardHeader>
          <CardTitle>最近 20 轮</CardTitle>
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>用户 / Request</TableHead>
                <TableHead>结果</TableHead>
                <TableHead className="text-right">耗时</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {snapshot?.recent.map((row) => (
                <TableRow key={row.requestId}>
                  <TableCell>
                    <p className="text-xs">{row.userId}</p>
                    <p className="text-xs text-muted-foreground">
                      {row.requestId}
                    </p>
                    {onInspectRequest && (
                      <Button
                        type="button"
                        variant="ghost"
                        size="sm"
                        className="mt-1 h-7 px-0 text-xs"
                        onClick={() =>
                          onInspectRequest(
                            row.requestId,
                            row.userId,
                            row.outcome,
                          )
                        }
                      >
                        查看处理过程
                      </Button>
                    )}
                  </TableCell>
                  <TableCell className="text-xs">
                    <p>{row.outcome}</p>
                    {row.campaignId && (
                      <p className="mt-1 text-muted-foreground">
                        {row.advertiserName || row.campaignId}
                        {row.priceFen !== undefined
                          ? ` · ¥${(row.priceFen / 100).toFixed(2)}/次`
                          : ''}
                      </p>
                    )}
                  </TableCell>
                  <TableCell className="text-right text-xs">
                    {row.durationMs.toFixed(1)} ms
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </CardContent>
      </Card>
    </div>
  );
});
