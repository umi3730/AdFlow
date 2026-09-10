'use client';

import { groupTargetingCandidates } from '@/lib/targeting-report';
import { profileDisplayName, isDemoProfile } from '@/lib/profile-presentation';
import { ProfileTags } from '@/components/profile-tags';

import { useCallback, useEffect, useRef, useState } from 'react';
import { LoaderCircle, Send } from 'lucide-react';
import {
  api,
  Campaign,
  Decision,
  Profile,
  TargetingExplanation,
  newClientID,
  type TraceTarget,
} from '@/lib/api';
import { DecisionPricing } from '@/components/decision-pricing';
import { FormSelect } from '@/components/form-select';
import { adSlotOptions, adSlotLabel } from '@/lib/ad-slots';
import {
  canRecordDecisionEvent,
  createDecisionEventController,
  type DecisionEventType,
} from '@/lib/decision-events';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import {
  profileTagLabel,
  profileFields,
  fieldValueOptions,
} from '@/lib/profile-options';
import { PageHeading } from '@/components/page-heading';
import { StatusBadge, Field, Empty, Result } from './shared';

export function DecisionView({
  active,
  busy,
  run,
  userID,
  slotID,
  onUserIDChange: setUserID,
  onSlotIDChange: setSlotID,
  campaigns,
  onInspectRequest,
  targetCampaign,
  onAddMaterials,
  onEventsAccepted,
  onReport,
}: {
  active: boolean;
  busy: boolean;
  run: (action: () => Promise<void>, message: string) => Promise<void>;
  userID: string;
  slotID: string;
  onUserIDChange: (id: string) => void;
  onSlotIDChange: (id: string) => void;
  campaigns: Campaign[];
  onInspectRequest: (target: TraceTarget) => void;
  targetCampaign: Campaign | null;
  onAddMaterials: (campaign: Campaign) => void;
  onEventsAccepted: () => Promise<void>;
  onReport: (campaignId: string) => void;
}) {
  const [decision, setDecision] = useState<Decision | null>(null);
  const [eventController] = useState(() =>
    createDecisionEventController(newClientID),
  );
  const [eventState, setEventState] = useState(() =>
    eventController.snapshot(),
  );
  const deciding = useRef(false);
  const decisionRevision = useRef(0);
  const [profiles, setProfiles] = useState<Profile[]>([]);
  const [profile, setProfile] = useState<Profile | null>(null);
  const [profileError, setProfileError] = useState('');
  const [explanation, setExplanation] = useState<TargetingExplanation | null>(
    null,
  );
  const [explanationError, setExplanationError] = useState('');
  const [decisionNow, setDecisionNow] = useState(() => Date.now());
  useEffect(() => {
    if (
      !decision?.matched ||
      !decision.expiresAt ||
      eventState.impression === 'recorded'
    )
      return;
    const timer = setTimeout(
      () => setDecisionNow(Date.now()),
      Math.max(0, Date.parse(decision.expiresAt) - Date.now() + 5),
    );
    return () => clearTimeout(timer);
  }, [decision, eventState.impression]);
  useEffect(
    () => () => {
      decisionRevision.current++;
      eventController.select(null);
    },
    [eventController],
  );
  useEffect(() => {
    if (!active) return;
    let live = true;
    void api
      .listProfiles({ limit: 100 })
      .then((page) => {
        if (live) setProfiles(page.items);
      })
      .catch((cause) => {
        if (live)
          setProfileError(
            cause instanceof Error ? cause.message : '用户列表读取失败',
          );
      });
    return () => {
      live = false;
    };
  }, [active]);
  useEffect(() => {
    if (!active) return;
    let live = true;
    const timer = setTimeout(() => {
      if (!userID.trim()) return;
      void api
        .getProfile(userID.trim())
        .then((result) => {
          if (live) {
            setProfile(result);
            setProfileError('');
          }
        })
        .catch((cause) => {
          if (live) {
            setProfile(null);
            setProfileError(
              cause instanceof Error ? cause.message : '画像读取失败',
            );
          }
        });
    }, 200);
    return () => {
      live = false;
      clearTimeout(timer);
    };
  }, [userID, active]);

  const clearDecision = useCallback(() => {
    decisionRevision.current++;
    eventController.select(null);
    setEventState(eventController.snapshot());
    setDecision(null);
    setExplanation(null);
    setExplanationError('');
  }, [eventController]);
  const chooseUser = useCallback(
    (id: string) => {
      setUserID(id);
      setProfile(null);
      setProfileError('');
      clearDecision();
    },
    [clearDecision, setUserID],
  );
  async function decide(event: { preventDefault(): void }) {
    event.preventDefault();
    if (busy || deciding.current || eventController.isPending()) return;
    deciding.current = true;
    clearDecision();
    const revision = decisionRevision.current;
    try {
      await run(async () => {
        const result = await api.decide({
          requestId: newClientID('req'),
          userId: userID.trim(),
          slotId: slotID.trim(),
        });
        if (revision !== decisionRevision.current) return;
        setDecision(result);
        setDecisionNow(Date.now());
        eventController.select(result);
        setEventState(eventController.snapshot());
        try {
          const report = await api.explainDecision({
            userId: userID.trim(),
            slotId: slotID.trim(),
          });
          if (revision !== decisionRevision.current) return;
          setExplanation(report);
          setProfile(report.profile);
        } catch (cause) {
          if (revision === decisionRevision.current)
            setExplanationError(
              cause instanceof Error ? cause.message : '定向检查暂不可用',
            );
        }
      }, '决策已完成');
    } finally {
      deciding.current = false;
    }
  }
  async function event(type: DecisionEventType) {
    if (busy || deciding.current) return;
    if (
      type === 'impression' &&
      decision?.expiresAt &&
      Date.now() >= Date.parse(decision.expiresAt)
    )
      return;
    const submission = eventController.begin(type);
    if (!submission) return;
    setEventState(eventController.snapshot());
    await run(
      async () => {
        try {
          await api.recordEvent(submission);
          void onEventsAccepted();
          if (eventController.finish(submission, true))
            setEventState(eventController.snapshot());
        } catch (cause) {
          if (eventController.finish(submission, false))
            setEventState(eventController.snapshot());
          throw cause;
        }
      },
      `${{ impression: '曝光', click: '点击', conversion: '转化' }[type]}事件已受理`,
    );
  }
  return (
    <>
      <PageHeading
        title="单次投放测试"
        description="选择一个用户，查看广告选择结果，再回传曝光、点击或转化。"
      />
      <div className="mt-5 grid items-start gap-5 xl:grid-cols-[minmax(280px,360px)_minmax(0,1fr)]">
        <Card className="bg-card text-card-foreground">
          <CardHeader>
            <CardTitle>发起决策</CardTitle>
            {targetCampaign && (
              <CardDescription>
                测试广告位：{adSlotLabel(targetCampaign.slotId)}
                。该广告位的其他计划也会参与选择。
              </CardDescription>
            )}
          </CardHeader>
          <CardContent>
            <form className="space-y-4" onSubmit={decide}>
              <Field label="选择已保存用户" dark>
                <select
                  aria-label="选择已保存用户"
                  className="h-9 w-full rounded-lg border border-input bg-card px-2 text-sm text-foreground"
                  value={
                    profiles.some((item) => item.userId === userID)
                      ? userID
                      : ''
                  }
                  disabled={busy}
                  onChange={(event) => chooseUser(event.target.value)}
                >
                  <option value="">选择用户（或在下方输入 ID）</option>
                  {profiles.map((item) => (
                    <option key={item.userId} value={item.userId}>
                      {profileDisplayName(item.userId)}
                    </option>
                  ))}
                </select>
                <p className="mt-1 text-xs text-muted-foreground">
                  快捷列表最多显示 100 人；更多用户可在画像表格中筛选。
                  演示名称表示用途，是否投放以当前规则为准。
                </p>
              </Field>
              <Field label="用户 ID" dark>
                <Input
                  className="bg-card text-foreground"
                  value={userID}
                  onChange={(e) => chooseUser(e.target.value)}
                  required
                  disabled={busy}
                />
              </Field>
              <Field label="广告位" dark>
                <FormSelect
                  label="广告位"
                  className="bg-card text-foreground"
                  value={slotID}
                  options={adSlotOptions}
                  onChange={(value) => {
                    setSlotID(value);
                    clearDecision();
                  }}
                  disabled={busy}
                />
              </Field>
              {profileError && (
                <p
                  role="alert"
                  className="rounded-lg bg-amber-50 p-3 text-sm text-amber-800"
                >
                  {profileError}
                </p>
              )}
              {profile && profile.userId === userID.trim() && (
                <div className="space-y-2 rounded-lg border bg-card p-3">
                  <p className="text-sm font-medium">
                    已保存画像 · {profileDisplayName(profile.userId)}
                  </p>
                  {isDemoProfile(profile.userId) && (
                    <p className="break-all font-mono text-xs text-muted-foreground">
                      {profile.userId}
                    </p>
                  )}
                  <ProfileTags tags={profile.tags} />
                  <dl className="space-y-1 text-sm">
                    {Object.entries(profile.fields).map(([key, value]) => (
                      <div
                        key={key}
                        className="flex flex-wrap justify-between gap-2"
                      >
                        <dt className="text-muted-foreground">
                          {profileFields.find((field) => field.id === key)
                            ?.label ?? key}
                        </dt>
                        <dd className="break-all">
                          {fieldValueOptions(key)?.find(
                            (option) => option.value === value,
                          )?.label ?? value}
                        </dd>
                      </div>
                    ))}
                  </dl>
                </div>
              )}
              <Button
                type="submit"
                className="w-full"
                disabled={busy || !userID.trim() || !slotID.trim()}
              >
                {busy ? <LoaderCircle className="animate-spin" /> : <Send />}
                {busy ? '处理中…' : '运行决策'}
              </Button>
            </form>
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle>决策结果</CardTitle>
            <CardDescription>
              曝光需在决策有效期内回传；曝光受理后，点击和转化可在 7
              天内回传。异步统计稍后更新。
            </CardDescription>
          </CardHeader>
          <CardContent>
            {!decision ? (
              <Empty text="填写参数并运行一次决策" />
            ) : (
              <div className="space-y-4">
                <div className="flex items-center gap-3">
                  <StatusBadge
                    status={decision.matched ? 'MATCHED' : 'NO-AD'}
                  />
                  <span className="text-sm text-muted-foreground">
                    {decisionReasonLabel(decision.reason)}
                  </span>
                </div>
                {decision.matched && (
                  <DecisionPricing pricing={decision.pricing} />
                )}
                <dl className="grid gap-3 text-sm md:grid-cols-2">
                  <div>
                    <Result label="请求编号" value={decision.requestId} />
                    <Button
                      type="button"
                      size="sm"
                      variant="outline"
                      className="mt-2"
                      onClick={() =>
                        onInspectRequest({ requestId: decision.requestId })
                      }
                    >
                      查看处理过程
                    </Button>
                  </div>
                  <Result
                    label={decision.matched ? '本次成交计划' : '广告计划'}
                    value={
                      campaigns.find((item) => item.id === decision.campaignId)
                        ?.name ||
                      decision.campaignId ||
                      '—'
                    }
                  />
                  <Result label="素材编号" value={decision.creativeId || '—'} />
                  <Result
                    label="曝光回传截止"
                    value={
                      decision.expiresAt
                        ? new Date(decision.expiresAt).toLocaleTimeString()
                        : '—'
                    }
                  />
                </dl>
                {explanationError && (
                  <p className="rounded-lg bg-amber-50 p-3 text-sm text-amber-800">
                    定向检查：{explanationError}
                  </p>
                )}
                {explanation && (
                  <TargetingReport
                    report={explanation}
                    campaigns={campaigns}
                    decision={decision}
                  />
                )}
                {decision.reason === 'no_creative' &&
                  explanation?.candidates
                    .filter((item) => item.targetingMatched)
                    .map((item) => {
                      const campaign = campaigns.find(
                        (campaign) => campaign.id === item.campaignId,
                      );
                      return campaign ? (
                        <Button
                          key={campaign.id}
                          variant="outline"
                          onClick={() => onAddMaterials(campaign)}
                        >
                          给「{campaign.name}」添加素材
                        </Button>
                      ) : null;
                    })}
                {decision.matched && (
                  <Button
                    variant="outline"
                    onClick={() => onReport(decision.campaignId)}
                  >
                    查看此计划报表
                  </Button>
                )}
                {decision.matched && (
                  <div className="flex flex-wrap gap-2 border-t pt-4">
                    <Button
                      size="sm"
                      disabled={
                        busy ||
                        (!!decision.expiresAt &&
                          decisionNow >= Date.parse(decision.expiresAt)) ||
                        !canRecordDecisionEvent(eventState, 'impression')
                      }
                      onClick={() => void event('impression')}
                    >
                      {eventState.impression === 'recorded'
                        ? '曝光已受理'
                        : eventState.impression === 'pending'
                          ? '曝光记录中…'
                          : decision.expiresAt &&
                              decisionNow >= Date.parse(decision.expiresAt)
                            ? '已过期，请重新决策'
                            : '记录曝光'}
                    </Button>
                    <Button
                      size="sm"
                      variant="outline"
                      disabled={
                        busy || !canRecordDecisionEvent(eventState, 'click')
                      }
                      title={
                        eventState.impression !== 'recorded'
                          ? '请先记录曝光'
                          : undefined
                      }
                      onClick={() => void event('click')}
                    >
                      {eventState.click === 'recorded'
                        ? '点击已受理'
                        : eventState.click === 'pending'
                          ? '点击记录中…'
                          : '记录点击'}
                    </Button>
                    <Button
                      size="sm"
                      variant="outline"
                      disabled={
                        busy ||
                        !canRecordDecisionEvent(eventState, 'conversion')
                      }
                      title={
                        eventState.impression !== 'recorded'
                          ? '请先记录曝光'
                          : undefined
                      }
                      onClick={() => void event('conversion')}
                    >
                      {eventState.conversion === 'recorded'
                        ? '转化已受理 ¥5'
                        : eventState.conversion === 'pending'
                          ? '转化记录中…'
                          : '记录转化 ¥5'}
                    </Button>
                  </div>
                )}
              </div>
            )}
          </CardContent>
        </Card>
      </div>
    </>
  );
}

