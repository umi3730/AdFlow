import type { Decision } from '@/lib/api';

export function DecisionPricing({ pricing }: { pricing: Decision['pricing'] }) {
  if (!pricing?.mode) return null;
  return (
    <section
      aria-label="本次成交信息"
      className="space-y-2 border-y py-3 text-sm"
    >
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h3 className="font-medium">
          {pricing.mode === 'first_price' ? '一价竞价胜出' : '固定成本投放'}
        </h3>
        <span className="font-semibold text-primary">
          单次成交 ¥{(pricing.priceFen / 100).toFixed(2)}
        </span>
      </div>
      {pricing.mode === 'first_price' && (
        <>
          <p>
            广告主：{pricing.advertiserName}（{pricing.advertiserId}）
          </p>
          <p className="text-muted-foreground">
            {pricing.advertisers} 家广告主参与 · 第 {pricing.rank} 顺位成交 ·
            出价 ¥{((pricing.bidFen ?? 0) / 100).toFixed(2)}
          </p>
        </>
      )}
      {(pricing.budgetRejected > 0 || pricing.frequencyRejected > 0) && (
        <p className="text-muted-foreground">
          此前跳过：预算不足 {pricing.budgetRejected} 条，频控已满{' '}
          {pricing.frequencyRejected} 条。
        </p>
      )}
      <p className="text-xs text-muted-foreground">
        本次按计划 v{pricing.version}{' '}
        预占上述金额，曝光后确认；这是历史决策记录。
      </p>
    </section>
  );
}
