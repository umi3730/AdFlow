import { Badge } from '@/components/ui/badge';
import { profileTagInfo, type ProfileTagKind } from '@/lib/profile-options';

const colors: Record<ProfileTagKind, string> = {
  interest:
    'border-sky-200 bg-sky-50 text-sky-800 dark:border-sky-800 dark:bg-sky-950 dark:text-sky-200',
  behavior:
    'border-emerald-200 bg-emerald-50 text-emerald-800 dark:border-emerald-800 dark:bg-emerald-950 dark:text-emerald-200',
  demo: 'border-primary/20 bg-primary/5 text-primary',
  exclusion:
    'border-amber-200 bg-amber-50 text-amber-900 dark:border-amber-800 dark:bg-amber-950 dark:text-amber-200',
  custom: 'border-dashed bg-muted/40 text-muted-foreground',
};

export function ProfileTags({ tags }: { tags: readonly string[] }) {
  if (!tags.length)
    return <span className="text-sm text-muted-foreground">暂无标签</span>;
  return (
    <span className="flex flex-wrap gap-1.5" aria-label="用户标签">
      {[...new Set(tags)].map((id) => {
        const tag = profileTagInfo(id);
        return (
          <Badge
            key={id}
            variant="outline"
            className={
              'max-w-full whitespace-normal break-all px-2 py-0.5 text-sm ' +
              colors[tag.kind]
            }
            title={tag.description + '\n标签标识：' + id}
          >
            {tag.label}
          </Badge>
        );
      })}
    </span>
  );
}