export function decisionReasonLabel(reason: string) {
  const labels: Record<string, string> = {
    matched: '已命中广告',
    profile_not_found: '用户画像不存在',
    no_candidate: '该广告位暂无有效投放计划',
    targeting_miss: '用户未满足定向条件',
    no_creative: '定向已满足，但计划缺少可用素材',
    frequency_capped: '该用户已达到每日曝光上限',
    budget_exhausted: '预算不足或已被预占',
    dependency_unavailable: '依赖服务暂不可用',
  };
  return labels[reason] ?? reason;
}

export function TargetingReport({
  report,
  campaigns,
  decision,
}: {
  report: TargetingExplanation;
  campaigns: Campaign[];
  decision: Decision;
}) {
  const { winner, others } = groupTargetingCandidates(report, decision);
  const winningName =
    campaigns.find((item) => item.id === decision.campaignId)?.name ??
    decision.campaignId;
  const groupLabels = {
    all: '必须全部满足',
    any: '至少满足一条（当前均不满足）',
    none: '命中排除条件',
  };
  function renderCandidate(
    candidate: TargetingExplanation['candidates'][number],
    isWinner = false,
  ) {
    return (
      <div
        key={candidate.campaignId}
        className={
          isWinner
            ? 'space-y-2 rounded-lg border border-primary/30 bg-primary/5 p-3'
            : 'space-y-2 rounded-lg border p-3'
        }
      >
        <div className="flex flex-wrap items-center justify-between gap-2">
          <p className="font-medium">
            {isWinner && (
              <span className="mb-1 block text-xs font-semibold text-primary">
                本次成交
              </span>
            )}
            {campaigns.find((item) => item.id === candidate.campaignId)?.name ??
              candidate.campaignId}
          </p>
          <Badge
            variant="outline"
            className={
              candidate.targetingMatched ? 'text-emerald-700' : 'text-rose-700'
            }
          >
            {candidate.targetingMatched ? '定向条件满足' : '定向条件不满足'}
          </Badge>
        </div>
        {isWinner &&
          (!candidate.targetingMatched || !candidate.hasCreative) && (
            <p className="rounded-md bg-amber-50 p-3 text-sm text-amber-900">
              当前检查与已保存的成交结果不同。规则、画像或素材可能已变化，本次成交仍以历史记录为准。
            </p>
          )}
        {!candidate.hasCreative && (
          <p className="text-sm text-amber-700">
            该计划当前没有有效素材，即使定向满足也不能投放。
          </p>
        )}
        {candidate.failures.map((failure, index) => {
          const condition = failure.condition;
          let message = '';
          if (failure.code === 'invalid_condition')
            message = `规则字段 ${condition.field} 的比较方式或值不合法，请修正规则后重新发布。`;
          else if (failure.code === 'missing_tag')
            message = `缺少标签：${profileTagLabel(condition.tag ?? '')}`;
          else if (failure.code === 'missing_field')
            message = `缺少字段：${condition.field}；规则要求 ${condition.op} ${condition.value}`;
          else if (failure.code === 'excluded_condition')
            message = condition.tag
              ? `用户带有排除标签：${profileTagLabel(condition.tag)}`
              : `命中排除字段：${condition.field} ${condition.op} ${condition.value}`;
          else
            message = `${condition.field} 的实际值是「${failure.actual ?? ''}」，规则要求 ${condition.op}「${condition.value}」`;
          return (
            <div
              key={index}
              className="rounded-lg bg-rose-50 p-3 text-sm text-rose-900"
              title={condition.tag ? '标签标识：' + condition.tag : undefined}
            >
              <p className="mb-1 text-xs font-semibold">
                {groupLabels[failure.group]}
              </p>
              <p className="break-words">{message}</p>
              {condition.field === 'platform' &&
                report.profile.fields.device !== undefined && (
                  <p className="mt-1">
                    画像已有 device={report.profile.fields.device}，但 platform
                    是不同字段；请在规则编辑器中确认并转换。
                  </p>
                )}
            </div>
          );
        })}
        {!isWinner && candidate.targetingMatched && candidate.hasCreative && (
          <p className="text-sm text-muted-foreground">
            标签和字段检查通过；最终是否投放仍受频控、预算和素材选择影响。
          </p>
        )}
      </div>
    );
  }
  return (
    <section className="space-y-3 border-t pt-4" aria-label="当前定向检查">
      <h3 className="font-semibold">当前定向检查</h3>
      <p className="text-sm text-muted-foreground">
        用户 {report.profile.userId} ·{' '}
        {new Date(report.checkedAt).toLocaleTimeString('zh-CN')}
        ，按当前配置检查。本次成交以已保存的结果为准。
      </p>
      {decision.matched ? (
        <>
          <section className="space-y-2" aria-label="本次成交计划的定向检查">
            {winner ? (
              renderCandidate(winner, true)
            ) : (
              <div className="space-y-2 rounded-lg border border-primary/30 bg-primary/5 p-3">
                <p className="text-xs font-semibold text-primary">本次成交</p>
                <p className="font-medium">{winningName}</p>
                <p className="text-sm text-muted-foreground">
                  当前候选列表中已没有该计划，无法复查当前定向；这不会改变已保存的成交结果。
                </p>
              </div>
            )}
          </section>
          {others.length > 0 && (
            <details className="rounded-lg border" key={decision.requestId}>
              <summary className="cursor-pointer rounded-lg px-3 py-3 text-sm font-medium focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring">
                查看其他计划（{others.length}）
              </summary>
              <div className="space-y-3 border-t p-3">
                <p className="text-sm text-muted-foreground">
                  以下计划均未在本次成交。定向按计划分别判断，其他计划不满足条件，不影响「
                  {winningName}」的成交结果。
                </p>
                {others.map((candidate) => renderCandidate(candidate))}
              </div>
            </details>
          )}
        </>
      ) : (
        <>
          <p className="text-sm text-muted-foreground">
            本次未成交，以下逐项列出各计划当前的检查结果。
          </p>
          {report.candidates.length === 0 && (
            <p className="rounded-lg bg-muted/50 p-3 text-sm">
              该广告位当前没有有效候选计划。检查广告位是否一致、计划是否发布、是否在投放周期内。
            </p>
          )}
          {others.map((candidate) => renderCandidate(candidate))}
        </>
      )}
    </section>
  );
}
