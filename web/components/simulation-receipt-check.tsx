'use client';

import { useEffect, useRef, useState } from 'react';
import { api } from '@/lib/api';
import type { SimulationSnapshot } from '@/lib/user-pool-simulator';
import { simulationTraceTarget } from '@/lib/request-trace';
import { receiptCounts } from '@/lib/simulation-receipts';
import { Button } from '@/components/ui/button';

export function SimulationReceiptCheck({
  snapshot,
  temporary,
  active,
}: {
  snapshot: SimulationSnapshot;
  temporary: boolean;
  active: boolean;
}) {
  const controller = useRef<AbortController | null>(null);
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState<{
    processed: number;
    attention: number;
    pending: number;
    unknown: number;
  } | null>(null);
  const rows = snapshot.recent.filter((row) => row.acceptedEventIds.length > 0);
  const total = rows.reduce((sum, row) => sum + row.acceptedEventIds.length, 0);
  useEffect(() => {
    if (!active) {
      controller.current?.abort();
    }
    return () => controller.current?.abort();
  }, [active]);

  async function check() {
    if (controller.current || !active) return;
    const request = new AbortController();
    controller.current = request;
    setBusy(true);
    setResult(null);
    const summary = { processed: 0, attention: 0, pending: 0, unknown: 0 };
    try {
      // At most 20 recent requests, read sequentially; no background polling or replay.
      for (const row of rows) {
        if (request.signal.aborted) return;
        const timeout = new AbortController();
        const timer = setTimeout(() => timeout.abort(), 3000);
        try {
          const trace = await api.requestTrace(
            simulationTraceTarget(
              row.requestId,
              row.userId,
              snapshot.runId,
              temporary,
            ),
            AbortSignal.any([request.signal, timeout.signal]),
          );
          const counts = receiptCounts(row.acceptedEventIds, trace);
          summary.processed += counts.processed;
          summary.attention += counts.attention;
          summary.pending += counts.pending;
        } catch {
          summary.unknown += row.acceptedEventIds.length;
        } finally {
          clearTimeout(timer);
        }
      }
      if (!request.signal.aborted) setResult(summary);
    } finally {
      if (controller.current === request) {
        controller.current = null;
        setBusy(false);
      }
    }
  }

  if (!total) return null;
  return (
    <section
      aria-label="统计确认"
      className="mt-4 space-y-2 rounded-lg border p-3 text-sm"
    >
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="font-medium">
          统计确认 · 最近 {snapshot.recent.length} 轮
        </p>
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={busy || !active}
          onClick={() => void check()}
        >
          {busy ? '正在核对…' : result ? '重新核对统计' : '核对统计'}
        </Button>
      </div>
      <p className="text-muted-foreground">
        核对其中已接收的 {total} 条事件；超过 20
        轮时仅检查最近记录，不代表整批结果。
      </p>
      <output aria-live="polite" className="block">
        {busy
          ? '正在读取计量记录…'
          : result
            ? `已计入统计 ${result.processed}/${total} 条 · 尚未确认 ${result.pending} 条 · 需处理 ${result.attention} 条 · 查询失败 ${result.unknown} 条`
            : '回传已接收，最终是否计入统计尚未核对。'}
      </output>
      {result && result.processed < total && (
        <p className="text-muted-foreground">
          可稍后重新核对；需处理或查询失败的记录，请从下方“查看处理过程”排查。查询不会重放事件。
        </p>
      )}
    </section>
  );
}
