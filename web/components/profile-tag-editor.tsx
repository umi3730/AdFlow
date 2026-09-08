'use client';
import { useState } from 'react';
import { X } from 'lucide-react';
import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
import {
  addProfileTags,
  profileTags,
  profileTagLabel,
  toggleProfileTag,
} from '@/lib/profile-options';

export function ProfileTagEditor({
  value,
  onChange,
  disabled,
}: {
  value: string;
  onChange: (value: string) => void;
  disabled?: boolean;
}) {
  const [draft, setDraft] = useState('');
  const selected = [
    ...new Set(
      value
        .split(/[,，]/)
        .map((tag) => tag.trim())
        .filter(Boolean),
    ),
  ];
  function add() {
    if (disabled || !draft.trim()) return;
    onChange(addProfileTags(value, draft));
    setDraft('');
  }
  return (
    <fieldset disabled={disabled} className="space-y-2">
      <legend className="mb-2 text-sm">用户标签</legend>
      <div className="flex min-h-9 flex-wrap gap-1.5 rounded-md border border-input p-2">
        {selected.length ? (
          selected.map((tag) => (
            <Button
              key={tag}
              type="button"
              size="sm"
              variant="secondary"
              disabled={disabled}
              aria-label={'移除' + profileTagLabel(tag)}
              onClick={() => onChange(toggleProfileTag(value, tag))}
            >
              {profileTagLabel(tag)}
              <X className="size-3" />
            </Button>
          ))
        ) : (
          <span className="text-sm text-muted-foreground">未添加标签</span>
        )}
      </div>
      <div className="flex gap-2">
        <Input
          id="profile-tags"
          aria-label="添加用户标签"
          placeholder="输入标签，回车添加"
          value={draft}
          disabled={disabled}
          onChange={(event) => setDraft(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === 'Enter' && !event.nativeEvent.isComposing) {
              event.preventDefault();
              add();
            }
          }}
        />
        <Button
          type="button"
          variant="outline"
          disabled={disabled || !draft.trim()}
          onClick={add}
        >
          添加
        </Button>
      </div>
      <div className="flex flex-wrap gap-1.5" aria-label="常用标签">
        {profileTags.map((tag) => (
          <Button
            key={tag.id}
            type="button"
            size="sm"
            disabled={disabled}
            variant={selected.includes(tag.id) ? 'default' : 'outline'}
            aria-pressed={selected.includes(tag.id)}
            onClick={() => onChange(toggleProfileTag(value, tag.id))}
          >
            {tag.label}
          </Button>
        ))}
      </div>
    </fieldset>
  );
}
