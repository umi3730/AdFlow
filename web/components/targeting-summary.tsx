import type { Campaign } from '@/lib/api';
import { describeCondition } from '@/lib/rule-presentation';

type Targeting = NonNullable<Campaign['activeVersion']>['targeting'];
const sections = [
  {
    key: 'all',
    title: '必须全部满足',
    empty: '未设置必须条件',
    help: '以下条件需要同时满足',
  },
  {
    key: 'any',
    title: '至少满足一条',
    empty: '不增加可选条件限制',
    help: '以下条件满足任意一条即可',
  },
  {
    key: 'none',
    title: '排除人群',
    empty: '未设置排除条件',
    help: '符合任意一条就不投放',
  },
] as const;

export function TargetingSummary({
  targeting,
  compact = false,
}: {
  targeting: Targeting;
  compact?: boolean;
}) {
  return (
    <div className="space-y-3" aria-label="人群规则摘要">
      {sections
        .filter(
          (section) => !compact || (targeting[section.key]?.length ?? 0) > 0,
        )
        .map((section) => (
          <section
            key={section.key}
            className="rounded-xl border bg-background p-4"
          >
            <div className="flex flex-wrap items-baseline justify-between gap-1">
              <h3 className="text-sm font-semibold">{section.title}</h3>
              {!compact && (
                <p className="text-xs text-muted-foreground">{section.help}</p>
              )}
            </div>
            {(targeting[section.key]?.length ?? 0) > 0 ? (
              <ul className="mt-3 flex flex-wrap gap-2">
                {targeting[section.key]!.map((condition, index) => (
                  <li
                    key={index}
                    className={`max-w-full break-words rounded-lg px-3 py-2 text-sm ${section.key === 'none' ? 'bg-rose-50 text-rose-800' : 'bg-primary/7 text-foreground'}`}
                  >
                    {describeCondition(condition)}
                  </li>
                ))}
              </ul>
            ) : (
              <p className="mt-2 text-sm text-muted-foreground">
                {section.empty}
              </p>
            )}
          </section>
        ))}
    </div>
  );
}
