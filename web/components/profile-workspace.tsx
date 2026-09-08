'use client';
import { useAccess } from '@/components/auth-gate';

import { useCallback, useEffect, useRef, useState } from 'react';
import {
  LoaderCircle,
  Plus,
  RefreshCw,
  Search,
  UsersRound,
} from 'lucide-react';
import { api, ApiError, type Profile, type ProfilePage } from '@/lib/api';
import {
  nextTestUserNumber,
  profileSaveMode,
  testUserID,
} from '@/lib/profile-defaults';
import {
  profileTagLabel,
  profileTagID,
  profileDeviceLabel,
  deviceOptions,
  validateFieldCondition,
} from '@/lib/profile-options';
import { ProfileTagEditor } from '@/components/profile-tag-editor';
import { FormSelect } from '@/components/form-select';
import { ProfileFieldsEditor } from '@/components/profile-fields-editor';
import { DeleteResourceButton } from '@/components/delete-resource-button';
import { Button } from '@/components/ui/button';
import { PageHeading } from '@/components/page-heading';
import { Input } from '@/components/ui/input';
import { Badge } from '@/components/ui/badge';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';

const emptyPage: ProfilePage = { items: [], total: 0, limit: 20, offset: 0 };

export function ProfileWorkspace({
  onDecide,
}: {
  onDecide: (userID: string) => void;
}) {
  const { canOperate } = useAccess();
  const [page, setPage] = useState(emptyPage);
  const [filter, setFilter] = useState({
    q: '',
    tag: '',
    device: '',
    offset: 0,
  });
  const [q, setQ] = useState('');
  const [tag, setTag] = useState('');
  const [filterDevice, setFilterDevice] = useState('');
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState('');
  const sequence = useRef(0);
  const [selected, setSelected] = useState<Profile | null>(null);
  const [userIDOverride, setUserID] = useState<string | null>(null);
  const [numberingProfiles, setNumberingProfiles] = useState<
    Pick<Profile, 'userId'>[]
  >([]);
  const userSequence = nextTestUserNumber(numberingProfiles);
  const userID = userIDOverride ?? selected?.userId ?? testUserID(userSequence);
  const saveMode = profileSaveMode(selected?.userId, userID);
  const savingRef = useRef(false);
  const [tags, setTags] = useState('tech_interest,gaming_interest,active_7d');
  const [device, setDevice] = useState('android');
  const [score, setScore] = useState('88');
  const [extraFields, setExtraFields] = useState<Record<string, string>>({});
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const [message, setMessage] = useState('');

  const load = useCallback(async () => {
    const revision = ++sequence.current;
    setLoading(true);
    setLoadError('');
    try {
      const [data, catalog] = await Promise.all([
        api.listProfiles({ ...filter, limit: 20 }),
        api.listProfileCatalog(),
      ]);
      if (revision === sequence.current) {
        setPage(data);
        setNumberingProfiles(catalog.map(({ userId }) => ({ userId })));
      }
    } catch (cause) {
      if (revision === sequence.current)
        setLoadError(cause instanceof Error ? cause.message : '读取画像失败');
    } finally {
      if (revision === sequence.current) setLoading(false);
    }
  }, [filter]);
  const cancelLoad = useCallback(() => {
    sequence.current++;
  }, []);
  useEffect(() => {
    void Promise.resolve().then(load);
    return cancelLoad;
  }, [load, cancelLoad]);

  function edit(profile: Profile) {
    setSelected(profile);
    setUserID(profile.userId);
    setTags(profile.tags.join(','));
    setDevice(profile.fields.device ?? '');
    setScore(profile.fields.score ?? '');
    setExtraFields(
      Object.fromEntries(
        Object.entries(profile.fields).filter(
          ([key]) => key !== 'device' && key !== 'score',
        ),
      ),
    );
    setError('');
    setMessage('');
  }
  function create() {
    setSelected(null);
    setUserID(null);
    setTags('tech_interest,gaming_interest,active_7d');
    setDevice('android');
    setScore('88');
    setExtraFields({});
    setError('');
    setMessage('');
  }
  async function save(event: { preventDefault(): void }) {
    event.preventDefault();
    if (!canOperate) return;
    if (loading || loadError) return;
    if (savingRef.current) return;
    savingRef.current = true;
    setSaving(true);
    setError('');
    setMessage('');
    try {
      const id = userID.trim();
      if (!id) throw new Error('请填写用户 ID');
      const extra = extraFields;
      if (
        !extra ||
        Array.isArray(extra) ||
        typeof extra !== 'object' ||
        !Object.entries(extra).every(
          ([key, value]) => key.trim() && typeof value === 'string',
        )
      )
        throw new Error('请检查画像字段名称和值');
      if ('device' in extra || 'score' in extra)
        throw new Error('device 和 score 请使用上方专用输入框');
      if (saveMode !== 'update') {
        let exists = false;
        try {
          await api.getProfile(id);
          exists = true;
        } catch (cause) {
          if (!(cause instanceof ApiError) || cause.status !== 404) throw cause;
        }
        if (exists)
          throw new Error(
            '此用户 ID 已存在，请先在列表中找到并编辑，避免覆盖原画像',
          );
      }
      const profile: Profile = {
        userId: id,
        tags: tags
          .split(/[,，]/)
          .map((value) => value.trim())
          .filter(Boolean),
        fields: { ...(extra as Record<string, string>) },
      };
      if (device.trim()) profile.fields.device = device.trim();
      if (score.trim()) profile.fields.score = score.trim();
      if (device.trim()) validateFieldCondition('device', 'eq', device.trim());
      if (score.trim()) validateFieldCondition('score', 'eq', score.trim());
      for (const field of ['age', 'member_level', 'channel']) {
        if (profile.fields[field] !== undefined)
          validateFieldCondition(field, 'eq', profile.fields[field]);
      }
      await api.putProfile(id, { tags: profile.tags, fields: profile.fields });
      setNumberingProfiles((previous) => [
        ...previous.filter((item) => item.userId !== id),
        { userId: id },
      ]);
      create();
      setMessage(`用户 ${id} 已保存，已切换到下一条新建画像。`);
      await load();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : '保存失败');
    } finally {
      savingRef.current = false;
      setSaving(false);
    }
  }

  return (
    <>
      <PageHeading
        title="用户画像"
        description="维护模拟用户属性，选择用户验证投放规则。"
        action={
          <Button
            type="button"
            variant="outline"
            disabled={saving}
            onClick={create}
          >
            <Plus />
            新建画像
          </Button>
        }
      />
      <div className="mt-5 grid gap-5 xl:grid-cols-[minmax(0,1fr)_340px]">
        <Card className="min-w-0 self-start">
          <CardHeader>
            <div className="flex items-center justify-between">
              <CardTitle>
                已保存用户 <Badge variant="secondary">{page.total}</Badge>
              </CardTitle>
              <Button
                type="button"
                variant="ghost"
                size="icon"
                disabled={loading}
                onClick={() => void load()}
                aria-label="刷新用户列表"
              >
                <RefreshCw className={loading ? 'animate-spin' : ''} />
              </Button>
            </div>
            <CardDescription>
              点击查看/编辑加载完整属性，发起决策自动带入 ID。
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <form
              onSubmit={(event) => {
                event.preventDefault();
                setFilter({
                  q: q.trim(),
                  tag: profileTagID(tag),
                  device: filterDevice.trim(),
                  offset: 0,
                });
              }}
              className="grid gap-2 sm:grid-cols-[1fr_1fr_1fr_auto]"
            >
              <Input
                aria-label="按用户 ID 搜索"
                placeholder="用户 ID 包含…"
                value={q}
                onChange={(event) => setQ(event.target.value)}
              />
              <Input
                aria-label="按标签筛选"
                placeholder="标签，例如 数码兴趣"
                value={tag}
                onChange={(event) => setTag(event.target.value)}
              />
              <FormSelect
                label="按设备筛选"
                value={filterDevice || 'all'}
                onChange={(value) =>
                  setFilterDevice(value === 'all' ? '' : value)
                }
                options={[
                  { value: 'all', label: '全部设备' },
                  ...deviceOptions,
                ]}
              />
              <Button type="submit" variant="outline" disabled={loading}>
                <Search />
                筛选
              </Button>
            </form>
            {loadError ? (
              <p
                role="alert"
                className="rounded-lg bg-destructive/10 p-3 text-sm text-destructive"
              >
                {loadError}
              </p>
            ) : loading ? (
              <p className="py-10 text-center text-sm text-muted-foreground">
                正在读取已保存画像…
              </p>
            ) : page.items.length === 0 ? (
              <div className="py-10 text-center text-muted-foreground">
                <UsersRound className="mx-auto mb-3 size-8" />
                <p>没有符合条件的已保存用户</p>
                <p className="mt-1 text-sm">先保存一个画像，或调整筛选条件。</p>
              </div>
            ) : (
              <Table className="min-w-[650px]">
                <TableHeader>
                  <TableRow>
                    <TableHead>用户 ID</TableHead>
                    <TableHead>标签</TableHead>
                    <TableHead>设备</TableHead>
                    <TableHead>分数</TableHead>
                    <TableHead className="sticky right-0 bg-card text-right">
                      操作
                    </TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {page.items.map((profile) => (
                    <TableRow
                      key={profile.userId}
                      className={
                        selected?.userId === profile.userId
                          ? 'bg-primary/5'
                          : ''
                      }
                    >
                      <TableCell className="font-mono text-xs">
                        {profile.userId}
                      </TableCell>
                      <TableCell>
                        <div className="flex max-w-64 flex-wrap gap-1">
                          {profile.tags.length
                            ? profile.tags.map((tag) => (
                                <Badge key={tag} variant="secondary">
                                  {profileTagLabel(tag)}
                                </Badge>
                              ))
                            : '—'}
                        </div>
                      </TableCell>
                      <TableCell>
                        {profileDeviceLabel(profile.fields.device || '—')}
                      </TableCell>
                      <TableCell>{profile.fields.score || '—'}</TableCell>
                      <TableCell className="sticky right-0 bg-card">
                        <div className="flex justify-end gap-1">
                          <Button
                            type="button"
                            variant="outline"
                            size="sm"
                            disabled={saving}
                            onClick={() => edit(profile)}
                          >
                            {canOperate ? '查看/编辑' : '查看'}
                          </Button>
                          <Button
                            type="button"
                            size="sm"
                            onClick={() => onDecide(profile.userId)}
                            disabled={!canOperate}
                          >
                            发起决策
                          </Button>
                          <DeleteResourceButton
                            name={profile.userId}
                            description="删除当前画像，历史决策不受影响。此操作不可撤销，之后可重新创建同名画像。"
                            disabled={saving}
                            onDelete={() => api.deleteProfile(profile.userId)}
                            onDeleted={async () => {
                              if (selected?.userId === profile.userId) create();
                              setFilter((previous) => ({
                                ...previous,
                                offset: 0,
                              }));
                              await load();
                            }}
                          />
                        </div>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            )}
            <div className="flex items-center justify-between gap-2 text-sm text-muted-foreground">
              <span>
                第 {Math.floor(filter.offset / 20) + 1} 页 · 共 {page.total} 人
              </span>
              <div className="flex gap-2">
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  disabled={loading || filter.offset === 0}
                  onClick={() =>
                    setFilter({
                      ...filter,
                      offset: Math.max(0, filter.offset - 20),
                    })
                  }
                >
                  上一页
                </Button>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  disabled={loading || filter.offset + 20 >= page.total}
                  onClick={() =>
                    setFilter({ ...filter, offset: filter.offset + 20 })
                  }
                >
                  下一页
                </Button>
              </div>
            </div>
          </CardContent>
        </Card>
        <Card className="min-w-0 self-start">
          <CardHeader>
            <CardTitle>
              {selected ? '编辑已保存画像' : '新建模拟画像'}
            </CardTitle>
            <CardDescription>
              {selected
                ? 'ID 区分大小写；修改 ID 将另存为新用户，原用户及历史记录保留。'
                : '下方是待保存内容。用户 ID 区分大小写，首尾空白会被去除。'}
            </CardDescription>
          </CardHeader>
          <CardContent>
            {error && (
              <p
                role="alert"
                className="mb-3 rounded-lg bg-destructive/10 p-3 text-sm text-destructive"
              >
                {error}
              </p>
            )}
            {message && (
              <output className="mb-3 block rounded-lg bg-emerald-50 p-3 text-sm text-emerald-800">
                {message}
              </output>
            )}
            {!canOperate && (
              <p className="mb-3 text-sm text-muted-foreground">
                只读账号，可查看画像；保存需要运营或管理员权限。
              </p>
            )}
            <form className="space-y-3" onSubmit={save}>
              <fieldset disabled={!canOperate} className="space-y-3">
                <label
                  htmlFor="profile-user-id"
                  className="block space-y-1 text-sm"
                >
                  用户 ID
                  <Input
                    id="profile-user-id"
                    disabled={saving}
                    value={userID}
                    required
                    maxLength={128}
                    placeholder="例如 user-10001"
                    onChange={(event) => setUserID(event.target.value)}
                  />
                </label>
                <ProfileTagEditor
                  key={selected?.userId ?? 'new'}
                  value={tags}
                  onChange={setTags}
                  disabled={saving || !canOperate}
                />
                <div className="grid grid-cols-2 gap-3">
                  <label
                    htmlFor="profile-device"
                    className="block space-y-1 text-sm"
                  >
                    设备
                    <FormSelect
                      id="profile-device"
                      label="设备"
                      value={device}
                      options={deviceOptions}
                      disabled={saving}
                      onChange={setDevice}
                    />
                  </label>
                  <label
                    htmlFor="profile-score"
                    className="block space-y-1 text-sm"
                  >
                    活跃分数（0～100）
                    <Input
                      id="profile-score"
                      disabled={saving}
                      type="number"
                      min={0}
                      max={100}
                      step="any"
                      value={score}
                      onChange={(event) => setScore(event.target.value)}
                    />
                  </label>
                </div>
                <ProfileFieldsEditor
                  value={extraFields}
                  onChange={setExtraFields}
                  disabled={saving}
                />
                <Button
                  type="submit"
                  className="w-full"
                  disabled={
                    saving ||
                    loading ||
                    Boolean(loadError) ||
                    !canOperate ||
                    !userID.trim()
                  }
                >
                  {saving && <LoaderCircle className="animate-spin" />}
                  {saving
                    ? '正在保存…'
                    : saveMode === 'copy'
                      ? '另存为新用户'
                      : '保存画像'}
                </Button>
              </fieldset>
            </form>
            {selected && (
              <>
                <Button
                  type="button"
                  variant="outline"
                  className="mt-3 w-full"
                  disabled={saving || !canOperate || saveMode === 'copy'}
                  onClick={() => onDecide(selected.userId)}
                >
                  {saveMode === 'copy'
                    ? '请先另存新用户'
                    : '使用此用户发起决策'}
                </Button>
                <details className="mt-3 rounded-lg border p-3">
                  <summary className="cursor-pointer text-sm">
                    完整已保存属性
                  </summary>
                  <pre className="mt-2 max-h-64 overflow-auto text-xs">
                    {JSON.stringify(selected, null, 2)}
                  </pre>
                </details>
              </>
            )}
          </CardContent>
        </Card>
      </div>
    </>
  );
}
