'use client';

import { useEffect, useRef, useState } from 'react';
import { LoaderCircle, Sparkles, ArrowRight } from 'lucide-react';
import { api, type Campaign } from '@/lib/api';
import type { DeliveryDiagnosis, DeliveryFilter } from '@/lib/delivery-report';
import {
  ReportFilters,
  initialReportSelection,
  selectedReportFilter,
} from '@/components/report-filters';
import { AgentWorkspace } from '@/components/agent-workspace';
import type { RuleEditorValue } from '@/lib/campaign-rules';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { Button } from '@/components/ui/button';
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  CardDescription,
} from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';
import { Badge } from '@/components/ui/badge';
import { PageHeading } from '@/components/page-heading';

export function AgentCenter({
  campaigns,
  initialFilter,
  onCreatePlan,
  onViewReport,
}: {
  campaigns: Campaign[];
  initialFilter?: DeliveryFilter;
  onCreatePlan: (v: RuleEditorValue) => void;
  onViewReport: (filter: DeliveryFilter) => void;
}) {
  return (
    <Tabs
      defaultValue={initialFilter ? 'diagnosis' : 'draft'}
      className="gap-5"
    >
      <TabsList aria-label="Agent 功能">
        <TabsTrigger value="draft">规则生成</TabsTrigger>
        <TabsTrigger value="diagnosis">投放诊断</TabsTrigger>
      </TabsList>
      <TabsContent value="draft">
        <AgentWorkspace onCreatePlan={onCreatePlan} />
      </TabsContent>
      <TabsContent value="diagnosis">
        <DeliveryDiagnosisWorkspace
          campaigns={campaigns}
          initialFilter={initialFilter}
          onViewReport={onViewReport}
        />
      </TabsContent>
    </Tabs>
  );
}

