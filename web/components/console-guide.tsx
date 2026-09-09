'use client';

import { ArrowRight } from 'lucide-react';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from '@/components/ui/dialog';

export type ConsoleView =
  | 'dashboard'
  | 'reports'
  | 'campaigns'
  | 'creatives'
  | 'profiles'
  | 'decision'
  | 'simulation'
  | 'operations'
  | 'agent';

const featureNotes: { view: ConsoleView; title: string; detail: string }[] = [
  {
    view: 'reports',
    title: '投放效果报表',
    detail:
      '按小时或天查看曝光、点击、转化与消耗，筛选日期和计划，导出报表或交给 Agent 分析。',
  },
  {
    view: 'campaigns',
    title: '广告计划',
    detail:
      '决定投在哪、投多久、给谁看，以及预算、出价和频控。发布后规则才会参与投放。',
  },
  {
    view: 'creatives',
    title: '素材管理',
    detail: '给计划添加图片、标题和跳转链接。计划需要至少一个启用的素材。',
  },
  {
    view: 'profiles',
    title: '用户画像',
    detail: '准备模拟观众的标签、设备和活跃分数。这些用户不是后台登录账号。',
  },
  {
    view: 'decision',
    title: '单次投放测试',
    detail:
      '测试一个用户会拿到哪条广告，查看成交价，并手动回传曝光、点击和转化。',
  },
  {
    view: 'simulation',
    title: '批量投放测试',
    detail:
      '让一批用户自动重复投放流程，查看命中、超时和回传结果。先用少量轮次理解流程。',
  },
  {
    view: 'dashboard',
    title: '投放总览',
    detail:
      '查看已经计入统计的曝光、点击和转化价值。异步回传后统计可能稍晚更新。',
  },
  {
    view: 'operations',
    title: '事件处理',
    detail: '统计迟迟不更新时，在这里检查等待结算、待发布、死信或待核对记录。',
  },
  {
    view: 'agent',
    title: 'Agent 规则助手',
    detail:
      '用自然语言生成规则草稿，由你检查并确认发布。它不负责实际广告竞价。',
  },
];

export function ConsoleHelp({
  open,
  onOpenChange,
  onNavigate,
  allowedViews,
  onPrepareDemo,
  demoDisabled,
}: {
  open: boolean;
  onOpenChange: (value: boolean) => void;
  onNavigate: (view: ConsoleView) => void;
  allowedViews: ConsoleView[];
  onPrepareDemo: () => void;
  demoDisabled: boolean;
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[85dvh] overflow-y-auto sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>这些功能怎样一起使用？</DialogTitle>
          <DialogDescription>
            配置广告 → 选择用户 → 决策与竞价 → 回传事件 → 查看统计
          </DialogDescription>
        </DialogHeader>
        {allowedViews.includes('simulation') && (
          <div className="flex flex-wrap items-center gap-3 rounded-lg bg-muted/60 p-3">
            <p className="min-w-0 flex-1 text-sm leading-6">
              第一次使用，可以先载入三轮竞价演示，手动开始后查看成交价与统计。
            </p>
            <Button size="sm" disabled={demoDisabled} onClick={onPrepareDemo}>
              准备竞价演示 <ArrowRight />
            </Button>
          </div>
        )}
        <div className="divide-y">
          {featureNotes.map((item) => (
            <div key={item.view} className="flex items-start gap-4 py-3">
              <div className="min-w-0 flex-1">
                <h3 className="text-sm font-semibold">{item.title}</h3>
                <p className="mt-1 text-sm leading-6 text-muted-foreground">
                  {item.detail}
                </p>
              </div>
              {allowedViews.includes(item.view) && (
                <Button
                  variant="ghost"
                  size="sm"
                  aria-label={`前往${item.title}`}
                  onClick={() => {
                    onOpenChange(false);
                    onNavigate(item.view);
                  }}
                >
                  <ArrowRight className="size-4" />
                </Button>
              )}
            </div>
          ))}
        </div>
        <div className="rounded-lg bg-muted/60 p-4 text-sm leading-6">
          <p>
            <strong>命中 ≠ 曝光：</strong>选出了广告，还需要回传展示事件。
          </p>
          <p>
            <strong>已受理 ≠ 已计入统计：</strong>
            异步模式需要完成结算和事件消费。
          </p>
          <p>
            <strong>未命中 ≠ 请求失败：</strong>
            定向不符、达到频控或预算不足都是可能的业务结果。
          </p>
          <p>
            <strong>转化价值 ≠ 广告花费：</strong>模拟转化
            ¥5，曝光仍按本次成交价计费。
          </p>
        </div>
      </DialogContent>
    </Dialog>
  );
}
