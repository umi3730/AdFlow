'use client';

import { useRef, useState } from 'react';
import {
  AlertTriangle,
  ArrowRight,
  LoaderCircle,
  Sparkles,
} from 'lucide-react';
import { api, type RuleDraft } from '@/lib/api';
import { prepareAgentPlan, type RuleEditorValue } from '@/lib/campaign-rules';
import {
  draftSource,
  editorFromAgentDraft,
  warningLabel,
} from '@/lib/rule-presentation';
import { TargetingSummary } from '@/components/targeting-summary';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { PageHeading } from '@/components/page-heading';
import { Input } from '@/components/ui/input';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';

export function AgentWorkspace({
  onCreatePlan,
}: {
  onCreatePlan: (value: RuleEditorValue) => void;
}) {
  const [prompt, setPrompt] = useState(
    '面向对数码或游戏感兴趣的活跃安卓用户，年龄18到35岁，每人每天最多展示3次。',
  );
  const [draft, setDraft] = useState<RuleDraft | null>(null);
  const [value, setValue] = useState<RuleEditorValue | null>(null);
  const [busy, setBusy] = useState(false);
  const inFlight = useRef(false);
  const [error, setError] = useState('');
  const hasPlatform =
    value &&
    Object.values(value.targeting).some((rows) =>
      rows.some((row) => row.field === 'platform'),
    );

  async function generate(event: { preventDefault(): void }) {
    event.preventDefault();
    if (inFlight.current) return;
    inFlight.current = true;
    setBusy(true);
    setDraft(null);
    setValue(null);
    setError('');
    try {
      const result = await api.generateRuleDraft(prompt.trim());
      setDraft(result);
      setValue(editorFromAgentDraft(result));
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : '生成失败，请重试');
    } finally {
      inFlight.current = false;
      setBusy(false);
    }
  }

  function handoff(event: { preventDefault(): void }) {
    event.preventDefault();
    if (!value || busy) return;
    try {
      onCreatePlan(prepareAgentPlan(value));
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : '请检查费用设置');
    }
  }

  function changeCost(
    key: 'dailyBudgetYuan' | 'impressionCostYuan' | 'frequencyLimit',
    next: string,
  ) {
    if (!value) return;
    setValue({ ...value, [key]: next });
    setError('');
  }

  return (
    <>
      <PageHeading
        title="Agent 规则助手"
        description="描述投放需求，生成规则后确认预算与人群条件。"
      />
      <div className="mt-5 grid items-start gap-5 xl:grid-cols-[minmax(280px,360px)_minmax(0,1fr)]">
        <Card className="min-w-0 bg-card text-card-foreground">
          <CardHeader>
            <CardTitle>投放需求</CardTitle>
            <CardDescription>描述人群、排除条件和投放限制。</CardDescription>
          </CardHeader>
          <CardContent>
            <form className="space-y-4" onSubmit={generate}>
              <textarea
                value={prompt}
                disabled={busy}
                maxLength={2000}
                minLength={5}
                required
                rows={7}
                onChange={(event) => {
                  setPrompt(event.target.value);
                  setDraft(null);
                  setValue(null);
                  setError('');
                }}
                className="w-full resize-y rounded-lg border border-input bg-card p-3 text-sm leading-6 text-foreground outline-none focus:border-ring disabled:opacity-60"
                aria-label="规则描述"
              />
              <Button
                type="submit"
                disabled={busy || prompt.trim().length < 5}
                className="w-full"
              >
                {busy ? (
                  <LoaderCircle className="animate-spin" />
                ) : (
                  <Sparkles />
                )}
                {busy ? '正在生成…' : '生成规则'}
              </Button>
            </form>
          </CardContent>
        </Card>
        <Card className="min-w-0">
          <CardHeader>
            <div className="flex flex-wrap items-center justify-between gap-2">
              <CardTitle>投放规则</CardTitle>
              {draft && <Badge variant="secondary">{draftSource(draft)}</Badge>}
            </div>
          </CardHeader>
          <CardContent className="space-y-4">
            {error && (
              <p
                role="alert"
                className="rounded-lg bg-rose-50 p-3 text-sm text-rose-800"
              >
                {error}
              </p>
            )}
            {!draft || !value ? (
              <output className="grid min-h-64 place-content-center gap-3 text-center text-sm text-muted-foreground">
                {busy ? (
                  <LoaderCircle className="mx-auto size-7 animate-spin" />
                ) : (
                  <Sparkles className="mx-auto size-7 text-primary/50" />
                )}
                <span>
                  {busy ? '正在生成并校验规则…' : '生成后在这里查看和调整'}
                </span>
              </output>
            ) : (
              <>
                {(draft.fallback || draft.provider === 'local-mock') && (
                  <p
                    role="alert"
                    className="rounded-lg bg-amber-50 p-3 text-sm text-amber-900"
                  >
                    {draft.fallback
                      ? '模型不可用，本次已降级为本地解析。请核对规则。'
                      : '本次为本地解析，请核对规则。'}
                  </p>
                )}
                <TargetingSummary targeting={value.targeting} compact />
                <form onSubmit={handoff} className="space-y-4">
                  <div className="grid gap-3 sm:grid-cols-3">
                    <label
                      className="border-l-2 border-primary/20 pl-3 text-sm"
                      htmlFor="agent-budget"
                    >
                      日预算（元）
                      <Input
                        id="agent-budget"
                        className="mt-2 bg-background font-semibold"
                        inputMode="decimal"
                        value={value.dailyBudgetYuan}
                        required
                        onChange={(event) =>
                          changeCost('dailyBudgetYuan', event.target.value)
                        }
                      />
                    </label>
                    <label
                      className="border-l-2 border-primary/20 pl-3 text-sm"
                      htmlFor="agent-cost"
                    >
                      单次曝光成本（元）
                      <Input
                        id="agent-cost"
                        className="mt-2 bg-background font-semibold"
                        inputMode="decimal"
                        value={value.impressionCostYuan}
                        required
                        onChange={(event) =>
                          changeCost('impressionCostYuan', event.target.value)
                        }
                      />
                    </label>
                    <label
                      className="border-l-2 border-primary/20 pl-3 text-sm"
                      htmlFor="agent-frequency"
                    >
                      每人每日曝光上限
                      <Input
                        id="agent-frequency"
                        className="mt-2 bg-background font-semibold"
                        type="number"
                        min={1}
                        max={100}
                        step={1}
                        value={value.frequencyLimit}
                        required
                        onChange={(event) =>
                          changeCost('frequencyLimit', event.target.value)
                        }
                      />
                    </label>
                  </div>
                  {hasPlatform && (
                    <p className="flex gap-2 rounded-lg bg-amber-50 p-3 text-sm text-amber-900">
                      <AlertTriangle className="size-4 shrink-0" />
                      规则使用 platform，画像默认使用 device，请在发布前核对。
                    </p>
                  )}
                  {draft.warnings.length > 0 && (
                    <ul className="space-y-1 rounded-lg bg-amber-50 p-3 text-sm text-amber-900">
                      {draft.warnings.map((warning, index) => (
                        <li key={index} className="break-words">
                          {warningLabel(warning)}
                        </li>
                      ))}
                    </ul>
                  )}
                  <div className="flex flex-wrap items-center justify-between gap-3 border-t pt-4">
                    <span className="text-xs text-muted-foreground">
                      带入广告计划页，仅新建，不直接发布。
                    </span>
                    <Button type="submit" disabled={busy}>
                      创建计划
                      <ArrowRight />
                    </Button>
                  </div>
                </form>
                <details className="text-sm text-muted-foreground">
                  <summary className="cursor-pointer">生成详情</summary>
                  <p className="mt-2 whitespace-pre-wrap break-words leading-6">
                    {draft.explanation}
                  </p>
                  <p className="mt-2 text-xs">{draft.promptVersion}</p>
                </details>
              </>
            )}
          </CardContent>
        </Card>
      </div>
    </>
  );
}