function DeliveryDiagnosisWorkspace({
  campaigns,
  initialFilter,
  onViewReport,
}: {
  campaigns: Campaign[];
  initialFilter?: DeliveryFilter;
  onViewReport: (filter: DeliveryFilter) => void;
}) {
  const [selection, setSelection] = useState(() =>
    initialReportSelection(initialFilter),
  );
  const [question, setQuestion] = useState(
    '分析当前投放效果，找出值得优先排查的问题并给出调整建议。',
  );
  const [requestId, setRequestId] = useState('');
  const [result, setResult] = useState<DeliveryDiagnosis | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const controller = useRef<AbortController | null>(null);
  useEffect(() => () => controller.current?.abort(), []);
  async function diagnose(e: { preventDefault: () => void }) {
    e.preventDefault();
    if (busy) return;
    setError('');
    setResult(null);
    try {
      const filter = selectedReportFilter(selection);
      controller.current?.abort();
      const active = new AbortController();
      controller.current = active;
      setBusy(true);
      const value = await api.diagnoseDelivery(
        {
          ...filter,
          question: question.trim(),
          ...(requestId.trim() ? { requestId: requestId.trim() } : {}),
        },
        active.signal,
      );
      if (!active.signal.aborted) setResult(value);
    } catch (cause) {
      if (!controller.current?.signal.aborted)
        setError(cause instanceof Error ? cause.message : '诊断失败，请重试');
    } finally {
      if (!controller.current?.signal.aborted) setBusy(false);
    }
  }
  return (
    <>
      <PageHeading
        title="Agent 投放诊断"
        description="结合投放数据、计划配置和请求结果，给出有依据的调整建议。"
      />
      <Card className="mt-5">
        <CardContent className="pt-5">
          <form onSubmit={diagnose} className="space-y-5">
            <ReportFilters
              value={selection}
              onChange={(value) => {
                setSelection(value);
                setResult(null);
              }}
              campaigns={campaigns}
              disabled={busy}
            />
            <div className="grid gap-4 lg:grid-cols-[minmax(0,2fr)_minmax(0,1fr)]">
              <label
                htmlFor="diagnosis-question"
                className="grid gap-2 text-sm"
              >
                想了解的问题
                <Textarea
                  id="diagnosis-question"
                  aria-label="诊断问题"
                  value={question}
                  maxLength={1000}
                  disabled={busy}
                  onChange={(e) => {
                    setQuestion(e.target.value);
                    setResult(null);
                  }}
                  className="min-h-24"
                />
              </label>
              <label
                htmlFor="diagnosis-request"
                className="grid content-start gap-2 text-sm"
              >
                请求 ID（可选）
                <Input
                  id="diagnosis-request"
                  aria-label="诊断请求 ID"
                  value={requestId}
                  maxLength={128}
                  disabled={busy}
                  onChange={(e) => {
                    setRequestId(e.target.value);
                    setResult(null);
                  }}
                  placeholder="粘贴一次投放的 requestId"
                />
                <span className="text-xs leading-5 text-muted-foreground">
                  补充请求后可分析定向、频控、预算或结算问题。
                </span>
              </label>
            </div>
            <div className="flex flex-wrap items-center justify-between gap-3">
              <span className="text-sm text-muted-foreground">
                生成建议后，由你确认是否调整计划。
              </span>
              <Button type="submit" disabled={busy}>
                {busy ? (
                  <LoaderCircle className="animate-spin" />
                ) : (
                  <Sparkles />
                )}
                {busy ? '正在分析…' : '开始诊断'}
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
      {busy && (
        <output className="mt-5 block text-sm text-muted-foreground">
          正在读取报表、检查配置并整理建议…
        </output>
      )}
      {result && (
        <div className="mt-5 grid items-start gap-5 xl:grid-cols-[minmax(0,1.4fr)_minmax(280px,1fr)]">
          <Card>
            <CardHeader>
              <div className="flex flex-wrap items-center justify-between gap-2">
                <CardTitle>诊断建议</CardTitle>
                <Badge variant="secondary">
                  {result.fallback
                    ? '本地诊断 · 已降级'
                    : result.provider === 'mock'
                      ? '本地诊断'
                      : '模型诊断'}
                </Badge>
              </div>
              <CardDescription>{result.model}</CardDescription>
            </CardHeader>
            <CardContent>
              <p className="mb-5 text-sm leading-7">{result.summary}</p>
              <ol className="space-y-5">
                {result.recommendations.map((r, i) => (
                  <li key={i} className="border-t pt-4">
                    <h3 className="font-medium">
                      {i + 1}. {r.title}
                    </h3>
                    <p className="mt-2 whitespace-pre-wrap text-sm leading-7 text-muted-foreground">
                      {r.action}
                    </p>
                    <div className="mt-2 flex flex-wrap gap-2">
                      {r.evidenceIds.map((id) => (
                        <a
                          key={id}
                          href={`#diagnosis-evidence-${id}`}
                          className="rounded-md bg-muted px-2 py-1 text-xs text-primary underline-offset-4 hover:underline"
                        >
                          依据：
                          {result.evidence.find((e) => e.id === id)?.title ??
                            id}
                        </a>
                      ))}
                    </div>
                  </li>
                ))}
              </ol>
              <Button
                type="button"
                className="mt-5"
                variant="outline"
                onClick={() => onViewReport(selectedReportFilter(selection))}
              >
                查看效果报表
                <ArrowRight />
              </Button>
            </CardContent>
          </Card>
          <Card>
            <CardHeader>
              <CardTitle>本次分析依据</CardTitle>
              <CardDescription>报表数据与当前配置</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              {result.evidence.map((e) => (
                <div
                  id={`diagnosis-evidence-${e.id}`}
                  key={e.id}
                  className="scroll-mt-24 border-b pb-4 last:border-0 last:pb-0"
                >
                  <h3 className="text-sm font-medium">{e.title}</h3>
                  <p className="mt-2 text-sm leading-6 text-muted-foreground">
                    {e.detail}
                  </p>
                </div>
              ))}
            </CardContent>
          </Card>
        </div>
      )}
    </>
  );
}
